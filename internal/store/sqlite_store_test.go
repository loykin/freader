package store

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteStore(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Create a new store
	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	require.NotNil(t, store)
	defer func() { _ = store.Close() }()

	// Test saving and loading with checksum strategy
	t.Run("Save and load with checksum strategy", func(t *testing.T) {
		fileID := "checksum123"
		strategy := "checksum"
		path := "/path/to/file.log"
		offset := int64(1024)

		// Save the offset
		err := store.Save(fileID, strategy, path, offset)
		assert.NoError(t, err)

		// Load the offset
		loadedOffset, found, err := store.Load(fileID, strategy)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, offset, loadedOffset)
	})

	// Test saving and loading with deviceAndInode strategy
	t.Run("Save and load with deviceAndInode strategy", func(t *testing.T) {
		fileID := "dev:123-ino:456-btime:789"
		strategy := "deviceAndInode"
		path := "/path/to/another/file.log"
		offset := int64(2048)

		// Save the offset
		err := store.Save(fileID, strategy, path, offset)
		assert.NoError(t, err)

		// Load the offset
		loadedOffset, found, err := store.Load(fileID, strategy)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, offset, loadedOffset)
	})

	// Test deleting an offset
	t.Run("Delete offset", func(t *testing.T) {
		fileID := "delete123"
		strategy := "checksum"
		path := "/path/to/delete.log"
		offset := int64(3000)

		// Save the offset
		err := store.Save(fileID, strategy, path, offset)
		assert.NoError(t, err)

		// Verify it was saved
		loadedOffset, found, err := store.Load(fileID, strategy)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, offset, loadedOffset)

		// Delete the offset
		err = store.Delete(fileID, strategy)
		assert.NoError(t, err)

		// Verify it was deleted
		_, found, err = store.Load(fileID, strategy)
		assert.NoError(t, err)
		assert.False(t, found)
	})

	// Test updating an existing offset
	t.Run("Update existing offset", func(t *testing.T) {
		fileID := "update123"
		strategy := "checksum"
		path := "/path/to/update.log"

		// Save initial offset
		initialOffset := int64(1000)
		err := store.Save(fileID, strategy, path, initialOffset)
		assert.NoError(t, err)

		// Update with new offset
		newOffset := int64(2000)
		err = store.Save(fileID, strategy, path, newOffset)
		assert.NoError(t, err)

		// Load and verify updated offset
		loadedOffset, found, err := store.Load(fileID, strategy)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, newOffset, loadedOffset)
	})

	// Test loading non-existent offset
	t.Run("Load non-existent offset", func(t *testing.T) {
		fileID := "nonexistent"
		strategy := "checksum"

		// Load non-existent offset
		_, found, err := store.Load(fileID, strategy)
		assert.NoError(t, err)
		assert.False(t, found)
	})
}

func TestSQLiteStore_MultipleInstances(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "multi.db")

	// Create first store instance
	store1, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store1.Close() }()

	// Save data in first instance
	fileID := "multi123"
	strategy := "checksum"
	path := "/path/to/multi.log"
	offset := int64(5000)

	err = store1.Save(fileID, strategy, path, offset)
	assert.NoError(t, err)

	// Create second store instance pointing to the same database
	store2, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = store2.Close() }()

	// Verify second instance can read data written by first
	loadedOffset, found, err := store2.Load(fileID, strategy)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, offset, loadedOffset)
}

func TestSQLiteStore_Errors(t *testing.T) {
	// Test with invalid database path
	t.Run("Invalid database path", func(t *testing.T) {
		// Create a path that can't be written to
		var invalidPath string
		if runtime.GOOS == "windows" {
			// 없는 드라이브 문자를 써서 무조건 실패하게
			invalidPath = `Z:\definitely_nonexistent\test.db`
		} else {
			// 루트 밑이라 권한 때문에 실패
			invalidPath = "/nonexistent/directory/test.db"
		}

		// Attempt to create store with invalid path
		store, err := NewSQLiteStore(invalidPath)
		assert.Error(t, err)
		assert.Nil(t, store)
	})
}

// TestNewSQLiteStore_Initialization verifies that NewSQLiteStore creates missing
// directories, applies migrations, and sets recommended PRAGMAs.
func TestNewSQLiteStore_Initialization(t *testing.T) {
	// On Windows, some deeply nested temp paths can exceed MAX_PATH for older tooling;
	// keep the nesting shallow.
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "nested", "dir", "collector.db")

	st, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// Open the DB directly to inspect metadata/PRAGMAs.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite directly: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// 1) Migrations: offsets table exists
	var name string
	row := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='offsets'`)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("offsets table not found after migrations: %v", err)
	}
	if name != "offsets" {
		t.Fatalf("unexpected table name: %q", name)
	}

	// 2) Goose version table exists (custom name in code: freader_db_version)
	name = ""
	row = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='freader_db_version'`)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("goose version table not found: %v", err)
	}

	// 3) PRAGMA journal_mode should be WAL (best-effort; assert when not Windows)
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode;`).Scan(&mode); err == nil {
		// Some drivers return lowercase/uppercase; normalize.
		mode = strings.ToLower(mode)
		if mode != "wal" {
			// On Windows with certain filesystems, WAL can be disabled; do not fail hard there.
			if runtime.GOOS != "windows" {
				t.Fatalf("journal_mode = %q, want wal", mode)
			}
		}
	}

	// 4) PRAGMA busy_timeout: this setting is connection-local in many drivers.
	// Since we are opening a fresh handle here, it may return driver defaults (often 0).
	// Query it to ensure the PRAGMA is supported, but do not assert the value strictly.
	var timeout int
	if err := db.QueryRow(`PRAGMA busy_timeout;`).Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout failed: %v", err)
	}
}
