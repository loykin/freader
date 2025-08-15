package console

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/loykin/freader/cmd/freader/sink/common"
)

// stdoutSink batches and writes lines to stdout (or any io.Writer) as a sink.
type stdoutSink struct {
	batcher common.Batcher
	w       io.Writer
}

// New returns a console sink writing to stdout or stderr depending on stream.
// stream: "stdout" (default) or "stderr".
func New(stream string, batchSize int, batchInterval time.Duration, includes, excludes []string) common.Sink {
	w := os.Stdout
	if stream == "stderr" {
		w = os.Stderr
	}
	s := &stdoutSink{batcher: common.NewBatcher(batchSize, batchInterval, includes, excludes), w: w}
	s.start()
	return s
}

type fileSink struct {
	batcher common.Batcher
	path    string
	f       *os.File
}

// NewFile creates a file sink and starts it.
func NewFile(path string, batchSize int, batchInterval time.Duration, includes, excludes []string) (common.Sink, error) {
	if path == "" {
		return nil, errors.New("file sink requires a path")
	}
	s := &fileSink{batcher: common.NewBatcher(batchSize, batchInterval, includes, excludes), path: path}
	s.start()
	return s, nil
}

func (s *fileSink) start() {
	s.batcher.Wg.Add(1)
	go func() {
		defer s.batcher.Wg.Done()
		var err error
		s.f, err = os.Create(s.path)
		if err != nil {
			slog.Error("file sink open failed", "error", err)
			return
		}
		buf := make([]string, 0, s.batcher.BatchSize)
		ticker := time.NewTicker(s.batcher.BatchInterval)
		defer ticker.Stop()
		flush := func() {
			for _, ln := range buf {
				_, _ = fmt.Fprintln(s.f, ln)
			}
			buf = buf[:0]
		}
		for {
			select {
			case <-s.batcher.StopCh:
				flush()
				return
			case <-ticker.C:
				flush()
			case line := <-s.batcher.Ch:
				buf = append(buf, line)
				if len(buf) >= s.batcher.BatchSize {
					flush()
				}
			}
		}
	}()
}

func (s *fileSink) Enqueue(line string) { s.batcher.Enqueue(line) }

func (s *fileSink) Stop() error {
	s.batcher.StopOnce.Do(func() { close(s.batcher.StopCh) })
	s.batcher.Wg.Wait()
	if s.f != nil {
		_ = s.f.Close()
	}
	return nil
}

func (s *stdoutSink) start() {
	s.batcher.Wg.Add(1)
	go func() {
		defer s.batcher.Wg.Done()
		buf := make([]string, 0, s.batcher.BatchSize)
		ticker := time.NewTicker(s.batcher.BatchInterval)
		defer ticker.Stop()
		flush := func() {
			for _, ln := range buf {
				_, _ = fmt.Fprintln(s.w, ln)
			}
			buf = buf[:0]
		}
		for {
			select {
			case <-s.batcher.StopCh:
				flush()
				return
			case <-ticker.C:
				flush()
			case line := <-s.batcher.Ch:
				buf = append(buf, line)
				if len(buf) >= s.batcher.BatchSize {
					flush()
				}
			}
		}
	}()
}

func (s *stdoutSink) Enqueue(line string) { s.batcher.Enqueue(line) }

func (s *stdoutSink) Stop() error {
	s.batcher.StopOnce.Do(func() { close(s.batcher.StopCh) })
	s.batcher.Wg.Wait()
	return nil
}
