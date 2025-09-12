package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// helper to fetch counter value from a CounterVec with specified labels
func getCounterVecValue(t *testing.T, cv *prometheus.CounterVec, labels ...string) float64 {
	t.Helper()
	c, err := cv.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("failed to get counter with labels %v: %v", labels, err)
	}
	return testutil.ToFloat64(c)
}

// helper to fetch histogram metrics (count and sum) from a registry for a given name and label set
func getHistogramCountAndSum(t *testing.T, reg *prometheus.Registry, metricName string, wantLabels map[string]string) (count uint64, sum float64) {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			// match labels
			ok := true
			if wantLabels != nil {
				got := make(map[string]string)
				for _, lp := range m.GetLabel() {
					got[lp.GetName()] = lp.GetValue()
				}
				for k, v := range wantLabels {
					if got[k] != v {
						ok = false
						break
					}
				}
			}
			if ok {
				h := m.GetHistogram()
				if h == nil {
					t.Fatalf("metric %s is not a histogram", metricName)
				}
				return h.GetSampleCount(), h.GetSampleSum()
			}
		}
	}
	t.Fatalf("histogram %s with labels %v not found", metricName, wantLabels)
	return 0, 0
}

func TestRegister_Idempotent(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := Register(reg); err != nil {
		t.Fatalf("second register (idempotent) failed: %v", err)
	}
}

func TestSinkMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 1) Enqueued with empty sink defaults to "unknown"
	SinkEnqueued("")
	if got := getCounterVecValue(t, enqueuedTotal, "unknown"); got != 1 {
		t.Fatalf("enqueued_total{sink=unknown} = %v, want 1", got)
	}

	// 2) Dropped with empty sink and reason defaults to "unknown"
	SinkDropped("", "")
	SinkDropped("", "")
	if got := getCounterVecValue(t, droppedTotal, "unknown", "unknown"); got != 2 {
		t.Fatalf("dropped_total{sink=unknown,reason=unknown} = %v, want 2", got)
	}

	// 3) Flush observe with size>0 and success=true records batch size, duration, and flush_total
	SinkFlushObserve("sinkA", 10, 200*time.Millisecond, true)

	// Check flush_total counter for sinkA
	if got := getCounterVecValue(t, flushTotal, "sinkA"); got != 1 {
		t.Fatalf("flush_total{sink=sinkA} = %v, want 1", got)
	}

	// Check failures counter didn't increment
	if got := getCounterVecValue(t, flushFailuresTotal, "sinkA"); got != 0 {
		t.Fatalf("flush_failures_total{sink=sinkA} = %v, want 0", got)
	}

	// Check batch size histogram observed exactly once with some sum
	count, sum := getHistogramCountAndSum(t, reg, "freader_sink_flush_batch_size", map[string]string{"sink": "sinkA"})
	if count != 1 {
		t.Fatalf("flush_batch_size count = %d, want 1", count)
	}
	if sum < 10 || sum >= 11 { // one observation of 10
		t.Fatalf("flush_batch_size sum = %v, want around 10", sum)
	}

	// Check duration histogram observed at least once (exactly one so far)
	dCount, dSum := getHistogramCountAndSum(t, reg, "freader_sink_flush_duration_seconds", map[string]string{"sink": "sinkA"})
	if dCount != 1 {
		t.Fatalf("flush_duration_seconds count = %d, want 1", dCount)
	}
	if dSum < 0.1 || dSum > 1.0 { // ~0.2 seconds
		t.Fatalf("flush_duration_seconds sum = %v, want roughly 0.2s", dSum)
	}

	// 4) Flush observe with size=0 and success=false: no batch/flush_total inc, but duration and failures inc
	SinkFlushObserve("sinkA", 0, 50*time.Millisecond, false)

	if got := getCounterVecValue(t, flushTotal, "sinkA"); got != 1 {
		t.Fatalf("flush_total{sink=sinkA} after size=0 = %v, want remain 1", got)
	}
	if got := getCounterVecValue(t, flushFailuresTotal, "sinkA"); got != 1 {
		t.Fatalf("flush_failures_total{sink=sinkA} = %v, want 1", got)
	}

	// Duration histogram should have one more sample
	dCount2, dSum2 := getHistogramCountAndSum(t, reg, "freader_sink_flush_duration_seconds", map[string]string{"sink": "sinkA"})
	if dCount2 != dCount+1 {
		t.Fatalf("flush_duration_seconds count = %d, want %d", dCount2, dCount+1)
	}
	if dSum2 <= dSum {
		t.Fatalf("flush_duration_seconds sum did not increase: before=%v after=%v", dSum, dSum2)
	}
}
