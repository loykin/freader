package watcher

import (
	"errors"
	"freader/pkg/file_tracker"
	"io/fs"
	"log/slog"
	"path/filepath"
	"time"
)

const FingerprintStrategyChecksum = "checksum"
const FingerprintStrategyDeviceAndInode = "deviceAndInode"
const DefaultFingerprintStrategySize = 1024

type Watcher struct {
	paths               []string
	FingerprintStrategy string
	FingerprintSize     int
	interval            time.Duration
	callback            func(id, path string)
	removeCallback      func(id string)
	stopCh              chan struct{}
	fileManager         *file_tracker.FileTracker
	exclude             []string
	include             []string
}

func NewWatcher(config Config, cb func(id, path string), removeCb func(id string)) (*Watcher, error) {
	for i := 0; i < len(config.Paths); i++ {
		base := filepath.Clean(config.Paths[i])
		for j := 0; j < len(config.Paths); j++ {
			if i == j {
				continue
			}
			other := filepath.Clean(config.Paths[j])
			if isSubPath(base, other) {
				return nil, errors.New("overlapping watch paths: " + base + " is subpath of " + other)
			}
		}
	}

	switch config.FingerprintStrategy {
	case FingerprintStrategyDeviceAndInode:
		break
	case FingerprintStrategyChecksum:
		if config.FingerprintSize <= 0 {
			return nil, errors.New("fingerprint size must be greater than 0")
		}
	default:
		return nil, errors.New("unsupported fingerprint strategy: " + config.FingerprintStrategy)
	}

	return &Watcher{
		paths:               config.Paths,
		interval:            config.PollInterval,
		callback:            cb,
		FingerprintStrategy: config.FingerprintStrategy,
		FingerprintSize:     config.FingerprintSize,
		removeCallback:      removeCb,
		stopCh:              make(chan struct{}),
		fileManager:         config.FileTracker,
		exclude:             config.Exclude,
		include:             config.Include,
	}, nil
}

func (w *Watcher) Start() {
	ticker := time.NewTicker(w.interval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-w.stopCh:
				return
			case <-ticker.C:
				w.scan()
			}
		}
	}()
}

func (w *Watcher) Stop() {
	close(w.stopCh)
}

func (w *Watcher) scan() {
	existingFiles := make(map[string]bool)

	for _, path := range w.paths {
		err := filepath.Walk(path, func(p string, info fs.FileInfo, err error) error {
			if err != nil {
				slog.Warn("failed to walk", "path", p, "error", err)
				return nil
			}

			if info != nil && info.IsDir() {
				return nil
			}

			// Apply include/exclude filters
			if len(w.include) > 0 {
				matched := false
				for _, pattern := range w.include {
					if matched, _ = filepath.Match(pattern, filepath.Base(p)); matched {
						break
					}
				}
				if !matched {
					return nil
				}
			}

			if len(w.exclude) > 0 {
				for _, pattern := range w.exclude {
					if matched, _ := filepath.Match(pattern, filepath.Base(p)); matched {
						return nil
					}
				}
			}

			// Skip processing if the file size is 0 to prevent detecting files
			// before they are fully created/written
			if info.Size() == 0 {
				return nil
			}

			var fileId string
			switch w.FingerprintStrategy {
			case FingerprintStrategyChecksum:
				fileId, err = file_tracker.GetFileFingerprintFromPath(p, int64(w.FingerprintSize))
				if file_tracker.IsFileSizeTooSmall(err) {
					return nil
				} else if err != nil {
					slog.Warn("failed to get file fingerprint", "path", p, "error", err)
					return nil
				}
			case FingerprintStrategyDeviceAndInode:
				fileId, err = file_tracker.GetFileIDFromPath(p)
				if err != nil {
					slog.Warn("failed to get file inode", "path", p, "error", err)
					return nil
				}
			default:
				return errors.New("unsupported fingerprint strategy: " + w.FingerprintStrategy)
			}

			existingFiles[fileId] = true

			manager := w.fileManager.Get(fileId)
			if manager == nil {
				w.fileManager.Add(fileId, p, w.FingerprintStrategy, int64(w.FingerprintSize), 0)
				w.callback(fileId, p)
			}

			return nil
		})
		if err != nil {
			slog.Error("failed to walk path", "path", path, "error", err)
			continue
		}
	}

	for fileId := range w.fileManager.GetAllFiles() {
		if !existingFiles[fileId] {
			if w.removeCallback != nil {
				w.removeCallback(fileId)
			}
			w.fileManager.Remove(fileId)
		}
	}
}
