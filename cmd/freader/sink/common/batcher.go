package common

import (
	"log/slog"
	"sync"
	"time"
)

// Batcher provides buffering, timing, and stop coordination for sinks.
type Batcher struct {
	Ch            chan string
	BatchSize     int
	BatchInterval time.Duration
	filter        *filter
	Wg            sync.WaitGroup
	StopOnce      sync.Once
	StopCh        chan struct{}
}

func NewBatcher(size int, interval time.Duration, includes, excludes []string) Batcher {
	return Batcher{
		Ch:            make(chan string, size*2),
		BatchSize:     size,
		BatchInterval: interval,
		filter:        &filter{includes: includes, excludes: excludes},
		StopCh:        make(chan struct{}),
	}
}

func (b *Batcher) Enqueue(line string) {
	if !b.filter.allow(line) {
		return
	}
	select {
	case b.Ch <- line:
	default:
		// buffer full, drop with a warning to avoid blocking file ingestion
		slog.Warn("sink buffer full; dropping line")
	}
}
