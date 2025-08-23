package tailer

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/loykin/freader/internal/file_tracker"
	"github.com/loykin/freader/internal/watcher"

	"github.com/stretchr/testify/assert"
)

func TestTailReader_SeparatorsAndRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based tailer tests on Windows")
	}
	baseDir := t.TempDir()

	// Helper to create tracker and fileId
	newTracker := func(path string) (*file_tracker.FileTracker, string) {
		fi, err := os.Stat(path)
		assert.NoError(t, err)
		id, err := file_tracker.GetFileID(fi)
		assert.NoError(t, err)
		tr := file_tracker.New()
		tr.Add(id, path, watcher.FingerprintStrategyDeviceAndInode, 0)
		return tr, id
	}

	t.Run("CRLF separator \\r\\n", func(t *testing.T) {
		p := filepath.Join(baseDir, "crlf.txt")
		// 3 lines terminated with CRLF
		err := os.WriteFile(p, []byte("a\r\nb\r\nc\r\n"), 0644)
		assert.NoError(t, err)
		tr, id := newTracker(p)

		reader := &TailReader{FileId: id, FileManager: tr, Separator: "\r\n"}
		var lines []string
		err = reader.ReadOnce(func(s string) { lines = append(lines, s) })
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, lines)
		// Offset should equal total bytes including separators
		assert.Equal(t, int64(len([]byte("a\r\nb\r\nc\r\n"))), reader.Offset)

		// Restart from stored offset: should read nothing new
		reader2 := &TailReader{FileId: id, FileManager: tr, Separator: "\r\n", Offset: reader.Offset}
		var lines2 []string
		err = reader2.ReadOnce(func(s string) { lines2 = append(lines2, s) })
		assert.NoError(t, err)
		assert.Empty(t, lines2)
	})

	t.Run("Custom token separator <END>", func(t *testing.T) {
		p := filepath.Join(baseDir, "token.txt")
		content := "part1<END>part2<END>part3<END>"
		assert.NoError(t, os.WriteFile(p, []byte(content), 0644))
		tr, id := newTracker(p)

		reader := &TailReader{FileId: id, FileManager: tr, Separator: "<END>"}
		var items []string
		err := reader.ReadOnce(func(s string) { items = append(items, s) })
		assert.NoError(t, err)
		assert.Equal(t, []string{"part1", "part2", "part3"}, items)
		assert.Equal(t, int64(len(content)), reader.Offset)

		// Append more data and simulate restart with restored offset
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("part4<END>part5<END>")
		assert.NoError(t, err)
		_ = f.Close()

		tr2, id2 := newTracker(p)
		reader2 := &TailReader{FileId: id2, FileManager: tr2, Separator: "<END>", Offset: reader.Offset}
		var items2 []string
		err = reader2.ReadOnce(func(s string) { items2 = append(items2, s) })
		assert.NoError(t, err)
		assert.Equal(t, []string{"part4", "part5"}, items2)
	})
}

func TestTailReader_Integration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based tailer tests on Windows")
	}
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
			Separator:   "\n",
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
			Separator:   "\n",
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
			Separator:   "\n",
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
			Separator:   "\n",
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
	if runtime.GOOS == "windows" {
		t.Skip("skip inode-based tailer tests on Windows")
	}
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
		Separator:   "\n",
	}

	err = reader.open()
	assert.NoError(t, err)
	assert.NotNil(t, reader.file)
	assert.NotNil(t, reader.reader)

	reader.cleanup()
	assert.Nil(t, reader.file)
	assert.Nil(t, reader.reader)
}

func TestTailReader_OpenWithChecksum_SuccessAndMismatch(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "chk.txt")
	// create content longer than 16 bytes
	content := []byte("line1\nline2\nline3\n")
	assert.NoError(t, os.WriteFile(p, content, 0644))

	// compute expected fingerprint for first 16 bytes
	const size = 16
	fp, err := file_tracker.GetFileFingerprintFromPath(p, size)
	assert.NoError(t, err)
	assert.NotEmpty(t, fp)

	// create tracker and register with checksum strategy
	tr := file_tracker.New()
	tr.Add(fp, p, watcher.FingerprintStrategyChecksum, int64(size), 0)

	// success path: correct id
	reader := &TailReader{FileId: fp, FileManager: tr, Separator: "\n"}
	var lines []string
	err = reader.ReadOnce(func(s string) { lines = append(lines, s) })
	assert.NoError(t, err)
	// Should read all full lines available
	assert.Equal(t, []string{"line1", "line2", "line3"}, lines)

	// mismatch path: wrong id should error on open()
	wrongId := "deadbeef"
	tr2 := file_tracker.New()
	tr2.Add(wrongId, p, watcher.FingerprintStrategyChecksum, int64(size), 0)
	reader2 := &TailReader{FileId: wrongId, FileManager: tr2, Separator: "\n"}
	err = reader2.ReadOnce(func(s string) {})
	assert.Error(t, err)
}
