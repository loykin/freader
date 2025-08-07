// Package main provides a sample example of using the log reader functionality
package main

import (
	"fmt"
	"freader/pkg/file_tracker"
	"freader/pkg/tailer"
	"freader/pkg/watcher"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// This example demonstrates how to use the TailReader to read log files
// It shows two main use cases:
// 1. Reading a file once from beginning to end
// 2. Continuously monitoring a file for new content (like 'tail -f' command)
func main() {
	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// Create a temporary log file for demonstration
	tempDir, err := os.MkdirTemp("", "log_reader_example")
	if err != nil {
		slog.Error("Failed to create temp directory", "error", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	logFilePath := filepath.Join(tempDir, "sample.log")
	slog.Info("Created temporary log file", "path", logFilePath)

	// Write initial content to the log file
	err = os.WriteFile(logFilePath, []byte("Initial log line 1\nInitial log line 2\n"), 0644)
	if err != nil {
		slog.Error("Failed to write to log file", "error", err)
		os.Exit(1)
	}

	// Create a file tracker to manage the file information
	fileTracker := file_tracker.New()

	// Get the file ID for the log file
	fileId, err := file_tracker.GetFileIDFromPath(logFilePath)
	if err != nil {
		slog.Error("Failed to get file ID", "error", err)
		os.Exit(1)
	}

	// Add the file to the tracker
	// Using device and inode as the fingerprint strategy
	fileTracker.Add(fileId, logFilePath, watcher.FingerprintStrategyDeviceAndInode, 0)

	// Create a tail reader to read the log file
	// The tail reader will start reading from the beginning of the file (offset 0)
	reader := &tailer.TailReader{
		FileId:      fileId,
		Offset:      0,
		Separator:   '\n',
		FileManager: fileTracker,
	}

	// Example 1: Read the file once
	slog.Info("Example 1: Reading the file once")
	err = reader.ReadOnce(func(line string) {
		fmt.Printf("ReadOnce: %s\n", line)
	})
	if err != nil {
		slog.Error("Failed to read file", "error", err)
		os.Exit(1)
	}

	// Example 2: Continuously read the file as new content is added
	slog.Info("Example 2: Continuously reading the file")
	reader.Run(func(line string) {
		fmt.Printf("Continuous read: %s\n", line)
	})

	// Simulate adding new content to the log file
	go func() {
		time.Sleep(1 * time.Second)
		f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			slog.Error("Failed to open log file for writing", "error", err)
			return
		}
		defer func() { _ = f.Close() }()

		for i := 1; i <= 5; i++ {
			_, err = f.WriteString(fmt.Sprintf("New log line %d\n", i))
			if err != nil {
				slog.Error("Failed to write to log file", "error", err)
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	// Let the program run for a while to demonstrate continuous reading
	time.Sleep(5 * time.Second)

	// Stop and clean up
	reader.Stop()
	slog.Info("Log reader example completed")
}
