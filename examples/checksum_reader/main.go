// Example demonstrating how to use the checksum fingerprint strategy with the Collector
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/loykin/freader"
)

// This example reads bundled sample logs under ./examples/checksum_reader/log using
// the checksum fingerprint strategy and prints collected lines. It also appends a
// couple of extra lines to demonstrate live updates.
//
// Notes:
//   - FingerprintSize is small so small demo files are registered quickly.
//   - Separator is a string (default "\n"), but you could set multi-byte separators
//     such as "\r\n" or a custom token like "<END>".
func main() {
	// Verbose logger for demo
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Path to bundled sample logs (relative to repo root when running `go run ./examples/checksum_reader`)
	include := []string{"./examples/checksum_reader/log/*.log"}

	// Configure the collector for checksum tracking
	var cfg freader.Config
	cfg.Default()         // start from defaults
	cfg.Include = include // watch bundled logs
	cfg.Separator = "\n"  // line separator (can be "\r\n" or "<END>")
	cfg.WorkerCount = 1
	cfg.PollInterval = 100 * time.Millisecond
	cfg.FingerprintStrategy = freader.FingerprintStrategyChecksum
	cfg.FingerprintSize = 64 // small size so even small files qualify
	cfg.StoreOffsets = false // not needed for a short demo

	cfg.OnLineFunc = func(line string) {
		fmt.Printf("[COLLECTED] %s\n", line)
	}

	c, err := freader.NewCollector(cfg)
	if err != nil {
		slog.Error("failed to create collector", "error", err)
		os.Exit(1)
	}

	c.Start()
	defer c.Stop()

	// Give it a moment to discover and read existing bundled lines
	slog.Info("reading bundled sample logs...")
	time.Sleep(1500 * time.Millisecond)

	// Optionally, append a couple of lines to the first sample file to show live updates.
	// This is best-effort; if the file is not writable, we just skip appending.
	func() {
		path := "./examples/checksum_reader/log/sample.log"
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			slog.Warn("could not open bundled sample for append (skipping live demo)", "error", err)
			return
		}
		defer func() { _ = f.Close() }()
		_, _ = f.WriteString("127.0.0.1 - - [23/Aug/2025:22:32:10 +0900] \"GET /live-demo HTTP/1.1\" 200 123\n")
		_, _ = f.WriteString("127.0.0.1 - - [23/Aug/2025:22:32:11 +0900] \"GET /live-demo2 HTTP/1.1\" 200 456\n")
	}()

	// Allow time to collect appended lines (if appended)
	time.Sleep(2 * time.Second)

	slog.Info("checksum example completed")
}
