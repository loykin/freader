package watcher

import (
	"time"

	"github.com/loykin/freader/internal/file_tracker"
)

type Config struct {
	PollInterval        time.Duration
	FingerprintStrategy string
	FingerprintSize     int
	Exclude             []string
	Include             []string
	FileTracker         *file_tracker.FileTracker
}

func DefaultConfig() Config {
	return Config{
		PollInterval:        2 * time.Second,
		FingerprintStrategy: FingerprintStrategyDeviceAndInode,
		FileTracker:         file_tracker.New(),
	}
}
