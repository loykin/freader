package clickhouse

import (
	"strings"
	"testing"
)

func TestClickHouseMigration_LabelsMapType(t *testing.T) {
	content, err := ReadEmbeddedMigration("00001_create_logs.sql")
	if err != nil {
		t.Fatalf("failed to read embedded migration: %v", err)
	}
	if !strings.Contains(content, "labels Map(String, String)") {
		t.Fatalf("expected labels column to be Map(String, String), got: %q", content)
	}
}

func TestClickHouseNew_MissingConfig(t *testing.T) {
	// Should fail fast before attempting any connection
	if _, err := New("", "", "", "", "", "", nil, 1, 1, nil, nil); err == nil {
		t.Fatal("expected error when addr or table is missing")
	}
}
