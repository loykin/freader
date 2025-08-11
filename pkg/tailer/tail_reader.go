package tailer

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/loykin/freader/pkg/file_tracker"
	"github.com/loykin/freader/pkg/watcher"
)

type TailReader struct {
	FileId      string
	Offset      int64
	Separator   byte
	stopCh      chan struct{}
	doneCh      chan struct{}
	FileManager *file_tracker.FileTracker
	file        *os.File
	reader      *bufio.Reader
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
	return t.reader.ReadBytes(t.Separator)
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
				callback(string(bytes.TrimSuffix(chunk, []byte{t.Separator})))
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
		if len(chunk) > 1 {
			callback(string(chunk[:len(chunk)-1]))
		}
	}
}

func (t *TailReader) Run(callback func(string)) {
	t.stopCh = make(chan struct{})
	t.doneCh = make(chan struct{})

	go func() {
		defer close(t.doneCh)
		if err := t.readLoop(callback); err != nil {
			slog.Error("failed to read file", "file", t.FileId, "error", err)
			t.FileManager.Remove(t.FileId)
		}
	}()
}

func (t *TailReader) Stop() {
	if t.stopCh == nil {
		return
	}
	select {
	case <-t.stopCh:
	default:
		close(t.stopCh)
	}
	<-t.doneCh
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
}

func (t *TailReader) Close() {
	t.Stop()
}
