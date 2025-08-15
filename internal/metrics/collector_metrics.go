package metrics

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	linesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "freader",
		Name:      "lines_total",
		Help:      "Total number of log lines processed.",
	})
	bytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "freader",
		Name:      "bytes_total",
		Help:      "Total number of bytes emitted from tailed files (approximate, excludes separators).",
	})
	errorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "freader",
		Name:      "errors_total",
		Help:      "Total number of read errors encountered while tailing files.",
	})
	activeFiles = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "freader",
		Name:      "active_files",
		Help:      "Current number of active files being tailed.",
	})
	filesSeenTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "freader",
		Name:      "files_seen_total",
		Help:      "Total number of files discovered by the watcher.",
	})
	restoredOffsetsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "freader",
		Name:      "restored_offsets_total",
		Help:      "Total number of files for which an offset was restored from the store upon discovery.",
	})
)

// Register registers all freader metrics to the provided Prometheus registerer.
// It is safe to call multiple times; AlreadyRegisteredError will be ignored.
func Register(r prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		linesTotal, bytesTotal, errorsTotal, activeFiles, filesSeenTotal, restoredOffsetsTotal,
	}
	for _, c := range collectors {
		if err := r.Register(c); err != nil {
			var alreadyRegisteredError prometheus.AlreadyRegisteredError
			if errors.As(err, &alreadyRegisteredError) {
				continue
			}
			return err
		}
	}
	return nil
}

// IncLines increments the processed lines counter by n.
func IncLines(n int) {
	if n > 0 {
		linesTotal.Add(float64(n))
	}
}

// AddBytes adds n to the bytes counter.
func AddBytes(n int) {
	if n > 0 {
		bytesTotal.Add(float64(n))
	}
}

// IncReadErrors increments the read errors counter by 1.
func IncReadErrors() { errorsTotal.Inc() }

// IncFilesSeen increments the files seen counter by 1.
func IncFilesSeen() { filesSeenTotal.Inc() }

// IncActiveFiles increments the active files gauge by 1.
func IncActiveFiles() { activeFiles.Inc() }

// DecActiveFiles decrements the active files gauge by 1.
func DecActiveFiles() { activeFiles.Dec() }

// IncRestoredOffsets increments the restored offsets counter by 1.
func IncRestoredOffsets() { restoredOffsetsTotal.Inc() }
