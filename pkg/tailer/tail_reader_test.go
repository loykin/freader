package tailer

import (
	"freader/pkg/file_tracker"
	"freader/pkg/watcher"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTailReader_Integration(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644)
	assert.NoError(t, err)

	// Get the actual file ID
	fileInfo, err := os.Stat(testFile)
	assert.NoError(t, err)
	fileId, err := file_tracker.GetFileID(fileInfo)
	assert.NoError(t, err)

	tracker := file_tracker.New()
	tracker.Add(fileId, testFile, watcher.FingerprintStrategyDeviceAndInode, 0)

	t.Run("Basic file reading", func(t *testing.T) {
		reader := &TailReader{
			FileId:      fileId,
			FileManager: tracker,
			Separator:   '\n',
		}

		lines := make([]string, 0)
		err := reader.ReadOnce(func(line string) {
			lines = append(lines, line)
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{"line1", "line2", "line3"}, lines)
	})

	t.Run("Reading with offset", func(t *testing.T) {
		reader := &TailReader{
			FileId:      fileId,
			FileManager: tracker,
			Separator:   '\n',
			Offset:      6, // After "line1\n"
		}

		lines := make([]string, 0)
		err := reader.ReadOnce(func(line string) {
			lines = append(lines, line)
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{"line2", "line3"}, lines)
	})

	t.Run("Real-time file monitoring", func(t *testing.T) {
		reader := &TailReader{
			FileId:      fileId,
			FileManager: tracker,
			Separator:   '\n',
		}

		var wg sync.WaitGroup
		lines := make([]string, 0)
		var mu sync.Mutex

		wg.Add(1)
		go func() {
			defer wg.Done()
			reader.Run(func(line string) {
				mu.Lock()
				lines = append(lines, line)
				mu.Unlock()
			})
		}()

		// Add new content to the file
		time.Sleep(100 * time.Millisecond)
		f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("line4\nline5\n")
		assert.NoError(t, err)
		_ = f.Close()

		time.Sleep(1 * time.Second)
		reader.Stop()
		wg.Wait()

		mu.Lock()
		assert.Equal(t, []string{"line1", "line2", "line3", "line4", "line5"}, lines)
		mu.Unlock()
	})

	t.Run("Large file processing", func(t *testing.T) {
		largeFile := filepath.Join(tempDir, "large.txt")
		f, err := os.Create(largeFile)
		assert.NoError(t, err)

		// Create large file
		for i := 0; i < 1000; i++ {
			_, err = f.WriteString("large line content\n")
			assert.NoError(t, err)
		}
		_ = f.Close()

		// Get the ID of the large file
		largeInfo, err := os.Stat(largeFile)
		assert.NoError(t, err)
		largeId, err := file_tracker.GetFileID(largeInfo)
		assert.NoError(t, err)

		tracker.Add(largeId, largeFile, watcher.FingerprintStrategyDeviceAndInode, 0)

		reader := &TailReader{
			FileId:      largeId,
			FileManager: tracker,
			Separator:   '\n',
		}

		lineCount := 0
		err = reader.ReadOnce(func(string) {
			lineCount++
		})

		assert.NoError(t, err)
		assert.Equal(t, 1000, lineCount)
	})
}

func TestTailReader_Cleanup(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "cleanup.txt")
	err := os.WriteFile(testFile, []byte("test\n"), 0644)
	assert.NoError(t, err)

	fileInfo, err := os.Stat(testFile)
	assert.NoError(t, err)
	fileId, err := file_tracker.GetFileID(fileInfo)
	assert.NoError(t, err)

	tracker := file_tracker.New()
	tracker.Add(fileId, testFile, watcher.FingerprintStrategyDeviceAndInode, 0)

	reader := &TailReader{
		FileId:      fileId,
		FileManager: tracker,
		Separator:   '\n',
	}

	err = reader.open()
	assert.NoError(t, err)
	assert.NotNil(t, reader.file)
	assert.NotNil(t, reader.reader)

	reader.cleanup()
	assert.Nil(t, reader.file)
	assert.Nil(t, reader.reader)
}
