package watcher

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loykin/freader/internal/file_tracker"
)

const FingerprintStrategyChecksum = "checksum"
const FingerprintStrategyChecksumSeperator = "checksumSeperator"
const FingerprintStrategyDeviceAndInode = "deviceAndInode"
const DefaultFingerprintStrategySize = 1024

type Watcher struct {
	FingerprintStrategy  string
	FingerprintSize      int
	FingerprintSeperator string
	interval             time.Duration
	callback             func(id, path string)
	removeCallback       func(id string)
	stopCh               chan struct{}
	fileManager          *file_tracker.FileTracker
	exclude              []string
	include              []string
}

func NewWatcher(config Config, cb func(id, path string), removeCb func(id string)) (*Watcher, error) {
	// Derive roots from include patterns (single unified concept).
	paths := deriveScanRoots(config.Include)

	for i := 0; i < len(paths); i++ {
		base := filepath.Clean(paths[i])
		for j := 0; j < len(paths); j++ {
			if i == j {
				continue
			}
			other := filepath.Clean(paths[j])
			if isSubPath(base, other) {
				return nil, errors.New("overlapping watch paths: " + base + " is subpath of " + other)
			}
		}
	}

	// Validate strategy-specific requirements via Config.Validate
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Watcher{
		interval:             config.PollInterval,
		callback:             cb,
		FingerprintStrategy:  config.FingerprintStrategy,
		FingerprintSize:      config.FingerprintSize,
		FingerprintSeperator: config.FingerprintSeperator,
		removeCallback:       removeCb,
		stopCh:               make(chan struct{}),
		fileManager:          config.FileTracker,
		exclude:              config.Exclude,
		include:              config.Include,
	}, nil
}

func (w *Watcher) Start() {
	ticker := time.NewTicker(w.interval)

	go func() {
		defer ticker.Stop()

		// Perform an immediate scan on start
		w.scan()

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

	// Determine if there are specific include patterns (globs or exact files)
	hasSpecific := false
	for _, pattern := range w.include {
		cp := filepath.Clean(pattern)
		if hasMeta(cp) {
			hasSpecific = true
			break
		}
		if fi, err := os.Stat(cp); err != nil {
			// Path does not exist: if no meta, treat as specific (likely a file name pattern)
			hasSpecific = true
			break
		} else if !fi.IsDir() {
			hasSpecific = true
			break
		}
	}

	// Derive roots dynamically from includes each scan (no persistent roots field)
	roots := deriveScanRoots(w.include)

	for _, root := range roots {
		err := filepath.Walk(root, func(p string, info fs.FileInfo, err error) error {
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
				base := filepath.Base(p)
				for _, pattern := range w.include {
					cleanPat := filepath.Clean(pattern)
					// Directory-like include (no glob)
					if !hasMeta(cleanPat) {
						if infoDir, err := os.Stat(cleanPat); (err == nil && infoDir.IsDir()) || strings.HasSuffix(pattern, string(filepath.Separator)) {
							// If we also have specific includes (globs or exact files), ignore broad directory includes as filters
							if !hasSpecific {
								if isSubPath(p, strings.TrimSuffix(cleanPat, string(filepath.Separator))) {
									matched = true
									break
								}
							}
						} else {
							// Treat as exact file path match (support relative/absolute by cleaning both)
							if filepath.Clean(p) == cleanPat || filepath.Base(p) == cleanPat {
								matched = true
								break
							}
						}
						continue
					}
					// Glob patterns: match against base and full path
					if ok, _ := filepath.Match(cleanPat, base); ok {
						matched = true
						break
					}
					if ok, _ := filepath.Match(cleanPat, p); ok {
						matched = true
						break
					}
				}
				if !matched {
					return nil
				}
			}

			if len(w.exclude) > 0 {
				base := filepath.Base(p)
				for _, pattern := range w.exclude {
					if ok, _ := filepath.Match(pattern, base); ok {
						return nil
					}
					if ok, _ := filepath.Match(pattern, p); ok {
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
			case FingerprintStrategyChecksumSeperator:
				fileId, err = file_tracker.GetFileFingerprintUntilNSeparatorsFromPath(p, w.FingerprintSeperator, w.FingerprintSize)
				if file_tracker.IsNotEnoughSeparators(err) {
					return nil
				} else if err != nil {
					slog.Warn("failed to get file fingerprint (separator)", "path", p, "error", err)
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
			slog.Error("failed to walk path", "path", root, "error", err)
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
