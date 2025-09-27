package watcher

import (
	"errors"
	"time"

	"github.com/loykin/freader/internal/file_tracker"
)

type Config struct {
	PollInterval         time.Duration
	FingerprintStrategy  string
	FingerprintSize      int
	FingerprintSeparator string
	Exclude              []string
	Include              []string
	FileTracker          *file_tracker.FileTracker
}

// Validate checks the configuration consistency according to the selected strategy.
func (c Config) Validate() error {
	switch c.FingerprintStrategy {
	case FingerprintStrategyDeviceAndInode:
		// no extra requirements
		return nil
	case FingerprintStrategyChecksum:
		if c.FingerprintSize <= 0 {
			return errors.New("fingerprint size must be greater than 0")
		}
		return nil
	case FingerprintStrategyChecksumSeparator:
		if c.FingerprintSize <= 0 {
			return errors.New("fingerprint size must be greater than 0")
		}
		if c.FingerprintSeparator == "" {
			return errors.New("fingerprint separator must be set for checksumSeparator strategy")
		}
		return nil
	default:
		return errors.New("unsupported fingerprint strategy: " + c.FingerprintStrategy)
	}
}

func DefaultConfig() Config {
	return Config{
		PollInterval:        2 * time.Second,
		FingerprintStrategy: FingerprintStrategyDeviceAndInode,
		FileTracker:         file_tracker.New(),
	}
}
