package common

import (
	"testing"
	"time"
)

func drain(ch <-chan string, max int, timeout time.Duration) []string {
	out := []string{}
	deadline := time.After(timeout)
	for len(out) < max {
		select {
		case s := <-ch:
			out = append(out, s)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestBatcher_Enqueue_FilterAndBuffer(t *testing.T) {
	b := NewBatcher(10, 10*time.Millisecond, []string{"ok"}, []string{"drop"}, "test")
	// Enqueue three lines: one allowed, one filtered by include, one excluded by exclude
	b.Enqueue("ok-first")        // contains include
	b.Enqueue("no-include-here") // should be filtered
	b.Enqueue("ok-but-drop-tag") // contains include and exclude -> exclude wins

	// Drain up to 3 entries quickly
	got := drain(b.Ch, 3, 20*time.Millisecond)
	if len(got) != 1 || got[0] != "ok-first" {
		t.Fatalf("expected only the allowed line to be in channel, got %+v", got)
	}
}

func TestBatcher_BufferFullDrops(t *testing.T) {
	// BatchSize=1 => channel capacity = size*2 = 2
	b := NewBatcher(1, 10*time.Millisecond, nil, nil, "test")
	// Fill up to capacity
	b.Enqueue("a")
	b.Enqueue("b")
	// This third enqueue should hit default case and be dropped
	b.Enqueue("c")

	got := drain(b.Ch, 10, 20*time.Millisecond)
	if len(got) != 2 {
		t.Fatalf("expected 2 items in buffer, got %d: %+v", len(got), got)
	}
	if !(got[0] == "a" && got[1] == "b") {
		t.Fatalf("unexpected channel content: %+v", got)
	}
}
