package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// getMetric returns the value of a metric by its fully-qualified name from gathered families.
func getMetric(mfs []*dto.MetricFamily, name string) float64 {
	for _, mf := range mfs {
		if mf.GetName() == name {
			// counters/gauges here are unlabelled, take the first
			if len(mf.Metric) > 0 {
				m := mf.Metric[0]
				if mf.GetType() == dto.MetricType_COUNTER {
					return m.GetCounter().GetValue()
				}
				if mf.GetType() == dto.MetricType_GAUGE {
					return m.GetGauge().GetValue()
				}
			}
		}
	}
	return 0
}

func TestRegisterAndCounters(t *testing.T) {
	reg := prometheus.NewRegistry()

	// First registration should succeed
	if err := Register(reg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	// Second registration should be idempotent (ignore AlreadyRegistered)
	if err := Register(reg); err != nil {
		t.Fatalf("Register (second) failed: %v", err)
	}

	// Capture baseline values (collectors are globals; use deltas for assertions)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	baseLines := getMetric(mfs, "freader_lines_total")
	baseBytes := getMetric(mfs, "freader_bytes_total")
	baseErrors := getMetric(mfs, "freader_errors_total")
	baseFilesSeen := getMetric(mfs, "freader_files_seen_total")
	baseActive := getMetric(mfs, "freader_active_files")
	baseRestored := getMetric(mfs, "freader_restored_offsets_total")

	// Perform updates
	IncLines(3)
	IncLines(0) // no-op
	AddBytes(10)
	AddBytes(-5) // no-op
	IncReadErrors()
	IncFilesSeen()
	IncActiveFiles()
	DecActiveFiles()
	IncRestoredOffsets()

	mfs2, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather 2 failed: %v", err)
	}

	if got := getMetric(mfs2, "freader_lines_total") - baseLines; got != 3 {
		t.Fatalf("lines_total delta = %v, want 3", got)
	}
	if got := getMetric(mfs2, "freader_bytes_total") - baseBytes; got != 10 {
		t.Fatalf("bytes_total delta = %v, want 10", got)
	}
	if got := getMetric(mfs2, "freader_errors_total") - baseErrors; got != 1 {
		t.Fatalf("errors_total delta = %v, want 1", got)
	}
	if got := getMetric(mfs2, "freader_files_seen_total") - baseFilesSeen; got != 1 {
		t.Fatalf("files_seen_total delta = %v, want 1", got)
	}
	if got := getMetric(mfs2, "freader_active_files") - baseActive; got != 0 { // inc then dec
		t.Fatalf("active_files delta = %v, want 0", got)
	}
	if got := getMetric(mfs2, "freader_restored_offsets_total") - baseRestored; got != 1 {
		t.Fatalf("restored_offsets_total delta = %v, want 1", got)
	}
}
