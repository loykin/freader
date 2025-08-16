package common

import (
	"log/slog"
	"sync"
	"time"

	cmdmetrics "github.com/loykin/freader/cmd/freader/metrics"
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
	Sink          string
}

func NewBatcher(size int, interval time.Duration, includes, excludes []string, sink string) Batcher {
	return Batcher{
		Ch:            make(chan string, size*2),
		BatchSize:     size,
		BatchInterval: interval,
		filter:        &filter{includes: includes, excludes: excludes},
		StopCh:        make(chan struct{}),
		Sink:          sink,
	}
}

func (b *Batcher) Enqueue(line string) {
	if !b.filter.allow(line) {
		cmdmetrics.SinkDropped(b.Sink, "filtered")
		return
	}
	select {
	case b.Ch <- line:
		cmdmetrics.SinkEnqueued(b.Sink)
	default:
		// buffer full, drop with a warning to avoid blocking file ingestion
		slog.Warn("sink buffer full; dropping line")
		cmdmetrics.SinkDropped(b.Sink, "buffer_full")
	}
}
