package store

import (
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// InitMigrations initializes the embedded migrations
func InitMigrations() {
	goose.SetBaseFS(migrationFS)
}
