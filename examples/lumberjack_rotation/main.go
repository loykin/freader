// Example demonstrating freader collecting logs written via lumberjack,
// verifying that collection continues correctly across log rotation
// for BOTH fingerprint strategies: device+inode and checksum.
package main

import (
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loykin/freader"
	"gopkg.in/natefinch/lumberjack.v2"
)

// runScenario sets up a temporary workspace, writes logs with lumberjack,
// rotates twice, and confirms freader continues reading after each rotation.
func runScenario(name, strategy string) error {
	slog.Info("starting scenario", "name", name, "strategy", strategy)

	// Prepare temp workspace for this scenario
	tmpDir, err := os.MkdirTemp("", "freader_lj_"+name+"_")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	logPath := filepath.Join(tmpDir, "app.log")
	slog.Info("using log file", "path", logPath)

	// Configure lumberjack (rotation will be forced via Rotate())
	lj := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    5, // MB (not relevant because we'll force Rotate())
		MaxBackups: 5,
		MaxAge:     1, // days
		Compress:   false,
	}
	defer func() { _ = lj.Close() }()
	std := log.New(lj, "", log.LstdFlags|log.Lmicroseconds)

	// Collected lines & markers
	var mu sync.Mutex
	var lines []string
	seenStartup := false
	seenPostRotate1 := false
	seenPostRotate2 := false

	// Configure freader collector
	var cfg freader.Config
	cfg.Default()
	cfg.WorkerCount = 1
	cfg.PollInterval = 100 * time.Millisecond
	cfg.Include = []string{logPath} // watch this exact file path
	cfg.Separator = '\n'
	cfg.DBPath = filepath.Join(tmpDir, "collector.db")
	cfg.StoreOffsets = false
	cfg.FingerprintStrategy = strategy
	if strategy == freader.FingerprintStrategyChecksum {
		// Use a small fingerprint size so the file is registered before the first rotation,
		// ensuring early (startup) lines are captured in checksum mode as well.
		cfg.FingerprintSize = 64
	}
	cfg.OnLineFunc = func(line string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, line)
		if strings.Contains(line, "startup line") {
			seenStartup = true
		}
		if strings.Contains(line, "POST1") {
			seenPostRotate1 = true
		}
		if strings.Contains(line, "POST2") {
			seenPostRotate2 = true
		}
		fmt.Printf("[COLLECTED][%s] %s\n", name, line)
	}

	collector, err := freader.NewCollector(cfg)
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}
	collector.Start()
	defer collector.Stop()

	// Writer goroutine: initial lines, rotate, more lines, rotate again, more lines
	done := make(chan struct{})
	go func() {
		defer close(done)
		std.Println("startup line 1")
		std.Println("startup line 2")
		for i := 0; i < 10; i++ {
			std.Printf("PRE line %02d rand=%d\n", i+1, rand.Intn(100000))
			time.Sleep(70 * time.Millisecond)
		}

		slog.Info("forcing rotation #1", "scenario", name)
		_ = lj.Rotate()
		for i := 0; i < 15; i++ {
			std.Printf("POST1 line %02d rand=%d\n", i+1, rand.Intn(100000))
			time.Sleep(70 * time.Millisecond)
		}

		slog.Info("forcing rotation #2", "scenario", name)
		_ = lj.Rotate()
		for i := 0; i < 12; i++ {
			std.Printf("POST2 line %02d rand=%d\n", i+1, rand.Intn(100000))
			time.Sleep(70 * time.Millisecond)
		}
	}()

	// Let the scenario run for a bit longer to ensure reads after rotations
	select {
	case <-done:
		// writer finished; give reader some extra time to drain
		time.Sleep(1 * time.Second)
	case <-time.After(10 * time.Second):
		slog.Warn("writer timed out (continuing)")
	}

	// Basic validations
	mu.Lock()
	defer mu.Unlock()
	if !seenStartup {
		return fmt.Errorf("%s: did not collect startup lines", name)
	}
	if !seenPostRotate1 {
		return fmt.Errorf("%s: did not collect any POST1 lines after first rotation", name)
	}
	if !seenPostRotate2 {
		return fmt.Errorf("%s: did not collect any POST2 lines after second rotation", name)
	}

	slog.Info("scenario succeeded", "name", name, "strategy", strategy, "total_lines", len(lines))
	return nil
}

func main() {
	// Verbose logger to observe what's happening
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Run with device+inode
	if err := runScenario("device_inode", freader.FingerprintStrategyDeviceAndInode); err != nil {
		log.Fatalf("device+inode scenario failed: %v", err)
	}

	// Run with checksum
	if err := runScenario("checksum", freader.FingerprintStrategyChecksum); err != nil {
		log.Fatalf("checksum scenario failed: %v", err)
	}

	slog.Info("all scenarios completed successfully")
}
