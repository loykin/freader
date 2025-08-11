// Package main provides an embedded example of using the collector directly in your app
package main

import (
	"fmt"
	"github.com/loykin/freader/pkg/collector"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// This example shows how to embed and run the collector inside your own program.
// It watches ./log/*.log files, prints each collected line to stdout, and stores
// read offsets in a local SQLite DB (collector.db) so it can resume on restart.
func main() {
	// Configure logging for the demo
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Prepare collector configuration
	cfg := collector.Config{}
	cfg.Default()
	cfg.WorkerCount = 1
	cfg.PollInterval = 500 * time.Millisecond
	cfg.Include = []string{"./log/*.log"} // watch only .log files in ./log
	// Optionally exclude some files or patterns
	// cfg.Exclude = []string{"*.bak", "./log/ignore.log"}

	// Optional: switch fingerprinting strategy
	// By default Default() uses device+inode. To use checksum-based tracking:
	// cfg.SetDefaultFingerprint() // uses checksum with default size

	// Handle each line as it is read
	cfg.OnLineFunc = func(line string) {
		fmt.Printf("collector: %s\n", line)
	}

	// Create the collector
	c, err := collector.NewCollector(cfg)
	if err != nil {
		slog.Error("failed to create collector", "error", err)
		os.Exit(1)
	}

	// Start the collector
	c.Start()
	slog.Info("collector started; press Ctrl+C to stop")

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	slog.Info("stopping collector...")
	c.Stop()
	slog.Info("collector stopped")
}
