package collector

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loykin/freader/internal/watcher"

	"github.com/stretchr/testify/assert"
	_ "modernc.org/sqlite"
)

func TestCollector_Integration(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644)
	assert.NoError(t, err)

	t.Run("DeviceAndInode Strategy", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skip inode-based collector test on Windows")
		}
		var lines []string
		var mu sync.Mutex

		cfg := Config{
			Include:             []string{tempDir},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\n",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			OnLineFunc: func(line string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, line)
			},
		}
		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		collector.Start()
		time.Sleep(2 * time.Second) // Wait for file detection

		// Check existing lines
		mu.Lock()
		assert.Contains(t, lines, "line1")
		assert.Contains(t, lines, "line2")
		assert.Contains(t, lines, "line3")
		mu.Unlock()

		// Add new content
		f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("line4\nline5\n")
		assert.NoError(t, err)
		_ = f.Close()

		time.Sleep(2 * time.Second) // Wait for new content detection

		mu.Lock()
		assert.Contains(t, lines, "line4")
		assert.Contains(t, lines, "line5")
		mu.Unlock()

		collector.Stop()
	})

	t.Run("Checksum Strategy", func(t *testing.T) {
		// Create a large file over 1024 bytes
		bigContent := ""
		for i := 0; i < 100; i++ {
			bigContent += fmt.Sprintf("line%d - This line is made long enough to ensure the total file size exceeds 1024 bytes........................\n", i)
		}

		bigFile := filepath.Join(tempDir, "big.txt")
		err := os.WriteFile(bigFile, []byte(bigContent), 0644)
		assert.NoError(t, err)

		// Verify file size
		fileInfo, err := os.Stat(bigFile)
		assert.NoError(t, err)
		assert.Greater(t, fileInfo.Size(), int64(1024), "File size must be larger than 1024 bytes")

		var lines []string
		var mu sync.Mutex

		cfg := Config{
			Include:             []string{tempDir},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\n",
			FingerprintStrategy: watcher.FingerprintStrategyChecksum,
			FingerprintSize:     1024,
			OnLineFunc: func(line string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, line)
			},
		}

		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		collector.Start()
		time.Sleep(2 * time.Second)

		mu.Lock()
		foundLines := 0
		for i := 0; i < 100; i++ {
			expectedLine := fmt.Sprintf("line%d - ", i)
			for _, line := range lines {
				if strings.HasPrefix(line, expectedLine) {
					foundLines++
					break
				}
			}
		}
		assert.Greater(t, foundLines, 0, "Should read at least some lines")
		mu.Unlock()

		collector.Stop()
	})
}

func TestCollector_MultipleFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based collector tests on Windows")
	}
	tempDir := t.TempDir()

	// Create multiple files
	files := []string{"file1.txt", "file2.txt", "file3.txt"}
	for i, fname := range files {
		content := []byte(fmt.Sprintf("content%d-1\ncontent%d-2\n", i+1, i+1))
		err := os.WriteFile(filepath.Join(tempDir, fname), content, 0644)
		assert.NoError(t, err)
	}

	var lines []string
	var mu sync.Mutex

	cfg := Config{
		Include:             []string{tempDir},
		PollInterval:        100 * time.Millisecond,
		WorkerCount:         2,
		Separator:           "\n",
		FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
		OnLineFunc: func(line string) {
			mu.Lock()
			defer mu.Unlock()
			lines = append(lines, line)
		},
	}

	collector, err := NewCollector(cfg)
	assert.NoError(t, err)

	collector.Start()
	// Wait until all three files are read (expect 6 lines) or timeout
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		count := len(lines)
		mu.Unlock()
		if count >= 6 {
			break
		}
		select {
		case <-deadline:
			break
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	mu.Lock()
	assert.Contains(t, lines, "content1-1")
	assert.Contains(t, lines, "content2-1")
	assert.Contains(t, lines, "content3-1")
	assert.Contains(t, lines, "content1-2")
	assert.Contains(t, lines, "content2-2")
	assert.Contains(t, lines, "content3-2")
	mu.Unlock()

	collector.Stop()
}

func TestCollector_FileRemoval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based collector tests on Windows")
	}
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "remove.txt")
	err := os.WriteFile(testFile, []byte("content1\ncontent2\n"), 0644)
	assert.NoError(t, err)

	var lines []string
	var mu sync.Mutex

	cfg := Config{
		Include:             []string{tempDir},
		PollInterval:        100 * time.Millisecond,
		WorkerCount:         1,
		Separator:           "\n",
		FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
		OnLineFunc: func(line string) {
			mu.Lock()
			defer mu.Unlock()
			lines = append(lines, line)
		},
	}

	collector, err := NewCollector(cfg)
	assert.NoError(t, err)

	collector.Start()
	time.Sleep(2 * time.Second)

	// Remove file
	err = os.Remove(testFile)
	assert.NoError(t, err)

	time.Sleep(2 * time.Second)

	mu.Lock()
	assert.Contains(t, lines, "content1")
	assert.Contains(t, lines, "content2")
	assert.Equal(t, 2, len(lines))
	mu.Unlock()

	collector.Stop()
}

func TestCollector_ErrorCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based collector tests on Windows")
	}
	t.Run("Invalid Directory", func(t *testing.T) {
		cfg := Config{
			Include:             []string{"/nonexistent/path"},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
		}

		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		collector.Start()
		time.Sleep(200 * time.Millisecond)
		collector.Stop()
		// Should log errors but not panic
	})

	t.Run("Invalid Fingerprint Strategy", func(t *testing.T) {
		cfg := Config{
			Include:             []string{t.TempDir()},
			FingerprintStrategy: "invalid",
		}

		_, err := NewCollector(cfg)
		assert.Error(t, err)
	})

	t.Run("Checksum Strategy Size Validation", func(t *testing.T) {
		cfg := Config{
			Include:             []string{t.TempDir()},
			FingerprintStrategy: watcher.FingerprintStrategyChecksum,
			FingerprintSize:     0,
		}

		_, err := NewCollector(cfg)
		assert.Error(t, err)

	})
}

func TestCollector_FileRemovalCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based collector tests on Windows")
	}
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "cleanup_test.db")

	// Create test files
	testFile := filepath.Join(tempDir, "cleanup_test.txt")
	err := os.WriteFile(testFile, []byte("line1\nline2\n"), 0644)
	assert.NoError(t, err)

	// Create a collector
	var lines []string
	var mu sync.Mutex

	cfg := Config{
		Include:             []string{tempDir, "cleanup_test.txt"},
		PollInterval:        100 * time.Millisecond,
		WorkerCount:         1,
		Separator:           "\n",
		FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
		OnLineFunc: func(line string) {
			mu.Lock()
			defer mu.Unlock()
			lines = append(lines, line)
		},
		DBPath:       dbPath,
		StoreOffsets: true,
	}

	collector, err := NewCollector(cfg)
	assert.NoError(t, err)

	// Start the collector and let it process the file
	collector.Start()
	time.Sleep(1 * time.Second)

	// Verify the file was processed
	mu.Lock()
	assert.Contains(t, lines, "line1")
	assert.Contains(t, lines, "line2")
	mu.Unlock()

	// Remove the file to trigger offset deletion in the next scan
	err = os.Remove(testFile)
	assert.NoError(t, err)

	// Wait for the file to be detected as removed
	time.Sleep(2 * time.Second)

	// Stop the collector
	collector.Stop()

	// Verify the database exists
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)

	// Open the database directly to check if the offset was removed
	db, err := sql.Open("sqlite", dbPath)
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Count the number of records
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM offsets").Scan(&count)
	assert.NoError(t, err)

	// The count should be 0 since the watcher should have detected the file removal
	// and triggered the offset deletion
	assert.Equal(t, 0, count, "Watcher should have removed the offset when the file was deleted")
}

func TestCollector_OffsetPersistence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based collector tests on Windows")
	}
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "offset_test.db")

	// Create test file
	testFile := filepath.Join(tempDir, "offset_test.txt")
	initialContent := "line1\nline2\nline3\n"
	err := os.WriteFile(testFile, []byte(initialContent), 0644)
	assert.NoError(t, err)

	// First collector run - process the file and store offsets
	{
		var lines []string
		var mu sync.Mutex

		cfg := Config{
			Include:             []string{tempDir, "offset_test.txt"},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\n",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			OnLineFunc: func(line string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, line)
			},
			DBPath:       dbPath,
			StoreOffsets: true,
		}

		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		// Start the collector and let it process the file
		collector.Start()
		time.Sleep(1 * time.Second)

		// Verify the file was processed
		mu.Lock()
		assert.Contains(t, lines, "line1")
		assert.Contains(t, lines, "line2")
		assert.Contains(t, lines, "line3")
		mu.Unlock()

		// Stop the collector
		collector.Stop()

		// Verify the database exists and has an offset record
		db, err := sql.Open("sqlite", dbPath)
		assert.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM offsets").Scan(&count)
		assert.NoError(t, err)
		assert.Equal(t, 1, count, "Should have one offset record")

		_ = db.Close()
	}

	// Append more content to the file
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	assert.NoError(t, err)
	_, err = f.WriteString("line4\nline5\n")
	assert.NoError(t, err)
	_ = f.Close()

	// Second collector run - should start reading from the stored offset
	{
		var lines []string
		var mu sync.Mutex

		cfg := Config{
			Include:             []string{tempDir, "offset_test.txt"},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\n",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			OnLineFunc: func(line string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, line)
			},
			DBPath:       dbPath,
			StoreOffsets: true,
		}

		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		// Start the collector and let it process the file
		collector.Start()
		time.Sleep(2 * time.Second)

		// Stop the collector
		collector.Stop()

		fmt.Println(lines)
		// Verify only the new lines were processed (not the initial ones)
		mu.Lock()
		assert.Equal(t, 2, len(lines), "Should only read new lines")
		assert.Contains(t, lines, "line4")
		assert.Contains(t, lines, "line5")
		assert.NotContains(t, lines, "line1")
		assert.NotContains(t, lines, "line2")
		assert.NotContains(t, lines, "line3")
		mu.Unlock()
	}
}

func TestCollector_IncludeExcludeFilters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based collector tests on Windows")
	}
	tempDir := t.TempDir()

	// Create test files with different extensions
	files := []string{
		"log1.txt",
		"log2.log",
		"data.json",
		"config.yaml",
	}

	for _, filename := range files {
		filePath := filepath.Join(tempDir, filename)
		content := []byte(fmt.Sprintf("content in %s\n", filename))
		err := os.WriteFile(filePath, content, 0644)
		assert.NoError(t, err)
	}

	t.Run("Include only log files", func(t *testing.T) {
		var lines []string
		var mu sync.Mutex

		cfg := Config{
			Include:             []string{tempDir, "*.log"},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\n",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			OnLineFunc: func(line string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, line)
			},
		}

		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		collector.Start()
		time.Sleep(2 * time.Second)

		mu.Lock()
		foundLog2 := false
		for _, line := range lines {
			if strings.Contains(line, "log2.log") {
				foundLog2 = true
			}
			// Should not find content from other files
			assert.NotContains(t, line, "log1.txt")
			assert.NotContains(t, line, "data.json")
			assert.NotContains(t, line, "config.yaml")
		}
		assert.True(t, foundLog2, "Should find content from log2.log")
		mu.Unlock()

		collector.Stop()
	})

	t.Run("Exclude log files", func(t *testing.T) {
		var lines []string
		var mu sync.Mutex

		cfg := Config{
			Include:             []string{tempDir},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\n",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			Exclude:             []string{"*.log"},
			OnLineFunc: func(line string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, line)
			},
		}

		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		collector.Start()
		time.Sleep(2 * time.Second)

		mu.Lock()
		foundTxt := false
		foundJson := false
		foundYaml := false
		for _, line := range lines {
			if strings.Contains(line, "log1.txt") {
				foundTxt = true
			}
			if strings.Contains(line, "data.json") {
				foundJson = true
			}
			if strings.Contains(line, "config.yaml") {
				foundYaml = true
			}
			// Should not find content from log files
			assert.NotContains(t, line, "log2.log")
		}
		assert.True(t, foundTxt, "Should find content from log1.txt")
		assert.True(t, foundJson, "Should find content from data.json")
		assert.True(t, foundYaml, "Should find content from config.yaml")
		mu.Unlock()

		collector.Stop()
	})

	t.Run("Include txt and log, exclude json", func(t *testing.T) {
		var lines []string
		var mu sync.Mutex

		cfg := Config{
			Include:             []string{tempDir, "*.txt", "*.log"},
			PollInterval:        100 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\n",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			Exclude:             []string{"*.json"},
			OnLineFunc: func(line string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, line)
			},
		}

		collector, err := NewCollector(cfg)
		assert.NoError(t, err)

		collector.Start()
		time.Sleep(2 * time.Second)

		mu.Lock()
		foundTxt := false
		foundLog := false
		for _, line := range lines {
			if strings.Contains(line, "log1.txt") {
				foundTxt = true
			}
			if strings.Contains(line, "log2.log") {
				foundLog = true
			}
			// Should not find content from excluded files
			assert.NotContains(t, line, "data.json")
			assert.NotContains(t, line, "config.yaml")
		}
		assert.True(t, foundTxt, "Should find content from log1.txt")
		assert.True(t, foundLog, "Should find content from log2.log")
		mu.Unlock()

		collector.Stop()
	})
}

func TestCollector_SeparatorsAndRestart_NoLoss(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based collector tests on Windows")
	}
	base := t.TempDir()

	// Subtest: CRLF separator "\r\n"
	t.Run("CRLF separator", func(t *testing.T) {
		p := filepath.Join(base, "crlf.log")
		// initial 2 lines with CRLF
		assert.NoError(t, os.WriteFile(p, []byte("a\r\nb\r\n"), 0644))

		var mu sync.Mutex
		var lines []string
		cfg := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\r\n",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			OnLineFunc: func(s string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, s)
			},
		}
		c, err := NewCollector(cfg)
		assert.NoError(t, err)
		c.Start()
		defer c.Stop()

		// wait for initial lines
		deadline := time.After(2 * time.Second)
		for {
			mu.Lock()
			n := len(lines)
			mu.Unlock()
			if n >= 2 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for initial CRLF lines: got %d", n)
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		// append another CRLF-terminated line
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("c\r\n")
		assert.NoError(t, err)
		_ = f.Close()

		deadline = time.After(2 * time.Second)
		for {
			mu.Lock()
			n := len(lines)
			mu.Unlock()
			if n >= 3 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for appended CRLF line")
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		mu.Lock()
		assert.Equal(t, []string{"a", "b", "c"}, lines)
		mu.Unlock()
	})

	// Subtest: custom token separator "<END>" and restart with partial trailing token
	t.Run("Custom token and restart no-loss", func(t *testing.T) {
		p := filepath.Join(base, "token.log")
		// Write content where the last token is NOT terminated by <END>
		initial := "part1<END>part2<END>part3"
		assert.NoError(t, os.WriteFile(p, []byte(initial), 0644))

		dbPath := filepath.Join(base, "token_offsets.db")
		var mu1 sync.Mutex
		var got1 []string
		cfg1 := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "<END>",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			OnLineFunc: func(s string) {
				mu1.Lock()
				defer mu1.Unlock()
				got1 = append(got1, s)
			},
			DBPath:       dbPath,
			StoreOffsets: true,
		}
		c1, err := NewCollector(cfg1)
		assert.NoError(t, err)
		c1.Start()

		// Wait for the first two tokens to be read
		deadline := time.After(2 * time.Second)
		for {
			mu1.Lock()
			n := len(got1)
			mu1.Unlock()
			if n >= 2 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for first run tokens, got %d", n)
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}
		c1.Stop()

		// Now append the missing terminator and an extra token
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("<END>part4<END>")
		assert.NoError(t, err)
		_ = f.Close()

		var mu2 sync.Mutex
		var got2 []string
		cfg2 := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "<END>",
			FingerprintStrategy: watcher.FingerprintStrategyDeviceAndInode,
			OnLineFunc: func(s string) {
				mu2.Lock()
				defer mu2.Unlock()
				got2 = append(got2, s)
			},
			DBPath:       dbPath,
			StoreOffsets: true,
		}
		c2, err := NewCollector(cfg2)
		assert.NoError(t, err)
		c2.Start()
		defer c2.Stop()

		deadline = time.After(3 * time.Second)
		for {
			mu2.Lock()
			n := len(got2)
			mu2.Unlock()
			if n >= 2 { // should read part3 and part4 only
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for second run tokens")
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		mu1.Lock()
		assert.Equal(t, []string{"part1", "part2"}, got1)
		mu1.Unlock()
		mu2.Lock()
		assert.Equal(t, []string{"part3", "part4"}, got2)
		mu2.Unlock()
	})
}

func TestCollector_Checksum_Restart_NoLoss_WithPartial(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "chk.log")
	// Start with two full tokens and one partial (no terminator)
	initial := "a<END>b<END>c"
	assert.NoError(t, os.WriteFile(p, []byte(initial), 0644))

	dbPath := filepath.Join(base, "chk_offsets.db")
	var mu1 sync.Mutex
	var got1 []string
	cfg1 := Config{
		Include:             []string{p},
		PollInterval:        50 * time.Millisecond,
		WorkerCount:         1,
		Separator:           "<END>",
		FingerprintStrategy: watcher.FingerprintStrategyChecksum,
		FingerprintSize:     8, // small enough so registration works immediately
		OnLineFunc: func(s string) {
			mu1.Lock()
			defer mu1.Unlock()
			got1 = append(got1, s)
		},
		DBPath:       dbPath,
		StoreOffsets: true,
	}
	c1, err := NewCollector(cfg1)
	assert.NoError(t, err)
	c1.Start()

	deadline := time.After(2 * time.Second)
	for {
		mu1.Lock()
		n := len(got1)
		mu1.Unlock()
		if n >= 2 { // a, b
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for first checksum run tokens")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	c1.Stop()

	// Complete the last partial token and add another
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
	assert.NoError(t, err)
	_, err = f.WriteString("<END>d<END>")
	assert.NoError(t, err)
	_ = f.Close()

	var mu2 sync.Mutex
	var got2 []string
	cfg2 := Config{
		Include:             []string{p},
		PollInterval:        50 * time.Millisecond,
		WorkerCount:         1,
		Separator:           "<END>",
		FingerprintStrategy: watcher.FingerprintStrategyChecksum,
		FingerprintSize:     8,
		OnLineFunc: func(s string) {
			mu2.Lock()
			defer mu2.Unlock()
			got2 = append(got2, s)
		},
		DBPath:       dbPath,
		StoreOffsets: true,
	}
	c2, err := NewCollector(cfg2)
	assert.NoError(t, err)
	c2.Start()
	defer c2.Stop()

	deadline = time.After(3 * time.Second)
	for {
		mu2.Lock()
		n := len(got2)
		mu2.Unlock()
		if n >= 2 { // c, d
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for second checksum run tokens")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	mu1.Lock()
	assert.Equal(t, []string{"a", "b"}, got1)
	mu1.Unlock()
	mu2.Lock()
	assert.Equal(t, []string{"c", "d"}, got2)
	mu2.Unlock()
}

func TestCollector_Checksum_SeparatorVariants_WindowsFriendly(t *testing.T) {
	base := t.TempDir()

	// CRLF separator with checksum strategy
	t.Run("CRLF checksum", func(t *testing.T) {
		p := filepath.Join(base, "crlf_checksum.log")
		// initial 2 lines with CRLF
		assert.NoError(t, os.WriteFile(p, []byte("a\r\nb\r\n"), 0644))

		var mu sync.Mutex
		var lines []string
		cfg := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\r\n",
			FingerprintStrategy: watcher.FingerprintStrategyChecksum,
			FingerprintSize:     2, // small to allow immediate registration (file is small)
			OnLineFunc: func(s string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, s)
			},
		}
		c, err := NewCollector(cfg)
		assert.NoError(t, err)
		c.Start()
		defer c.Stop()

		deadline := time.After(2 * time.Second)
		for {
			mu.Lock()
			n := len(lines)
			mu.Unlock()
			if n >= 2 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for initial CRLF lines: got %d", n)
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		// append another CRLF-terminated line
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("c\r\n")
		assert.NoError(t, err)
		_ = f.Close()

		deadline = time.After(2 * time.Second)
		for {
			mu.Lock()
			n := len(lines)
			mu.Unlock()
			if n >= 3 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for appended CRLF line")
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		mu.Lock()
		assert.Equal(t, []string{"a", "b", "c"}, lines)
		mu.Unlock()
	})

	// Custom token separator with checksum (no restart)
	t.Run("Custom token checksum", func(t *testing.T) {
		p := filepath.Join(base, "token_checksum.log")
		content := "part1<END>part2<END>part3<END>"
		assert.NoError(t, os.WriteFile(p, []byte(content), 0644))

		var mu sync.Mutex
		var items []string
		cfg := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "<END>",
			FingerprintStrategy: watcher.FingerprintStrategyChecksum,
			FingerprintSize:     8,
			OnLineFunc: func(s string) {
				mu.Lock()
				defer mu.Unlock()
				items = append(items, s)
			},
		}
		c, err := NewCollector(cfg)
		assert.NoError(t, err)
		c.Start()
		defer c.Stop()

		deadline := time.After(2 * time.Second)
		for {
			mu.Lock()
			n := len(items)
			mu.Unlock()
			if n >= 3 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for token items: got %d", n)
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		mu.Lock()
		assert.Equal(t, []string{"part1", "part2", "part3"}, items)
		mu.Unlock()
	})
}

func TestCollector_ChecksumSeperator_VariantsAndRestart(t *testing.T) {
	base := t.TempDir()

	// CRLF variant (Windows-friendly)
	t.Run("CRLF checksumSeperator", func(t *testing.T) {
		p := filepath.Join(base, "crlf_sep.log")
		assert.NoError(t, os.WriteFile(p, []byte("a\r\nb\r\n"), 0644))

		var mu sync.Mutex
		var lines []string
		cfg := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "\r\n",
			FingerprintStrategy: watcher.FingerprintStrategyChecksumSeperator,
			FingerprintSize:     2, // N separators to include in fingerprint
			OnLineFunc: func(s string) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, s)
			},
		}
		c, err := NewCollector(cfg)
		assert.NoError(t, err)
		c.Start()
		defer c.Stop()

		deadline := time.After(2 * time.Second)
		for {
			mu.Lock()
			n := len(lines)
			mu.Unlock()
			if n >= 2 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for initial CRLF lines: got %d", n)
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("c\r\n")
		assert.NoError(t, err)
		_ = f.Close()

		deadline = time.After(2 * time.Second)
		for {
			mu.Lock()
			n := len(lines)
			mu.Unlock()
			if n >= 3 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for appended CRLF line")
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		mu.Lock()
		assert.Equal(t, []string{"a", "b", "c"}, lines)
		mu.Unlock()
	})

	// Restart with partial using custom token <END>
	t.Run("ChecksumSeperator restart no-loss", func(t *testing.T) {
		p := filepath.Join(base, "token_sep.log")
		initial := "a<END>b<END>c"
		assert.NoError(t, os.WriteFile(p, []byte(initial), 0644))

		dbPath := filepath.Join(base, "token_sep_offsets.db")
		var mu1 sync.Mutex
		var got1 []string
		cfg1 := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "<END>",
			FingerprintStrategy: watcher.FingerprintStrategyChecksumSeperator,
			FingerprintSize:     2, // need two separators to fingerprint
			OnLineFunc: func(s string) {
				mu1.Lock()
				defer mu1.Unlock()
				got1 = append(got1, s)
			},
			DBPath:       dbPath,
			StoreOffsets: true,
		}
		c1, err := NewCollector(cfg1)
		assert.NoError(t, err)
		c1.Start()

		deadline := time.After(2 * time.Second)
		for {
			mu1.Lock()
			n := len(got1)
			mu1.Unlock()
			if n >= 2 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for first run tokens")
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}
		c1.Stop()

		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("<END>d<END>")
		assert.NoError(t, err)
		_ = f.Close()

		var mu2 sync.Mutex
		var got2 []string
		cfg2 := Config{
			Include:             []string{p},
			PollInterval:        50 * time.Millisecond,
			WorkerCount:         1,
			Separator:           "<END>",
			FingerprintStrategy: watcher.FingerprintStrategyChecksumSeperator,
			FingerprintSize:     2,
			OnLineFunc: func(s string) {
				mu2.Lock()
				defer mu2.Unlock()
				got2 = append(got2, s)
			},
			DBPath:       dbPath,
			StoreOffsets: true,
		}
		c2, err := NewCollector(cfg2)
		assert.NoError(t, err)
		c2.Start()
		defer c2.Stop()

		deadline = time.After(3 * time.Second)
		for {
			mu2.Lock()
			n := len(got2)
			mu2.Unlock()
			if n >= 2 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timeout waiting for second run tokens")
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}

		mu1.Lock()
		assert.Equal(t, []string{"a", "b"}, got1)
		mu1.Unlock()
		mu2.Lock()
		assert.Equal(t, []string{"c", "d"}, got2)
		mu2.Unlock()
	})
}
