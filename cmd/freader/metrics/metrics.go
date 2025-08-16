package metrics

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Counters for enqueue and drops
	enqueuedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "freader",
			Subsystem: "sink",
			Name:      "enqueued_total",
			Help:      "Total number of lines enqueued to sink buffers.",
		},
		[]string{"sink"},
	)
	droppedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "freader",
			Subsystem: "sink",
			Name:      "dropped_total",
			Help:      "Total number of lines dropped before enqueue (filtered or buffer_full).",
		},
		[]string{"sink", "reason"},
	)
	flushTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "freader",
			Subsystem: "sink",
			Name:      "flush_total",
			Help:      "Total number of flush attempts with at least one record.",
		},
		[]string{"sink"},
	)
	flushFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "freader",
			Subsystem: "sink",
			Name:      "flush_failures_total",
			Help:      "Total number of failed flushes.",
		},
		[]string{"sink"},
	)
	batchSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "freader",
			Subsystem: "sink",
			Name:      "flush_batch_size",
			Help:      "Number of records per flush.",
			Buckets:   []float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000},
		},
		[]string{"sink"},
	)
	flushDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "freader",
			Subsystem: "sink",
			Name:      "flush_duration_seconds",
			Help:      "Duration of sink flush operations in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"sink"},
	)
)

// Register registers sink-related metrics to the provided Prometheus registerer.
// Safe to call multiple times; AlreadyRegistered is ignored.
func Register(r prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		enqueuedTotal, droppedTotal, flushTotal, flushFailuresTotal, batchSize, flushDuration,
	}
	for _, c := range collectors {
		if err := r.Register(c); err != nil {
			var already prometheus.AlreadyRegisteredError
			if errors.As(err, &already) {
				continue
			}
			return err
		}
	}
	return nil
}

// SinkEnqueued increments the enqueued counter for a sink.
func SinkEnqueued(sink string) {
	if sink == "" {
		sink = "unknown"
	}
	enqueuedTotal.WithLabelValues(sink).Inc()
}

// SinkDropped increments the dropped counter for a sink with a reason.
func SinkDropped(sink, reason string) {
	if sink == "" {
		sink = "unknown"
	}
	if reason == "" {
		reason = "unknown"
	}
	droppedTotal.WithLabelValues(sink, reason).Inc()
}

// SinkFlushObserve records a flush metrics set: batch size, duration, and success/failure counts.
func SinkFlushObserve(sink string, size int, dur time.Duration, success bool) {
	if sink == "" {
		sink = "unknown"
	}
	if size > 0 {
		batchSize.WithLabelValues(sink).Observe(float64(size))
		flushTotal.WithLabelValues(sink).Inc()
	}
	flushDuration.WithLabelValues(sink).Observe(dur.Seconds())
	if !success {
		flushFailuresTotal.WithLabelValues(sink).Inc()
	}
}
