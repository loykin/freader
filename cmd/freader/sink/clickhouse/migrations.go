package clickhouse

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Expose embedded migration content for testing purposes.
func ReadEmbeddedMigration(name string) (string, error) {
	b, err := migrationFS.ReadFile("migrations/" + name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// runMigrations injects the configured table into embedded SQL and applies it via goose.
func runMigrations(opts *ch.Options, database, table string) error {
	// Open DB and ping
	db := ch.OpenDB(opts)
	defer func() { _ = db.Close() }()
	if err := db.Ping(); err != nil {
		return err
	}
	if err := goose.SetDialect("clickhouse"); err != nil {
		return err
	}
	// Create temp dir and write processed migration files
	tmpDir, err := os.MkdirTemp("", "freader_ch_mig_*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	// Compute full table name
	fullTable := table
	if database != "" && !strings.Contains(fullTable, ".") {
		fullTable = database + "." + table
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		b, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		content := strings.ReplaceAll(string(b), "__TABLE_FULL__", fullTable)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0o600); err != nil {
			return err
		}
	}
	if err := goose.Up(db, tmpDir); err != nil {
		return fmt.Errorf("goose up failed: %w", err)
	}
	return nil
}
