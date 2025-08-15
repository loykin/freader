package file_tracker

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileTracker_New(t *testing.T) {
	tracker := New()
	assert.NotNil(t, tracker)
	assert.NotNil(t, tracker.info)
	assert.Empty(t, tracker.info)
}

func TestFileTracker_BasicOperations(t *testing.T) {
	tracker := New()

	t.Run("Add and Get with default offset", func(t *testing.T) {
		tracker.Add("file1", "/path/1", "checksum", 1024)
		file := tracker.Get("file1")

		assert.NotNil(t, file)
		assert.Equal(t, "/path/1", file.Path)
		assert.Equal(t, "checksum", file.FingerprintStrategy)
		assert.Equal(t, int64(1024), file.FingerprintSize)
		assert.Equal(t, int64(0), file.Offset, "Default offset should be 0")
	})

	t.Run("Add and Get with explicit offset", func(t *testing.T) {
		tracker.Add("file2", "/path/2", "checksum", 1024, 500)
		file := tracker.Get("file2")

		assert.NotNil(t, file)
		assert.Equal(t, "/path/2", file.Path)
		assert.Equal(t, "checksum", file.FingerprintStrategy)
		assert.Equal(t, int64(1024), file.FingerprintSize)
		assert.Equal(t, int64(500), file.Offset, "Offset should be set to 500")
	})

	t.Run("Update existing file", func(t *testing.T) {
		tracker.Add("file1", "/new/path", "deviceAndInode", 2048)
		file := tracker.Get("file1")

		assert.NotNil(t, file)
		assert.Equal(t, "/new/path", file.Path)
		assert.Equal(t, "deviceAndInode", file.FingerprintStrategy)
		assert.Equal(t, int64(2048), file.FingerprintSize)
	})

	t.Run("Get non-existent file", func(t *testing.T) {
		file := tracker.Get("nonexistent")
		assert.Nil(t, file)
	})

	t.Run("Remove", func(t *testing.T) {
		tracker.Remove("file1")
		file := tracker.Get("file1")
		assert.Nil(t, file)
	})

	t.Run("Remove non-existent file", func(t *testing.T) {
		tracker.Remove("nonexistent") // should not panic
	})

	t.Run("UpdateOffset", func(t *testing.T) {
		// Add a file with default offset (0)
		tracker.Add("offset-test", "/path/offset", "checksum", 1024)
		file := tracker.Get("offset-test")
		assert.NotNil(t, file)
		assert.Equal(t, int64(0), file.Offset)

		// Update the offset
		success := tracker.UpdateOffset("offset-test", 100)
		assert.True(t, success)

		// Verify the offset was updated
		file = tracker.Get("offset-test")
		assert.NotNil(t, file)
		assert.Equal(t, int64(100), file.Offset)

		// Try to update a non-existent file
		success = tracker.UpdateOffset("nonexistent", 200)
		assert.False(t, success)
	})
}

func TestFileTracker_GetAllFiles(t *testing.T) {
	tracker := New()

	testFiles := map[string]struct {
		path     string
		strategy string
		size     int64
	}{
		"file1": {"/path/1", "checksum", 1024},
		"file2": {"/path/2", "deviceAndInode", 2048},
		"file3": {"/path/3", "checksum", 4096},
	}

	for id, file := range testFiles {
		tracker.Add(id, file.path, file.strategy, file.size)
	}

	files := tracker.GetAllFiles()
	assert.Equal(t, len(testFiles), len(files))

	for id, expected := range testFiles {
		actual, exists := files[id]
		assert.True(t, exists)
		assert.Equal(t, expected.path, actual.Path)
		assert.Equal(t, expected.strategy, actual.FingerprintStrategy)
		assert.Equal(t, expected.size, actual.FingerprintSize)
	}
}

func TestFileTracker_ConcurrentAccess(t *testing.T) {
	tracker := New()
	var wg sync.WaitGroup
	iterations := 100

	// Concurrent addition test
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tracker.Add(string(rune('A'+id%26)), "/path", "checksum", 1024)
		}(i)
	}

	// Concurrent reading test
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = tracker.Get(string(rune('A' + id%26)))
		}(i)
	}

	// Concurrent deletion test
	for i := 0; i < iterations/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tracker.Remove(string(rune('A' + id%26)))
		}(i)
	}

	// Concurrent GetAllFiles call test
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			files := tracker.GetAllFiles()
			assert.NotNil(t, files)
		}()
	}

	wg.Wait()
}

func TestFileTracker_EdgeCases(t *testing.T) {
	tracker := New()

	t.Run("Empty values", func(t *testing.T) {
		tracker.Add("", "", "", 0)
		file := tracker.Get("")
		assert.NotNil(t, file)
		assert.Empty(t, file.Path)
		assert.Empty(t, file.FingerprintStrategy)
		assert.Zero(t, file.FingerprintSize)
	})

	t.Run("Negative size", func(t *testing.T) {
		tracker.Add("test", "/path", "checksum", -1)
		file := tracker.Get("test")
		assert.NotNil(t, file)
		assert.Equal(t, int64(-1), file.FingerprintSize)
	})

	t.Run("Map integrity", func(t *testing.T) {
		tracker = New()
		count := 10

		// Add files
		for i := 0; i < count; i++ {
			tracker.Add(string(rune('A'+i)), "/path", "checksum", 1024)
		}

		// Delete some files
		for i := 0; i < count/2; i++ {
			tracker.Remove(string(rune('A' + i)))
		}

		files := tracker.GetAllFiles()
		assert.Equal(t, count/2, len(files))
	})
}
