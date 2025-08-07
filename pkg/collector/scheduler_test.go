package collector

import (
	"fmt"
	"freader/pkg/file_tracker"
	"freader/pkg/tailer"
	"sync"
	"testing"
	"time"
)

func TestTailScheduler_Comprehensive(t *testing.T) {
	t.Run("Initialization Test", func(t *testing.T) {
		scheduler := NewTailScheduler()
		if scheduler.available.Len() != 0 {
			t.Error("Initial available list should be empty")
		}
		if len(scheduler.running) != 0 {
			t.Error("Initial running map should be empty")
		}
		if len(scheduler.index) != 0 {
			t.Error("Initial index map should be empty")
		}
		if scheduler.cursor != nil {
			t.Error("Initial cursor should be nil")
		}
	})

	t.Run("Basic Add/Remove Test", func(t *testing.T) {
		scheduler := NewTailScheduler()
		fm := file_tracker.New()
		file := &tailer.TailReader{FileId: "test1", FileManager: fm}

		// Add file
		scheduler.Add("test1", file, false)
		if scheduler.available.Len() != 1 {
			t.Error("File was not added")
		}
		if scheduler.cursor == nil {
			t.Error("Cursor was not set")
		}

		// Remove file
		scheduler.Remove("test1")
		if scheduler.available.Len() != 0 {
			t.Error("File was not removed")
		}
		if len(scheduler.index) != 0 {
			t.Error("File was not removed from index")
		}
		if scheduler.running["test1"] {
			t.Error("Running state was not cleared")
		}
	})

	t.Run("Round Robin Traversal Test", func(t *testing.T) {
		scheduler := NewTailScheduler()
		fm := file_tracker.New()
		files := []*tailer.TailReader{
			{FileId: "test1", FileManager: fm},
			{FileId: "test2", FileManager: fm},
			{FileId: "test3", FileManager: fm},
		}

		// Add files
		for _, f := range files {
			scheduler.Add(f.FileId, f, false)
		}

		// Get files sequentially
		expectedOrder := []string{"test1", "test2", "test3", "test1"}
		for _, expected := range expectedOrder {
			file, ok := scheduler.getNextAvailable()
			if !ok {
				t.Errorf("Failed to get file %s", expected)
				continue
			}
			if file.FileId != expected {
				t.Errorf("Incorrect order: expected %s, got %s", expected, file.FileId)
			}
			// Set to idle for next iteration
			scheduler.SetIdle(file.FileId)
		}
	})

	t.Run("Running State Management Test", func(t *testing.T) {
		scheduler := NewTailScheduler()
		fm := file_tracker.New()
		file := &tailer.TailReader{FileId: "test1", FileManager: fm}

		scheduler.Add("test1", file, false)

		// First execution
		file1, ok1 := scheduler.getNextAvailable()
		if !ok1 || file1.FileId != "test1" {
			t.Error("Failed to get file")
		}

		// Try to get the file again while it's running
		file2, ok2 := scheduler.getNextAvailable()
		if ok2 || file2 != nil {
			t.Error("Should not be able to get a running file")
		}

		// Set to idle state
		scheduler.SetIdle("test1")

		// Get the file again
		file3, ok3 := scheduler.getNextAvailable()
		if !ok3 || file3.FileId != "test1" {
			t.Error("Failed to get idle file")
		}
	})

	t.Run("Cursor Management Test", func(t *testing.T) {
		scheduler := NewTailScheduler()
		fm := file_tracker.New()

		// Set cursor when adding a file
		file1 := &tailer.TailReader{FileId: "test1", FileManager: fm}
		scheduler.Add("test1", file1, false)
		if scheduler.cursor == nil {
			t.Error("Cursor not set after adding first file")
		}

		// Remove the file that cursor points to
		currentFile := scheduler.cursor.Value.(*tailer.TailReader)
		scheduler.Remove(currentFile.FileId)
		if scheduler.cursor != nil {
			t.Error("Cursor should be nil after removing the last file")
		}
	})

	t.Run("Concurrency Safety Test", func(t *testing.T) {
		scheduler := NewTailScheduler()
		fm := file_tracker.New()
		var wg sync.WaitGroup

		// Add 100 files concurrently
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				file := &tailer.TailReader{FileId: id, FileManager: fm}
				scheduler.Add(id, file, false)
			}(fmt.Sprintf("test%d", i))
		}

		// Get files concurrently
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				scheduler.getNextAvailable()
			}()
		}

		// Set files to idle concurrently
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				scheduler.SetIdle(id)
			}(fmt.Sprintf("test%d", i))
		}

		wg.Wait()
		time.Sleep(100 * time.Millisecond) // Wait for all operations to complete

		if scheduler.available.Len() != 100 {
			t.Errorf("Unexpected number of files: expected 100, got %d", scheduler.available.Len())
		}
	})

	t.Run("Non-existent File Handling Test", func(t *testing.T) {
		scheduler := NewTailScheduler()

		// Try to remove a non-existent file
		scheduler.Remove("nonexistent")

		// Try to set a non-existent file to idle
		scheduler.SetIdle("nonexistent")

		// State should not change
		if scheduler.available.Len() != 0 {
			t.Error("State changed after handling non-existent file")
		}
	})
}
