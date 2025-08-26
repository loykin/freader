package tailer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMultilineReader_ContinueThrough(t *testing.T) {
	m := &MultilineReader{Mode: MultilineReaderModeContinueThrough, ConditionPattern: "^\\s", StartPattern: "^(ERROR|INFO)", Timeout: time.Second}
	lines := []string{
		"ERROR start",
		"  detail1",
		"  detail2",
		"INFO ok",
		"  cont",
	}
	var out []string
	for _, l := range lines {
		assert.NoError(t, m.Write([]byte(l)))
		for {
			rec, err := m.Read()
			if err != nil {
				break
			}
			out = append(out, string(rec))
		}
	}
	// flush remaining
	m.Flush()
	for {
		rec, err := m.Read()
		if err != nil {
			break
		}
		out = append(out, string(rec))
	}
	assert.Equal(t, []string{
		"ERROR start\n  detail1\n  detail2",
		"INFO ok\n  cont",
	}, out)
}

func TestMultilineReader_ContinuePast(t *testing.T) {
	m := &MultilineReader{Mode: MultilineReaderModeContinuePast, ConditionPattern: "^\\s", StartPattern: "^(ERROR|INFO)", Timeout: time.Second}
	lines := []string{
		"ERROR start",
		"  detail1",
		"  detail2",
		"INFO ok",
		"  cont",
	}
	var out []string
	for _, l := range lines {
		assert.NoError(t, m.Write([]byte(l)))
		for {
			rec, err := m.Read()
			if err != nil {
				break
			}
			out = append(out, string(rec))
		}
	}
	m.Flush()
	for {
		rec, err := m.Read()
		if err != nil {
			break
		}
		out = append(out, string(rec))
	}
	// In continuePast, the first non-matching line (INFO ok) is included with the previous record and emitted.
	assert.Equal(t, []string{
		"ERROR start\n  detail1\n  detail2\nINFO ok",
		"  cont",
	}, out)
}

func TestMultilineReader_HaltBefore(t *testing.T) {
	m := &MultilineReader{Mode: MultilineReaderModeHaltBefore, ConditionPattern: "^(INFO|ERROR)", StartPattern: "^(ERROR|INFO)", Timeout: time.Second}
	lines := []string{
		"ERROR start",
		"  detail1",
		"  detail2",
		"INFO ok",
		"  cont",
	}
	var out []string
	for _, l := range lines {
		assert.NoError(t, m.Write([]byte(l)))
		for {
			rec, err := m.Read()
			if err != nil {
				break
			}
			out = append(out, string(rec))
		}
	}
	m.Flush()
	for {
		rec, err := m.Read()
		if err != nil {
			break
		}
		out = append(out, string(rec))
	}
	assert.Equal(t, []string{
		"ERROR start\n  detail1\n  detail2",
		"INFO ok\n  cont",
	}, out)
}

func TestMultilineReader_HaltWith(t *testing.T) {
	m := &MultilineReader{Mode: MultilineReaderModeHaltWith, ConditionPattern: "^(INFO|ERROR)", StartPattern: "^(ERROR|INFO)", Timeout: time.Second}
	lines := []string{
		"ERROR start",
		"  detail1",
		"  detail2",
		"INFO ok",
		"  cont",
	}
	var out []string
	for _, l := range lines {
		assert.NoError(t, m.Write([]byte(l)))
		for {
			rec, err := m.Read()
			if err != nil {
				break
			}
			out = append(out, string(rec))
		}
	}
	m.Flush()
	for {
		rec, err := m.Read()
		if err != nil {
			break
		}
		out = append(out, string(rec))
	}
	// INFO ok is included with previous and emitted, continuation line starts its own record
	assert.Equal(t, []string{
		"ERROR start\n  detail1\n  detail2\nINFO ok",
		"  cont",
	}, out)
}

// StartPattern should determine where grouping begins; ConditionPattern controls continuation.
func TestMultilineReader_StartPattern_WithContinuation(t *testing.T) {
	m := &MultilineReader{
		Mode:             MultilineReaderModeContinueThrough,
		StartPattern:     "^(ERROR|INFO)",
		ConditionPattern: "^\\s",
		Timeout:          time.Second,
	}
	lines := []string{
		"DEBUG ignore",
		"ERROR start",
		"  detail1",
		"  detail2",
		"INFO ok",
		"  cont",
	}
	var out []string
	for _, l := range lines {
		assert.NoError(t, m.Write([]byte(l)))
		for {
			rec, err := m.Read()
			if err != nil {
				break
			}
			out = append(out, string(rec))
		}
	}
	m.Flush()
	for {
		rec, err := m.Read()
		if err != nil {
			break
		}
		out = append(out, string(rec))
	}
	assert.Equal(t, []string{
		"DEBUG ignore",
		"ERROR start\n  detail1\n  detail2",
		"INFO ok\n  cont",
	}, out)
}

// Java stack trace style: start line with level or Exception, continuations with whitespace, "at ", or "Caused by:".
func TestMultilineReader_JavaStackTrace_Grouping(t *testing.T) {
	m := &MultilineReader{
		Mode:             MultilineReaderModeContinueThrough,
		StartPattern:     "^(ERROR|WARN|INFO|Exception)",
		ConditionPattern: "^(\\s|at\\s|Caused by:)",
		Timeout:          200 * time.Millisecond,
	}
	lines := []string{
		"ERROR Something failed",
		"    at com.example.App.main(App.java:10)",
		"Caused by: java.lang.IllegalStateException: bad",
		"    at com.example.Service.call(Service.java:42)",
		"INFO next record",
		"    at com.example.Other.run(Other.java:5)",
	}
	var out []string
	for _, l := range lines {
		assert.NoError(t, m.Write([]byte(l)))
		for {
			rec, err := m.Read()
			if err != nil {
				break
			}
			out = append(out, string(rec))
		}
	}
	m.Flush()
	for {
		rec, err := m.Read()
		if err != nil {
			break
		}
		out = append(out, string(rec))
	}
	assert.Equal(t, []string{
		"ERROR Something failed\n    at com.example.App.main(App.java:10)\nCaused by: java.lang.IllegalStateException: bad\n    at com.example.Service.call(Service.java:42)",
		"INFO next record\n    at com.example.Other.run(Other.java:5)",
	}, out)
}

// Channel/timeout test: after inactivity longer than Timeout, the current buffer is emitted to Recv().
func TestMultilineReader_ChannelTimeout(t *testing.T) {
	m := &MultilineReader{
		Mode:             MultilineReaderModeContinueThrough,
		StartPattern:     "^(ERROR|INFO)",
		ConditionPattern: "^\\s",
		Timeout:          50 * time.Millisecond,
	}
	ch := m.Recv()
	defer m.Close()

	// Write a start and one continuation, then wait past Timeout
	assert.NoError(t, m.Write([]byte("ERROR start")))
	assert.NoError(t, m.Write([]byte("  detail1")))
	// Wait for timeout flush
	time.Sleep(120 * time.Millisecond)

	// Expect a record on channel
	select {
	case rec := <-ch:
		assert.Equal(t, "ERROR start\n  detail1", string(rec))
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("did not receive timeout-flushed record")
	}
}
