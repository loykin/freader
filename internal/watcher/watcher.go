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
const FingerprintStrategyChecksumSeparator = "checksumSeparator"
const FingerprintStrategyDeviceAndInode = "deviceAndInode"
const DefaultFingerprintStrategySize = 1024

type Watcher struct {
	FingerprintStrategy  string
	FingerprintSize      int
	FingerprintSeparator string
	interval             time.Duration
	callback             func(id, path string)
	removeCallback       func(id string)
	stopCh               chan struct{}
	doneCh               chan struct{} // Signal when goroutine has finished
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
		FingerprintSeparator: config.FingerprintSeparator,
		removeCallback:       removeCb,
		stopCh:               make(chan struct{}),
		doneCh:               make(chan struct{}),
		fileManager:          config.FileTracker,
		exclude:              config.Exclude,
		include:              config.Include,
	}, nil
}

// computeFileID computes the file fingerprint/id according to the watcher's strategy.
// Returns ok=false for expected skip conditions (e.g., zero-size, too small, not enough separators).
func (w *Watcher) computeFileID(p string, info fs.FileInfo) (string, bool) {
	if info == nil {
		return "", false
	}
	// Skip empty files to avoid premature detection
	if info.Size() == 0 {
		return "", false
	}
	var (
		id  string
		err error
	)
	switch w.FingerprintStrategy {
	case FingerprintStrategyChecksum:
		id, err = file_tracker.GetFileFingerprintFromPath(p, int64(w.FingerprintSize))
		if file_tracker.IsFileSizeTooSmall(err) {
			return "", false
		} else if err != nil {
			slog.Warn("failed to get file fingerprint", "path", p, "error", err)
			return "", false
		}
	case FingerprintStrategyChecksumSeparator:
		id, err = file_tracker.GetFileFingerprintUntilNSeparatorsFromPath(p, w.FingerprintSeparator, w.FingerprintSize)
		if file_tracker.IsNotEnoughSeparators(err) {
			return "", false
		} else if err != nil {
			slog.Warn("failed to get file fingerprint (separator)", "path", p, "error", err)
			return "", false
		}
	case FingerprintStrategyDeviceAndInode:
		id, err = file_tracker.GetFileIDFromPath(p)
		if err != nil {
			slog.Warn("failed to get file inode", "path", p, "error", err)
			return "", false
		}
	default:
		// preserve previous behavior: return an error to stop walk on unexpected strategy
		slog.Error("unsupported fingerprint strategy", "strategy", w.FingerprintStrategy)
		return "", false
	}
	return id, true
}

func (w *Watcher) Start() {
	ticker := time.NewTicker(w.interval)

	go func() {
		defer func() {
			ticker.Stop()
			close(w.doneCh) // Signal that goroutine has finished
		}()

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
	select {
	case <-w.stopCh:
		return // Already stopped
	default:
		close(w.stopCh)
	}
}

// StopAndWait stops the watcher and waits for the goroutine to finish
func (w *Watcher) StopAndWait() {
	w.Stop()
	<-w.doneCh // Wait for goroutine to finish
}

func (w *Watcher) scan() {
	existingFiles := make(map[string]bool)

	// Determine if there are specific include patterns (globs or exact files)
	hasSpecific := hasSpecificIncludes(w.include)

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

			// Filters: include first, then exclude
			if len(w.include) > 0 && !pathIncluded(p, w.include, hasSpecific) {
				return nil
			}
			if len(w.exclude) > 0 && pathExcluded(p, w.exclude) {
				return nil
			}

			// Compute file ID according to strategy (with size/condition checks)
			fileId, ok := w.computeFileID(p, info)
			if !ok {
				return nil
			}

			existingFiles[fileId] = true

			if w.fileManager.Get(fileId) == nil {
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

// hasSpecificIncludes returns true if includes contain any glob, non-existent path,
// or an explicit file (non-directory). This affects how broad directory includes are treated.
func hasSpecificIncludes(includes []string) bool {
	for _, pattern := range includes {
		cp := filepath.Clean(pattern)
		if hasMeta(cp) {
			return true
		}
		if fi, err := os.Stat(cp); err != nil {
			// Path does not exist: if no meta, treat as specific (likely a file name pattern)
			return true
		} else if !fi.IsDir() {
			return true
		}
	}
	return false
}

// pathIncluded checks whether path p should be included according to include patterns.
// When hasSpecific is true (globs or explicit files present), broad directory includes are ignored as filters.
func pathIncluded(p string, includes []string, hasSpecific bool) bool {
	base := filepath.Base(p)
	for _, pattern := range includes {
		cleanPat := filepath.Clean(pattern)
		// Directory-like include (no glob)
		if !hasMeta(cleanPat) {
			if infoDir, err := os.Stat(cleanPat); (err == nil && infoDir.IsDir()) || strings.HasSuffix(pattern, string(filepath.Separator)) {
				// If we also have specific includes (globs or exact files), ignore broad directory includes as filters
				if !hasSpecific {
					if isSubPath(p, strings.TrimSuffix(cleanPat, string(filepath.Separator))) {
						return true
					}
				}
			} else {
				// Treat as exact file path match (support relative/absolute by cleaning both)
				if filepath.Clean(p) == cleanPat || filepath.Base(p) == cleanPat {
					return true
				}
			}
			continue
		}
		// Glob patterns: match against base and full path
		if ok, _ := filepath.Match(cleanPat, base); ok {
			return true
		}
		if ok, _ := filepath.Match(cleanPat, p); ok {
			return true
		}
	}
	return false
}

// pathExcluded checks whether path p matches any exclude pattern.
func pathExcluded(p string, excludes []string) bool {
	base := filepath.Base(p)
	for _, pattern := range excludes {
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, p); ok {
			return true
		}
	}
	return false
}
