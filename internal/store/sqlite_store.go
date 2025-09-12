package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Store interface defines methods for saving and loading file offsets
// with support for different fingerprint strategies
type Store interface {
	// Save stores the offset for a file identified by its ID and strategy
	Save(fileID string, strategy string, path string, offset int64) error

	// Load retrieves the offset for a file identified by its ID and strategy
	Load(fileID string, strategy string) (int64, bool, error)

	// Delete removes the offset for a file identified by its ID and strategy
	Delete(fileID string, strategy string) error

	// Close closes the store and releases any resources
	Close() error
}

type sqliteStore struct {
	db *sql.DB
}

// isBusyError returns true if error indicates SQLITE_BUSY
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite surfaces busy as an error string containing SQLITE_BUSY
	return strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked")
}

// execWithRetry executes a statement with args, retrying on SQLITE_BUSY up to a small limit.
func (s *sqliteStore) execWithRetry(query string, args ...any) (sql.Result, error) {
	var (
		res sql.Result
		err error
	)
	for attempt := 0; attempt < 5; attempt++ {
		res, err = s.db.Exec(query, args...)
		if err == nil {
			return res, nil
		}
		if !isBusyError(err) {
			return nil, err
		}
		// backoff a bit and retry
		time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
	}
	return nil, err
}

// NewSQLiteStore creates a new SQLite-based store with migrations
func NewSQLiteStore(dbPath string) (Store, error) {
	// Ensure directory exists
	if dir := filepath.Dir(dbPath); dir != "" {
		if err := ensureDir(dir); err != nil {
			return nil, fmt.Errorf("failed to create directory for database: %w", err)
		}
	}

	// Open database connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Improve concurrency/robustness: set busy timeout and WAL mode
	// Ignore errors from pragmas; they are best-effort and platform/driver dependent
	_, _ = db.Exec("PRAGMA busy_timeout = 2000")
	_, _ = db.Exec("PRAGMA journal_mode = WAL")

	// Set up goose with embedded migrations
	InitMigrations()

	// Run migrations
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set dialect: %w", err)
	}

	goose.SetTableName("freader_db_version")

	if err := goose.Up(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) Save(fileID string, strategy string, path string, offset int64) error {
	_, err := s.execWithRetry(
		`INSERT INTO offsets (id, strategy, path, offset, updated_at) 
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP) 
		 ON CONFLICT(id, strategy) DO UPDATE SET 
		 offset = excluded.offset, 
		 path = excluded.path,
		 updated_at = CURRENT_TIMESTAMP`,
		fileID, strategy, path, offset)

	if err != nil {
		return fmt.Errorf("failed to save offset: %w", err)
	}

	return nil
}

func (s *sqliteStore) Load(fileID string, strategy string) (int64, bool, error) {
	row := s.db.QueryRow(
		`SELECT offset FROM offsets WHERE id = ? AND strategy = ?`,
		fileID, strategy)

	var offset int64
	if err := row.Scan(&offset); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to load offset: %w", err)
	}

	return offset, true, nil
}

func (s *sqliteStore) Delete(fileID string, strategy string) error {
	_, err := s.execWithRetry(
		`DELETE FROM offsets WHERE id = ? AND strategy = ?`,
		fileID, strategy)

	if err != nil {
		return fmt.Errorf("failed to delete offset: %w", err)
	}

	return nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

// ensureDir makes sure a directory exists
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}
