package tailer

import (
	"errors"
	"regexp"
	"sync"
	"time"
)

const (
	MultilineReaderModeContinuePast    = "continuePast"
	MultilineReaderModeContinueThrough = "continueThrough"
	MultilineReaderModeHaltBefore      = "haltBefore"
	MultilineReaderModeHaltWith        = "haltWith"
)

type MultilineReader struct {
	Mode             string
	ConditionPattern string // e.g. "^\\s" for indented lines, or "^(INFO|ERROR)" for boundaries
	StartPattern     string // start of a multiline record; if set, only lines matching this begin accumulation
	Timeout          time.Duration

	re      *regexp.Regexp // compiled condition pattern
	startRe *regexp.Regexp // compiled start pattern
	buf     []byte         // current assembling record (without trailing separator)
	queue   [][]byte       // ready records to be Read()
	last    time.Time      // last time buf was updated

	// channel-based delivery
	outCh   chan []byte
	stopCh  chan struct{}
	started bool

	mu sync.Mutex // protects buf/queue/last and started/outCh/stopCh
}

func (m *MultilineReader) Validate() error {
	if m.StartPattern == "" {
		return errors.New("StartPattern is required")
	}
	if m.ConditionPattern == "" {
		return errors.New("ConditionPattern is required")
	}
	if m.Timeout <= 0 {
		return errors.New("timeout must be > 0")
	}
	return nil
}

func (m *MultilineReader) init() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.re == nil && m.ConditionPattern != "" {
		re, err := regexp.Compile(m.ConditionPattern)
		if err != nil {
			return err
		}
		m.re = re
	}
	if m.startRe == nil && m.StartPattern != "" {
		sre, err := regexp.Compile(m.StartPattern)
		if err != nil {
			return err
		}
		m.startRe = sre
	}
	m.start()
	return nil
}

// start initializes the output channel and a background goroutine that
// periodically checks for Timeout to flush the current buffer.
func (m *MultilineReader) start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return
	}
	m.outCh = make(chan []byte, 64)
	m.stopCh = make(chan struct{})
	m.started = true
	go func() {
		// Use a ticker granularity smaller than Timeout
		interval := m.Timeout / 4
		if interval <= 0 {
			interval = m.Timeout
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.mu.Lock()
				if len(m.buf) > 0 && !m.last.IsZero() && time.Since(m.last) >= m.Timeout {
					// flush due to timeout
					rec := append([]byte(nil), m.buf...)
					m.queue = append(m.queue, rec)
					m.buf = nil
					// non-blocking send
					if m.outCh != nil {
						select {
						case m.outCh <- rec:
						default:
						}
					}
				}
				m.mu.Unlock()
			}
		}
	}()
}

// Recv returns a channel that delivers completed multiline records.
func (m *MultilineReader) Recv() <-chan []byte {
	_ = m.init()
	m.mu.Lock()
	ch := m.outCh
	m.mu.Unlock()
	return ch
}

// Close stops the background goroutine.
func (m *MultilineReader) Close() {
	m.mu.Lock()
	if !m.started || m.stopCh == nil {
		m.mu.Unlock()
		return
	}
	close(m.stopCh)
	m.started = false
	m.mu.Unlock()
}

// Write ingests one logical line (without its separator) and updates the multiline state.
// It is safe to call sequentially from a single goroutine (TailReader loop).
func (m *MultilineReader) Write(b []byte) error {
	if err := m.init(); err != nil {
		return err
	}
	line := append([]byte(nil), b...) // copy
	m.mu.Lock()
	defer m.mu.Unlock()
	// If no current buffer, decide whether to start a new record using StartPattern if provided.
	if len(m.buf) == 0 {
		if m.startRe != nil {
			if m.startRe.Match(line) {
				m.buf = line
				m.last = time.Now()
				return nil
			}
			// Not a start line: emit as a standalone record and publish to channel
			rec := append([]byte(nil), line...)
			m.queue = append(m.queue, rec)
			if m.outCh != nil {
				select {
				case m.outCh <- rec:
				default:
				}
			}
			return nil
		}
		// No start pattern configured; start with incoming line
		m.buf = line
		m.last = time.Now()
		return nil
	}

	matches := false
	if m.re != nil {
		matches = m.re.Match(line)
	}

	switch m.Mode {
	case MultilineReaderModeContinuePast:
		// If line matches condition => keep accumulating.
		// If it does NOT match => include it to current and emit the record (past the condition), start new buffer empty.
		if matches {
			m.buf = appendWithNL(m.buf, line)
			m.last = time.Now()
			return nil
		}
		m.buf = appendWithNL(m.buf, line)
		m.enqueueAndResetLocked()
		return nil

	case MultilineReaderModeContinueThrough:
		// If line matches => keep accumulating; if not => emit current, then start new if StartPattern allows, else emit as single
		if matches {
			m.buf = appendWithNL(m.buf, line)
			m.last = time.Now()
			return nil
		}
		m.enqueueAndResetLocked()
		if m.startRe != nil {
			if m.startRe.Match(line) {
				m.buf = line
				m.last = time.Now()
				return nil
			}
			// Not a start line; emit it as standalone and publish to channel
			rec := append([]byte(nil), line...)
			m.queue = append(m.queue, rec)
			if m.outCh != nil {
				select {
				case m.outCh <- rec:
				default:
				}
			}
			return nil
		}
		m.buf = line
		m.last = time.Now()
		return nil

	case MultilineReaderModeHaltBefore:
		// When condition matches, close previous and start new with this line (halt before this line)
		if matches {
			m.enqueueAndResetLocked()
			if m.startRe != nil {
				if m.startRe.Match(line) {
					m.buf = line
					m.last = time.Now()
					return nil
				}
				// Not a start line; emit as standalone and keep buffer empty, and publish to channel
				rec := append([]byte(nil), line...)
				m.queue = append(m.queue, rec)
				if m.outCh != nil {
					select {
					case m.outCh <- rec:
					default:
					}
				}
				return nil
			}
			m.buf = line
			m.last = time.Now()
			return nil
		}
		m.buf = appendWithNL(m.buf, line)
		m.last = time.Now()
		return nil

	case MultilineReaderModeHaltWith:
		// When condition matches, include this line in previous and emit.
		if matches {
			m.buf = appendWithNL(m.buf, line)
			m.enqueueAndResetLocked()
			return nil
		}
		m.buf = appendWithNL(m.buf, line)
		m.last = time.Now()
		return nil
	default:
		// If mode is empty/unknown, default: no multiline, simply emit previous and make this line current
		m.enqueueAndResetLocked()
		m.buf = line
		m.last = time.Now()
		return nil
	}
}

// Read returns the next completed record if available, else io.EOF
func (m *MultilineReader) Read() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.queue) == 0 {
		return nil, errors.New("EOF")
	}
	rec := m.queue[0]
	m.queue = append([][]byte{}, m.queue[1:]...)
	return rec, nil
}

// Flush emits the currently buffered record, if any.
func (m *MultilineReader) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.buf) > 0 {
		m.enqueueAndResetLocked()
	}
}

func (m *MultilineReader) enqueueAndResetLocked() {
	if len(m.buf) == 0 {
		return
	}
	rec := append([]byte(nil), m.buf...)
	m.queue = append(m.queue, rec)
	m.buf = nil
	if m.outCh != nil {
		select {
		case m.outCh <- rec:
		default:
		}
	}
}

func appendWithNL(dst, line []byte) []byte {
	if len(dst) == 0 {
		return append(dst, line...)
	}
	// Join lines with a single newline to reconstruct logical message
	dst = append(dst, '\n')
	return append(dst, line...)
}
