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

type TailReader struct {
	FileId    string
	Offset    int64
	Separator string
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
			return err
		}
	case watcher.FingerprintStrategyChecksumSeperator:
		fileId, err = file_tracker.GetFileFingerprintUntilNSeparators(file, t.Separator, int(fileInfo.FingerprintSize))
		if err != nil {
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
		slog.Warn("file id mismatch",
			"path", fileInfo.Path, "fileId", fileId, "expected", t.FileId)
		_ = file.Close()
		return errors.New("file id mismatch(path:" + fileInfo.Path + ": " + fileId + " != " + t.FileId)
	}

	_, err = file.Seek(t.Offset, io.SeekStart)
	if err != nil {
		_ = file.Close()
		return err
	}

	t.file = file
	t.reader = bufio.NewReader(t.file)

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
			// advance buffer
			t.buf = append([]byte{}, t.buf[end:]...)
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
					time.Sleep(500 * time.Millisecond)
					continue
				}
				return err
			}

			t.Offset += int64(len(chunk))
			if len(chunk) > 1 {
				callback(string(bytes.TrimSuffix(chunk, []byte(t.Separator))))
			}
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
				return nil
			}
			return err
		}
		// Always advance offset for consumed chunk, even if it's just a separator (blank line)
		t.Offset += int64(len(chunk))
		sep := []byte(t.Separator)
		if len(chunk) > len(sep) {
			callback(string(chunk[:len(chunk)-len(sep)]))
		}
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
	t.buf = nil
}

func (t *TailReader) Close() {
	t.Stop()
}
