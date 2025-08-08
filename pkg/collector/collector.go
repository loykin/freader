package collector

import (
	"freader/pkg/file_tracker"
	"freader/pkg/store"
	"freader/pkg/tailer"
	"freader/pkg/watcher"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
)

type Collector struct {
	cfg         Config
	fileManager *file_tracker.FileTracker
	watcher     *watcher.Watcher
	offsetDB    store.Store
	scheduler   *TailScheduler
	mu          sync.Mutex
	onLineFunc  func(line string)
	stopCh      chan struct{}
	workerWg    sync.WaitGroup
}

func (c *Collector) worker() {
	defer c.workerWg.Done()

	loopCount := 0
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 100 * time.Millisecond
	bo.MaxInterval = 2 * time.Second
	bo.MaxElapsedTime = 0

	loopLimit := c.scheduler.GetCount()

	for {
		select {
		case <-c.stopCh:
			return
		default:
			if loopCount >= loopLimit {
				select {
				case <-c.stopCh:
					return
				case <-time.After(bo.NextBackOff()):
					loopLimit = c.scheduler.GetCount()
					loopCount = 0
				}
			}
			loopCount++

			fileTail, ok := c.scheduler.getNextAvailable()
			if !ok {
				continue
			}

			err := fileTail.ReadOnce(func(line string) {
				c.mu.Lock()
				defer c.mu.Unlock()
				if c.onLineFunc != nil {
					c.onLineFunc(line)
				}
				bo.Reset()
			})
			if os.IsNotExist(err) {
				slog.Debug("file not found", "file", fileTail.FileId, "error", err)
			} else if err != nil {
				slog.Error("failed to read file", "file", fileTail.FileId, "error", err)
			} else if err == nil {
				// Update the offset in the FileTracker
				c.fileManager.UpdateOffset(fileTail.FileId, fileTail.Offset)

				// Save the current offset to the store if enabled
				if c.offsetDB != nil && c.cfg.StoreOffsets {
					fileInfo := c.fileManager.Get(fileTail.FileId)
					if fileInfo != nil {
						if err := c.offsetDB.Save(fileTail.FileId, c.cfg.FingerprintStrategy, fileInfo.Path, fileTail.Offset); err != nil {
							slog.Error("failed to save offset", "file", fileTail.FileId, "offset", fileTail.Offset, "error", err)
						} else {
							slog.Debug("saved offset", "file", fileTail.FileId, "path", fileInfo.Path, "offset", fileTail.Offset)
						}
					}
				}
			}

			c.scheduler.SetIdle(fileTail.FileId)
		}
	}
}

func NewCollector(cfg Config) (*Collector, error) {
	c := &Collector{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}

	// Initialize offset store if enabled
	if cfg.StoreOffsets {
		var err error
		c.offsetDB, err = store.NewSQLiteStore(cfg.DBPath)
		if err != nil {
			return nil, err
		}
	}

	c.scheduler = NewTailScheduler()

	c.fileManager = file_tracker.New()

	// If we have an offset store, load existing files and their offsets
	if c.offsetDB != nil && c.cfg.StoreOffsets {
		// We'll implement this in the watcher's callback function
		slog.Debug("offset store enabled, offsets will be loaded when files are discovered")
	}

	var err error
	config := watcher.DefaultConfig()
	config.PollInterval = cfg.PollInterval
	config.FileTracker = c.fileManager
	config.FingerprintStrategy = cfg.FingerprintStrategy
	config.FingerprintSize = cfg.FingerprintSize
	config.Include = cfg.Include
	config.Exclude = cfg.Exclude

	c.onLineFunc = cfg.OnLineFunc

	c.watcher, err = watcher.NewWatcher(
		config,
		func(id, path string) {
			// Initialize with offset 0
			offset := int64(0)

			// Try to load offset from store if available
			if c.offsetDB != nil {
				// Load by ID and strategy
				storedOffset, found, err := c.offsetDB.Load(id, c.cfg.FingerprintStrategy)
				if err != nil {
					slog.Error("failed to load offset", "file", id, "error", err)
				} else if found {
					offset = storedOffset
					slog.Debug("loaded offset from store", "file", id, "offset", offset)

					// Update the offset in the FileTracker
					// The file was just added by the watcher, so we need to update its offset
					c.fileManager.UpdateOffset(id, offset)
				}
			}

			fileTail := tailer.TailReader{
				FileId:      id,
				Offset:      offset,
				Separator:   c.cfg.Separator,
				FileManager: c.fileManager,
			}
			slog.Debug("file added", "file", id, "path", path, "offset", offset)
			c.scheduler.Add(id, &fileTail, false)
		},
		func(id string) {
			// Remove from scheduler
			c.scheduler.Remove(id)

			// Delete offset from store if available
			if c.offsetDB != nil && c.cfg.StoreOffsets {
				if err := c.offsetDB.Delete(id, c.cfg.FingerprintStrategy); err != nil {
					slog.Error("failed to delete offset", "file", id, "error", err)
				} else {
					slog.Debug("deleted offset", "file", id)
				}
			}
		})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Collector) Start() {
	// Start worker goroutines
	if c.cfg.WorkerCount > 0 {
		for i := 0; i < c.cfg.WorkerCount; i++ {
			c.workerWg.Add(1)
			go c.worker()
		}
	}

	// Start the watcher
	c.watcher.Start()
}

func (c *Collector) Stop() {
	// Signal all workers to stop
	close(c.stopCh)

	// Wait for all workers to finish
	c.workerWg.Wait()

	// Stop the watcher
	c.watcher.Stop()

	// Close the offset store if it exists
	if c.offsetDB != nil {
		if err := c.offsetDB.Close(); err != nil {
			slog.Error("failed to close offset store", "error", err)
		}
	}
}
