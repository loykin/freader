// Package freader provides a simplified, stable root-level API for external users.
//
// Instead of importing internal subpackages like "github.com/loykin/freader/pkg/collector",
// consumers can just:
//
//	import "github.com/loykin/freader"
//
// and then use freader.NewCollector and freader.Config directly.
package freader

import (
	"github.com/loykin/freader/internal/collector"
	"github.com/loykin/freader/internal/file_tracker"
	"github.com/loykin/freader/internal/metrics"
	"github.com/loykin/freader/internal/tailer"
	"github.com/loykin/freader/internal/watcher"
	"github.com/prometheus/client_golang/prometheus"
)

// Config re-exports collector.Config for convenient use from the module root.
// This is a type alias, so it's fully compatible with the underlying type.
type Config = collector.Config

// Collector re-exports collector.Collector so callers can keep the concrete type
// when using the root-level constructor.
type Collector = collector.Collector

// FileTracker re-exports file_tracker.FileTracker for root-level usage.
type FileTracker = file_tracker.FileTracker

// NewFileTracker constructs a new FileTracker.
func NewFileTracker() *FileTracker { return file_tracker.New() }

// GetFileIDFromPath re-exports the utility to compute a file ID from a path.
func GetFileIDFromPath(path string) (string, error) { return file_tracker.GetFileIDFromPath(path) }

// TailReader re-exports tailer.TailReader for root-level usage.
type TailReader = tailer.TailReader

// Fingerprint strategy constants re-exported for convenient configuration.
const (
	FingerprintStrategyChecksum       = watcher.FingerprintStrategyChecksum
	FingerprintStrategyDeviceAndInode = watcher.FingerprintStrategyDeviceAndInode
)

// NewCollector constructs a new Collector using the provided configuration.
// It is a thin wrapper around collector.NewCollector.
func NewCollector(cfg Config) (*Collector, error) {
	return collector.NewCollector(cfg)
}

// StartMetrics registers freader metrics on the default Prometheus registry and starts an HTTP server.
// It returns a stop function to gracefully shut down the metrics server.
func StartMetrics(addr string) (func() error, error) {
	if err := metrics.Register(prometheus.DefaultRegisterer); err != nil {
		return nil, err
	}
	srv, err := metrics.Start(addr)
	if err != nil {
		return nil, err
	}
	return srv.Stop, nil
}
