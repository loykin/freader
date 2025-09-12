package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestDefaultConfigAndValidate(t *testing.T) {
	cfg := DefaultConfig()

	// Basic defaults
	if cfg.Sink.Type != "console" {
		t.Fatalf("default sink.type = %q, want console", cfg.Sink.Type)
	}
	if cfg.Prometheus.Enable {
		t.Fatal("prometheus.enable should default to false")
	}

	// Collector defaults should include embedded example paths and checksum strategy
	if len(cfg.Collector.Include) == 0 {
		t.Fatal("collector.include should have defaults for examples")
	}
	if cfg.Collector.FingerprintStrategy == "" {
		t.Fatal("collector.fingerprint-strategy should be set by default")
	}

	// Validate should pass for defaults
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should validate, got error: %v", err)
	}
}

func TestValidate_SinkTypes(t *testing.T) {
	// Invalid sink.type should error
	cfg := DefaultConfig()
	cfg.Sink.Type = "does-not-exist"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid sink.type, got nil")
	}

	// File sink requires a path
	cfg2 := DefaultConfig()
	cfg2.Sink.Type = "file"
	cfg2.Sink.File.Path = ""
	if err := cfg2.Validate(); err == nil {
		t.Fatal("expected error when sink.type=file and sink.file.path is empty")
	}
	cfg2.Sink.File.Path = filepath.Join(t.TempDir(), "out.log")
	if err := cfg2.Validate(); err != nil {
		t.Fatalf("unexpected error for valid file sink: %v", err)
	}
}

func TestLoadFromViper_WithEnvConfigAndFlags(t *testing.T) {
	// Prepare a Cobra command and default config
	cfg := DefaultConfig()
	cmd := &cobra.Command{Use: "freader-test"}
	cfg.SetupFlags(cmd)

	// Use repository example config file via FREADER_CONFIG. When running tests from
	// the cmd/freader package, the working directory is this package dir, so the
	// config lives two levels up. Probe a few likely locations to be robust.
	candidates := []string{
		filepath.Join(".", "config", "config.toml"),
		filepath.Join("..", "config", "config.toml"),
		filepath.Join("..", "..", "config", "config.toml"),
		filepath.Join("..", "..", "..", "config", "config.toml"),
	}
	var configPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}
	}
	if configPath == "" {
		t.Fatalf("missing test config file: tried %v", candidates)
	}

	// Set env var and ensure cleanup
	prev := os.Getenv("FREADER_CONFIG")
	if err := os.Setenv("FREADER_CONFIG", configPath); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("FREADER_CONFIG", prev) })

	// Set some flags that should override config file values
	// Enable Prometheus and set a different addr
	if err := cmd.Flags().Set("prometheus.enable", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := cmd.Flags().Set("prometheus.addr", "127.0.0.1:0"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	// Override include patterns via flags to ensure precedence over config file
	customIncludes := []string{"./some/other/path", "./another/*.log"}
	if err := cmd.Flags().Set("include", customIncludes[0]+","+customIncludes[1]); err != nil {
		t.Fatalf("set include flag: %v", err)
	}

	// Load from Viper (env+file+flags)
	if err := cfg.LoadFromViper(cmd); err != nil {
		t.Fatalf("LoadFromViper failed: %v", err)
	}

	// Flags should take precedence over file
	if !cfg.Prometheus.Enable {
		t.Fatal("prometheus.enable should be true from flag override")
	}
	if got := cfg.Prometheus.Addr; got != "127.0.0.1:0" {
		t.Fatalf("prometheus.addr = %q, want 127.0.0.1:0", got)
	}
	if !reflect.DeepEqual(cfg.Collector.Include, customIncludes) {
		t.Fatalf("collector.include = %#v, want %#v (flags override file)", cfg.Collector.Include, customIncludes)
	}

	// Some fields should come from the file (we didn't override them)
	if cfg.Sink.Type != "console" {
		t.Fatalf("sink.type from file = %q, want console", cfg.Sink.Type)
	}
	if cfg.Collector.FingerprintStrategy != "checksum" {
		t.Fatalf("collector.fingerprint-strategy from file = %q, want checksum", cfg.Collector.FingerprintStrategy)
	}

	// Final validation should pass after loading
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate failed after LoadFromViper: %v", err)
	}
}
