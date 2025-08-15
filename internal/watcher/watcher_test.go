package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loykin/freader/internal/file_tracker"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 2*time.Second, config.PollInterval)
	assert.Equal(t, FingerprintStrategyDeviceAndInode, config.FingerprintStrategy)
	assert.NotNil(t, config.FileTracker)
}

func TestNewWatcher_FingerprintStrategyValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid DeviceAndInode Strategy",
			config: Config{
				Include:             []string{"/tmp"},
				FingerprintStrategy: FingerprintStrategyDeviceAndInode,
				FileTracker:         file_tracker.New(),
			},
			expectError: false,
		},
		{
			name: "Valid Checksum Strategy",
			config: Config{
				Include:             []string{"/tmp"},
				FingerprintStrategy: FingerprintStrategyChecksum,
				FingerprintSize:     1024,
				FileTracker:         file_tracker.New(),
			},
			expectError: false,
		},
		{
			name: "Checksum Strategy with Invalid Size",
			config: Config{
				Include:             []string{"/tmp"},
				FingerprintStrategy: FingerprintStrategyChecksum,
				FingerprintSize:     0,
				FileTracker:         file_tracker.New(),
			},
			expectError: true,
			errorMsg:    "fingerprint size must be greater than 0",
		},
		{
			name: "Unsupported Strategy",
			config: Config{
				Include:             []string{"/tmp"},
				FingerprintStrategy: "invalid",
				FileTracker:         file_tracker.New(),
			},
			expectError: true,
			errorMsg:    "unsupported fingerprint strategy: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWatcher(tt.config, func(id, path string) {}, func(id string) {})

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Equal(t, tt.errorMsg, err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWatcher_StartStop(t *testing.T) {
	config := Config{
		Include:             []string{t.TempDir()},
		PollInterval:        10 * time.Millisecond,
		FingerprintStrategy: FingerprintStrategyDeviceAndInode,
		FileTracker:         file_tracker.New(),
	}

	w, err := NewWatcher(config,
		func(id, path string) {},
		func(id string) {},
	)
	assert.NoError(t, err)

	// Start watcher in background
	done := make(chan struct{})
	go func() {
		w.Start()
		close(done)
	}()

	// Wait for some scans to occur
	time.Sleep(50 * time.Millisecond)

	// Stop watcher
	w.Stop()

	// Ensure watcher has stopped
	select {
	case <-done:
		// Success: watcher stopped
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Watcher did not stop within the expected timeframe")
	}
}

func TestWatcher_PathValidation(t *testing.T) {
	tests := []struct {
		name        string
		paths       []string
		expectError bool
	}{
		{
			name:        "Single Valid Path",
			paths:       []string{"/tmp/logs"},
			expectError: false,
		},
		{
			name:        "Multiple Non-overlapping Paths",
			paths:       []string{"/tmp/logs", "/var/logs", "/opt/logs"},
			expectError: false,
		},
		{
			name:        "Overlapping Paths",
			paths:       []string{"/tmp/logs", "/tmp/logs/app"},
			expectError: true,
		},
		{
			name:        "Duplicate Paths",
			paths:       []string{"/tmp/logs", "/tmp/logs"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Include:             tt.paths,
				FingerprintStrategy: FingerprintStrategyDeviceAndInode,
				FileTracker:         file_tracker.New(),
			}

			_, err := NewWatcher(config, func(id, path string) {}, func(id string) {})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWatcher_IncludeExcludeFilters(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"log1.txt",
		"log2.log",
		"data.json",
		"config.yaml",
	}

	for _, filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte("test content"), 0644)
		assert.NoError(t, err)
	}

	tests := []struct {
		name            string
		include         []string
		exclude         []string
		expectedFiles   []string
		unexpectedFiles []string
	}{
		{
			name:            "Include only log files",
			include:         []string{"*.log"},
			exclude:         []string{},
			expectedFiles:   []string{"log2.log"},
			unexpectedFiles: []string{"log1.txt", "data.json", "config.yaml"},
		},
		{
			name:            "Exclude log files",
			include:         []string{},
			exclude:         []string{"*.log"},
			expectedFiles:   []string{"log1.txt", "data.json", "config.yaml"},
			unexpectedFiles: []string{"log2.log"},
		},
		{
			name:            "Include txt and log, exclude json",
			include:         []string{"*.txt", "*.log"},
			exclude:         []string{"*.json"},
			expectedFiles:   []string{"log1.txt", "log2.log"},
			unexpectedFiles: []string{"data.json", "config.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create channels to collect found files
			foundFiles := make(map[string]bool)
			fileChan := make(chan string)

			// Setup watcher
			config := Config{
				Include:             append([]string{tempDir}, tt.include...),
				PollInterval:        10 * time.Millisecond,
				FingerprintStrategy: FingerprintStrategyDeviceAndInode,
				Exclude:             tt.exclude,
				FileTracker:         file_tracker.New(),
			}

			// Create watcher with callback that records found files
			w, err := NewWatcher(config,
				func(id, path string) {
					fileChan <- filepath.Base(path)
				},
				func(id string) {},
			)
			assert.NoError(t, err)

			// Start watcher
			go w.Start()

			// Collect found files for a short period
			timeout := time.After(100 * time.Millisecond)
			for {
				select {
				case file := <-fileChan:
					foundFiles[file] = true
				case <-timeout:
					goto done
				}
			}

		done:
			// Stop watcher
			w.Stop()

			// Verify expected files were found
			for _, file := range tt.expectedFiles {
				assert.True(t, foundFiles[file], "Expected file %s was not found", file)
			}

			// Verify unexpected files were not found
			for _, file := range tt.unexpectedFiles {
				assert.False(t, foundFiles[file], "Unexpected file %s was found", file)
			}
		})
	}
}
