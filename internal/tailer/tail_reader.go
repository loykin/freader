package tailer

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/loykin/freader/internal/file_tracker"
	"github.com/loykin/freader/internal/watcher"
)

// bufferPool is a global pool for reusing byte slices to reduce memory allocations
var bufferPool = sync.Pool{
	New: func() interface{} {
		// Start with a reasonable buffer size (4KB)
		buf := make([]byte, 0, 4096)
		return &buf
	},
}

type TailReader struct {
	FileId    string
	Offset    int64
	Separator string
	// Optional multiline aggregator; if set, physical lines are grouped into logical records.
	Multiline *MultilineReader
	// mu protects access to stopCh and doneCh to avoid data races between Run and Stop
	mu          sync.Mutex
	stopCh      chan struct{}
	doneCh      chan struct{}
	FileManager *file_tracker.FileTracker
	file        *os.File
	reader      *bufio.Reader
	buf         []byte // internal buffer across reads for multi-byte separators
}

func (t *TailReader) open() error {
	if t.file != nil {
		return nil
	}

	fileInfo := t.FileManager.Get(t.FileId)
	if fileInfo == nil {
		return errors.New("file not found: " + t.FileId)
	}

	file, err := os.Open(fileInfo.Path)
	if err != nil {
		return err
	}

	var fileId string
	switch fileInfo.FingerprintStrategy {
	case watcher.FingerprintStrategyChecksum:
		fileId, err = file_tracker.GetFileFingerprint(file, fileInfo.FingerprintSize)
		if err != nil {
			// If file is too small for fingerprinting, it should have been skipped by watcher
			// This can happen if file grew after initial scan
			if file_tracker.IsFileSizeTooSmall(err) {
				slog.Debug("file too small for fingerprinting",
					"path", fileInfo.Path, "fileId", t.FileId, "error", err)
			}
			return err
		}
	case watcher.FingerprintStrategyChecksumSeparator:
		fileId, err = file_tracker.GetFileFingerprintUntilNSeparators(file, t.Separator, int(fileInfo.FingerprintSize))
		if err != nil {
			// If file doesn't have enough separators, it should have been skipped by watcher
			// This can happen if file content changed after initial scan
			if file_tracker.IsNotEnoughSeparators(err) {
				slog.Debug("file has insufficient separators",
					"path", fileInfo.Path, "fileId", t.FileId, "error", err)
			}
			return err
		}
	case watcher.FingerprintStrategyDeviceAndInode:
		stat, err := file.Stat()
		if err != nil {
			return err
		}

		fileId, err = file_tracker.GetFileID(stat)
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported fingerprint strategy: " + fileInfo.FingerprintStrategy)
	}

	if fileId != t.FileId {
		// File content has changed (rotation, truncation, or overwrite)
		// This is a normal scenario in dynamic environments
		slog.Debug("file content changed, fingerprint mismatch",
			"path", fileInfo.Path, "current_fingerprint", fileId, "tracked_fingerprint", t.FileId)
		_ = file.Close()
		return &FileFingerprintMismatchError{
			Path:                fileInfo.Path,
			ExpectedFingerprint: t.FileId,
			ActualFingerprint:   fileId,
		}
	}

	_, err = file.Seek(t.Offset, io.SeekStart)
	if err != nil {
		_ = file.Close()
		return err
	}

	t.file = file
	t.reader = bufio.NewReader(t.file)

	// Initialize buffer from pool if not already set
	if t.buf == nil {
		bufPtr := bufferPool.Get().(*[]byte)
		t.buf = (*bufPtr)[:0] // reset length but keep capacity
	}

	return nil
}

func (t *TailReader) readNextChunk() ([]byte, error) {
	sep := []byte(t.Separator)
	if len(sep) == 0 {
		return nil, errors.New("separator must not be empty")
	}
	// Use internal buffer t.buf. Keep reading until we find sep or hit EOF.
	for {
		// Search for separator in existing buffer
		if idx := bytes.Index(t.buf, sep); idx >= 0 {
			end := idx + len(sep)
			chunk := t.buf[:end]
			// advance buffer efficiently using copy instead of allocating new slice
			if end < len(t.buf) {
				copy(t.buf, t.buf[end:])
				t.buf = t.buf[:len(t.buf)-end]
			} else {
				t.buf = t.buf[:0] // reset buffer if we consumed everything
			}
			return chunk, nil
		}
		// Read more data
		data, err := t.reader.ReadBytes(sep[len(sep)-1])
		t.buf = append(t.buf, data...)
		if err != nil {
			if err == io.EOF {
				// No complete separator in buffer; do not emit partial
				return nil, io.EOF
			}
			return nil, err
		}
	}
}

func (t *TailReader) readLoop(callback func(string)) error {
	if err := t.open(); err != nil {
		return err
	}
	defer t.cleanup()

	for {
		select {
		case <-t.stopCh:
			return nil
		default:
			chunk, err := t.readNextChunk()
			if err != nil {
				if err == io.EOF {
					// No new complete chunk. If multiline is enabled, drain any timeout-flushed records.
					if t.Multiline != nil {
						for {
							rec, rerr := t.Multiline.Read()
							if rerr != nil {
								break
							}
							callback(string(rec))
						}
					}
					time.Sleep(500 * time.Millisecond)
					continue
				}
				return err
			}

			// Process chunk respecting multiline configuration
			sep := []byte(t.Separator)
			line := chunk
			if len(chunk) >= len(sep) {
				line = chunk[:len(chunk)-len(sep)]
			}

			if t.Multiline != nil {
				_ = t.Multiline.Write(line)
				for {
					rec, rerr := t.Multiline.Read()
					if rerr != nil {
						break
					}
					callback(string(rec))
				}
			} else {
				if len(chunk) > len(sep) {
					callback(string(line))
				}
			}

			// Advance offset for consumed chunk
			t.Offset += int64(len(chunk))
		}
	}
}

func (t *TailReader) ReadOnce(callback func(string)) error {
	if err := t.open(); err != nil {
		return err
	}
	defer t.cleanup()

	for {
		chunk, err := t.readNextChunk()
		if err != nil {
			if err == io.EOF {
				// EOF for one-shot read. If there's residual data in our buffer (no trailing separator),
				// account for it in the offset and deliver it appropriately.
				// If multiline configured, flush residual aggregated record(s) and drain them.
				if t.Multiline != nil {
					var residual []byte
					if len(t.buf) > 0 {
						residual = append([]byte(nil), t.buf...)
						// clear buffer as we're consuming it now
						t.buf = nil
						_ = t.Multiline.Write(residual)
						// advance offset by the unread bytes we've buffered
						t.Offset += int64(len(residual))
					}
					t.Multiline.Flush()
					for {
						rec, rerr := t.Multiline.Read()
						if rerr != nil {
							break
						}
						callback(string(rec))
					}
				}
				return nil
			}
			return err
		}

		// multipline flush, get
		sep := []byte(t.Separator)
		line := chunk
		if len(chunk) >= len(sep) {
			line = chunk[:len(chunk)-len(sep)]
		}

		if t.Multiline != nil {
			// Feed the physical line into the multiline aggregator and drain any ready records.
			_ = t.Multiline.Write(line)
			for {
				rec, rerr := t.Multiline.Read()
				if rerr != nil {
					break
				}
				callback(string(rec))
			}
		} else {
			// If not using multiline, emit the single logical line when there is content beyond the separator.
			if len(chunk) > len(sep) {
				callback(string(line))
			}
		}

		// Always advance offset for consumed chunk, even if it's just a separator (blank line)
		t.Offset += int64(len(chunk))
	}
}

func (t *TailReader) Run(callback func(string)) {
	// Initialize channels under lock to prevent races with Stop()
	t.mu.Lock()
	t.stopCh = make(chan struct{})
	t.doneCh = make(chan struct{})
	localDone := t.doneCh
	t.mu.Unlock()

	go func() {
		defer close(localDone)
		if err := t.readLoop(callback); err != nil {
			slog.Error("failed to read file", "file", t.FileId, "error", err)
			t.FileManager.Remove(t.FileId)
		}
	}()
}

func (t *TailReader) Stop() {
	// Safely capture channels under lock to avoid races with Run()
	t.mu.Lock()
	if t.stopCh == nil || t.doneCh == nil {
		t.mu.Unlock()
		return
	}
	localStop := t.stopCh
	localDone := t.doneCh
	t.mu.Unlock()

	// Close stop channel once (non-blocking if already closed)
	select {
	case <-localStop:
		// already closed
	default:
		close(localStop)
	}

	// Wait for the reader goroutine to finish
	<-localDone

	if t.FileId != "" {
		t.FileManager.Remove(t.FileId)
	}
}

func (t *TailReader) cleanup() {
	if t.file != nil {
		_ = t.file.Close()
		t.file = nil
	}
	t.reader = nil

	// Return buffer to pool for reuse instead of setting to nil
	if t.buf != nil {
		// Reset the buffer length but keep capacity for reuse
		t.buf = t.buf[:0]
		bufferPool.Put(&t.buf)
		t.buf = nil
	}
}

func (t *TailReader) Close() {
	t.Stop()
}
