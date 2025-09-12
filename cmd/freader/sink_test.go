package main

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test that buildSink returns nil when disabled and error on invalid type
func TestBuildSink_DisabledAndInvalid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sink.Type = ""
	if s, err := buildSink(cfg); err != nil || s != nil {
		t.Fatalf("disabled sink: got s=%v err=%v, want nil,nil", s, err)
	}

	cfg2 := DefaultConfig()
	cfg2.Sink.Type = "does-not-exist"
	if s, err := buildSink(cfg2); err == nil || s != nil {
		t.Fatalf("invalid type: expected error, got s=%v err=%v", s, err)
	}
}

// Test console sink created via factory writes to stdout with batching
func TestBuildSink_ConsoleStdoutWrites(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sink.Type = "console"
	cfg.Sink.Console.Stream = "stdout"
	cfg.Sink.BatchSize = 2
	cfg.Sink.BatchInterval = 10 * time.Millisecond

	// redirect stdout
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

	s, err := buildSink(cfg)
	if err != nil {
		t.Fatalf("buildSink(console): %v", err)
	}
	defer func() { _ = s.Stop() }()

	s.Enqueue("c1")
	s.Enqueue("c2") // reach batch size triggers flush
	// give time for flush
	time.Sleep(50 * time.Millisecond)
	_ = w.Close()
	data, _ := io.ReadAll(r)
	out := string(data)
	if out != "c1\nc2\n" {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

// Test file sink via factory writes to the target file and Stop flushes remaining lines
func TestBuildSink_FileWritesAndStopFlush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")

	cfg := DefaultConfig()
	cfg.Sink.Type = "file"
	cfg.Sink.File.Path = path
	cfg.Sink.BatchSize = 100 // large so Stop triggers the flush, not size
	cfg.Sink.BatchInterval = time.Hour

	s, err := buildSink(cfg)
	if err != nil {
		t.Fatalf("buildSink(file): %v", err)
	}

	// enqueue a few lines, then stop to force flush
	s.Enqueue("f1")
	s.Enqueue("f2")
	// allow background goroutine to receive from channel
	time.Sleep(30 * time.Millisecond)
	_ = s.Stop()

	// read file content
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open out: %v", err)
	}
	defer func() { _ = f.Close() }()
	b, _ := io.ReadAll(f)
	got := string(b)
	if strings.TrimSpace(got) != "f1\nf2" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

// Test include/exclude filters integration: only included lines are written
func TestBuildSink_FiltersAffectOutput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sink.Type = "console"
	cfg.Sink.Console.Stream = "stdout"
	cfg.Sink.BatchSize = 5
	cfg.Sink.BatchInterval = 10 * time.Millisecond
	cfg.Sink.Include = []string{"keep"}
	cfg.Sink.Exclude = []string{"drop"}

	// capture stdout
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = orig
		_ = r.Close()
		_ = w.Close()
	}()

	s, err := buildSink(cfg)
	if err != nil {
		t.Fatalf("buildSink(console): %v", err)
	}
	defer func() { _ = s.Stop() }()

	// Lines: only those containing "keep" and not containing "drop" should pass
	for _, ln := range []string{"noise", "keep A", "keep and drop", "drop only", "keep B"} {
		s.Enqueue(ln)
	}
	// let timer flush
	time.Sleep(30 * time.Millisecond)
	_ = w.Close()
	br := bufio.NewReader(r)
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			lines = append(lines, strings.TrimSuffix(line, "\n"))
		}
		if err != nil {
			break
		}
	}
	joined := strings.Join(lines, "|")
	if joined != "keep A|keep B" {
		t.Fatalf("unexpected filtered output: %q", joined)
	}
}
