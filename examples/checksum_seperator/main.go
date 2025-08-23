// Example demonstrating how to use the checksumSeperator fingerprint strategy with the Collector
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/loykin/freader"
)

// This example reads bundled sample logs under ./examples/checksum_seperator/log using
// the checksumSeperator fingerprint strategy with a custom token separator ("<END>")
// and prints collected records. It then appends a couple extra records to demonstrate
// live updates.
//
// Notes:
//   - FingerprintSize here means the number of separators to include when computing
//     the checksum-based fingerprint (N-th separator is included in the hash).
//   - Separator is a string and can be multi-byte (e.g., "\r\n") or a token like "<END>".
func main() {
	// Verbose logger for demo
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Path to bundled sample tokens (relative to repo root when running `go run ./examples/checksum_seperator`)
	include := []string{"./examples/checksum_seperator/log/*.log"}

	// Configure the collector for checksumSeperator tracking
	var cfg freader.Config
	cfg.Default()           // start from defaults
	cfg.Include = include   // watch bundled logs
	cfg.Separator = "<END>" // use custom token separator
	cfg.WorkerCount = 1
	cfg.PollInterval = 100 * time.Millisecond
	cfg.FingerprintStrategy = freader.FingerprintStrategyChecksumSeperator
	cfg.FingerprintSize = 2  // compute fingerprint until 2nd occurrence of <END>
	cfg.StoreOffsets = false // not needed for a short demo

	cfg.OnLineFunc = func(rec string) {
		fmt.Printf("[COLLECTED] %s\n", rec)
	}

	c, err := freader.NewCollector(cfg)
	if err != nil {
		slog.Error("failed to create collector", "error", err)
		os.Exit(1)
	}

	c.Start()
	defer c.Stop()

	// Give it a moment to discover and read existing bundled lines
	slog.Info("reading bundled sample logs (checksumSeperator/<END>)...")
	time.Sleep(1500 * time.Millisecond)

	// Append a couple of records to the bundled sample to show live updates.
	func() {
		path := "./examples/checksum_seperator/log/sample.log"
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			slog.Warn("could not open bundled sample for append (skipping live demo)", "error", err)
			return
		}
		defer func() { _ = f.Close() }()
		_, _ = f.WriteString("live1<END>")
		_, _ = f.WriteString("live2<END>")
	}()

	// Allow time to collect appended lines (if appended)
	time.Sleep(2 * time.Second)

	slog.Info("checksumSeperator example completed")
}
