package collector

import (
	"time"

	"github.com/loykin/freader/internal/watcher"
)

type Config struct {
	WorkerCount         int
	Separator           string
	PollInterval        time.Duration
	FingerprintStrategy string
	FingerprintSize     int
	Include             []string
	Exclude             []string
	OnLineFunc          func(line string)
	DBPath              string
	StoreOffsets        bool
}

func (c *Config) Default() {
	c.WorkerCount = 1
	c.PollInterval = 100 * time.Millisecond
	c.Separator = "\n"
	c.FingerprintStrategy = watcher.FingerprintStrategyDeviceAndInode
	c.DBPath = "collector.db"
	c.StoreOffsets = true
}

func (c *Config) SetDefaultFingerprint() {
	c.FingerprintStrategy = watcher.FingerprintStrategyChecksum
	c.FingerprintSize = watcher.DefaultFingerprintStrategySize
}

// Validate checks the collector configuration and underlying watcher-related options.
func (c *Config) Validate() error {
	// Basic checks
	if len(c.Include) == 0 {
		// Allow empty includes; watcher can still run but will find nothing. Not a hard error.
		// If you want hard enforcement, uncomment the following line:
		// return errors.New("collector.include must not be empty")
	}
	// Build a watcher config to reuse its validation rules
	wc := watcher.Config{
		PollInterval:        c.PollInterval,
		FingerprintStrategy: c.FingerprintStrategy,
		FingerprintSize:     c.FingerprintSize,
		Include:             c.Include,
		Exclude:             c.Exclude,
		FileTracker:         nil, // set at runtime by NewCollector
		// For checksumSeperator strategy, watcher expects FingerprintSeperator to be the record separator
		FingerprintSeperator: c.Separator,
	}
	return wc.Validate()
}
