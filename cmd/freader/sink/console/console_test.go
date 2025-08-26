package console

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/loykin/freader/cmd/freader/sink/common"
)

func TestStdoutSink_FlushOnBatchSize(t *testing.T) {
	var out bytes.Buffer
	// construct internal sink directly to inject writer
	s := &stdoutSink{batcher: common.NewBatcher(2, time.Hour, nil, nil, "console"), w: &out}
	s.start()
	defer func() { _ = s.Stop() }()

	s.Enqueue("line1")
	s.Enqueue("line2") // triggers immediate flush due to batch size

	// allow goroutine to run
	time.Sleep(50 * time.Millisecond)

	got := out.String()
	if got != "line1\nline2\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestStdoutSink_FlushOnStop(t *testing.T) {
	var out bytes.Buffer
	s := &stdoutSink{batcher: common.NewBatcher(100, time.Hour, nil, nil, "console"), w: &out}
	s.start()

	s.Enqueue("only-one")
	// give goroutine time to pull from channel into buffer, then stop should flush
	time.Sleep(30 * time.Millisecond)
	_ = s.Stop()

	got := out.String()
	if got != "only-one\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestConsoleNew_WritesToStdout(t *testing.T) {
	// Redirect os.Stdout
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = orig
		_ = r.Close()
		_ = w.Close()
	}()

	sink := New("stdout", 2, 10*time.Millisecond, nil, nil)
	defer func() { _ = sink.Stop() }()

	// Reach batch size to force flush
	sink.Enqueue("A")
	sink.Enqueue("B")

	// give it time to flush then close writer and read
	time.Sleep(50 * time.Millisecond)
	_ = w.Close()
	data, _ := io.ReadAll(r)
	out := string(data)
	if out != "A\nB\n" {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestConsoleNew_WritesToStderr(t *testing.T) {
	// Redirect os.Stderr
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = orig
		_ = r.Close()
		_ = w.Close()
	}()

	sink := New("stderr", 2, 10*time.Millisecond, nil, nil)
	defer func() { _ = sink.Stop() }()

	sink.Enqueue("X")
	sink.Enqueue("Y")

	time.Sleep(50 * time.Millisecond)
	_ = w.Close()
	data, _ := io.ReadAll(r)
	out := string(data)
	if out != "X\nY\n" {
		t.Fatalf("unexpected stderr: %q", out)
	}
}
