package collector

import (
	"freader/pkg/watcher"
	"time"
)

type Config struct {
	Paths               []string
	WorkerCount         int
	Separator           byte
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
	c.Separator = '\n'
	c.FingerprintStrategy = watcher.FingerprintStrategyDeviceAndInode
	c.DBPath = "collector.db"
	c.StoreOffsets = true
}

func (c *Config) SetDefaultFingerprint() {
	c.FingerprintStrategy = watcher.FingerprintStrategyChecksum
	c.FingerprintSize = watcher.DefaultFingerprintStrategySize
}
