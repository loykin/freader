package file_tracker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetFileID_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// 1) Create new file
	file1 := filepath.Join(tmpDir, "file1.txt")
	content := []byte("hello world")
	if err := os.WriteFile(file1, content, 0644); err != nil {
		assert.NoError(t, err)
	}

	id1, err := GetFileIDFromPath(file1)
	assert.NoError(t, err)
	if id1 == "" {
		assert.Fail(t, "empty fileID")
	}

	// 2) Same file content, change name (rename)
	file1Renamed := filepath.Join(tmpDir, "file1_renamed.txt")
	if err := os.Rename(file1, file1Renamed); err != nil {
		assert.NoError(t, err)
	}

	id2, err := GetFileIDFromPath(file1Renamed)
	if err != nil {
		assert.NoError(t, err)
	}

	// On Linux/macOS, since it's inode-based, the id remains the same after rename
	if id1 != id2 {
		assert.Fail(t, "expected same fileID after rename, got different %s", id2)
	}

	// 3) Create a copy (expecting new inode)
	file1Copy := filepath.Join(tmpDir, "file1_copy.txt")
	src, err := os.Open(file1Renamed)
	assert.NoError(t, err)

	defer func() { _ = src.Close() }()
	dst, err := os.Create(file1Copy)
	assert.NoError(t, err)
	defer func() { _ = dst.Close() }()

	_, err = io.Copy(dst, src)
	assert.NoError(t, err)

	id3, err := GetFileIDFromPath(file1Copy)
	assert.NoError(t, err)

	if id3 == id1 {
		assert.Fail(t, "expected different fileID after copy, got same %s", id3)
	}

	// 4) Test deletion and recreation
	err = os.Remove(file1Copy)
	assert.NoError(t, err)

	err = os.WriteFile(file1Copy, content, 0644)
	assert.NoError(t, err)

	id4, err := GetFileIDFromPath(file1Copy)
	assert.NoError(t, err)

	if id4 == id3 {
		assert.Fail(t, "expected different fileID after deletion and recreation, got same %s", id4)
	}
}

func TestGetFileID_MultipleFilesAndModification(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		return
	case "windows":
		return
	}

	tmpDir := t.TempDir()
	err := os.MkdirAll(tmpDir, 0755)
	assert.NoError(t, err)

	count := 10000
	filePaths := make([]string, count)
	originalIDs := make([]string, count)

	// 1. File creation and ID collection
	for i := 0; i < count; i++ {
		filename := "test_file_" + strconv.Itoa(i) + ".log"
		filePath := filepath.Join(tmpDir, filename)
		content := []byte("initial content " + strconv.Itoa(i))
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("failed to create file %d: %v", i, err)
		}
		filePaths[i] = filePath

		id, err := GetFileIDFromPath(filePath)
		if err != nil {
			t.Fatalf("GetFileID error on creation %d: %v", i, err)
		}
		if originalIDs[i] != "" {
			assert.Fail(t, "fileID should be empty on creation, got %s", id)
		}
		originalIDs[i] = id
	}

	// 2. Check for duplicate IDs
	seen := make(map[string]struct{})
	for i, id := range originalIDs {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate ID found at index %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}

	// 3. Check if IDs remain the same after adding content to files
	for i, path := range filePaths {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("failed to open file %d: %v", i, err)
		}
		_, err = f.WriteString("\nappended line\n")
		_ = f.Close()
		if err != nil {
			t.Fatalf("failed to append to file %d: %v", i, err)
		}

		newID, err := GetFileIDFromPath(path)
		if err != nil {
			t.Fatalf("GetFileID after append error %d: %v", i, err)
		}
		if newID != originalIDs[i] {
			t.Errorf("file ID changed after append (file %d): %s → %s", i, originalIDs[i], newID)
		}
	}

	// 4. Delete files and recreate them → IDs should be different
	for i, path := range filePaths {
		err := os.Remove(path)
		if err != nil {
			t.Fatalf("failed to delete file %d: %v", i, err)
		}

		// Short sleep to reduce the probability of inode reuse
		time.Sleep(10 * time.Millisecond)

		// Recreate the file
		newContent := []byte("recreated content " + strconv.Itoa(i))
		if err := os.WriteFile(path, newContent, 0644); err != nil {
			t.Fatalf("failed to recreate file %d: %v", i, err)
		}

		newID, err := GetFileIDFromPath(path)
		if err != nil {
			t.Fatalf("GetFileID after recreate error %d: %v", i, err)
		}
		if newID == originalIDs[i] {
			t.Errorf("file ID should differ after deletion and recreation (file %d): %s", i, newID)
		}
	}

	// 5. Delete all files (cleanup)
	for _, path := range filePaths {
		_ = os.Remove(path) // Ignore failures
	}
}

func TestGetFileID_RepeatDeleteRecreate(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		return
	case "windows":
		return
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.log")

	// Map to store all generated IDs
	allIDs := make(map[string]int)
	iterations := 500

	for i := 0; i < iterations; i++ {
		// 1. Create file
		content := []byte(fmt.Sprintf("content-%d\n", i))
		err := os.WriteFile(filePath, content, 0644)
		assert.NoError(t, err)

		// 2. Get ID
		id, err := GetFileIDFromPath(filePath)
		assert.NoError(t, err)

		// 3. Check for duplicate IDs
		if count, exists := allIDs[id]; exists {
			t.Errorf("ID collision detected on iteration %d: ID %s was previously seen in iteration %d",
				i, id, count)
		}
		allIDs[id] = i

		// 4. Add content to the file
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
		assert.NoError(t, err)
		_, err = f.WriteString("additional content\n")
		_ = f.Close()
		assert.NoError(t, err)

		// 5. Check if ID remains the same after modification
		modifiedID, err := GetFileIDFromPath(filePath)
		assert.NoError(t, err)
		if modifiedID != id {
			t.Errorf("ID changed after modification in iteration %d: %s → %s",
				i, id, modifiedID)
		}

		// 6. Delete the file
		err = os.Remove(filePath)
		assert.NoError(t, err)

	}

	// Output statistics
	t.Logf("Total unique IDs: %d out of %d iterations", len(allIDs), iterations)
}
