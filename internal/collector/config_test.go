package collector

import (
	"testing"

	"github.com/loykin/freader/internal/tailer"
	"github.com/loykin/freader/internal/watcher"
)

func TestConfig_DefaultsAndSetDefaultFingerprint(t *testing.T) {
	var c Config
	c.Default()

	if c.WorkerCount != 1 {
		t.Fatalf("WorkerCount default = %d, want 1", c.WorkerCount)
	}
	if c.PollInterval <= 0 {
		t.Fatalf("PollInterval default should be > 0, got %v", c.PollInterval)
	}
	if c.Separator != "\n" {
		t.Fatalf("Separator default = %q, want \\\"\\n\\\"", c.Separator)
	}
	if c.FingerprintStrategy != watcher.FingerprintStrategyDeviceAndInode {
		t.Fatalf("FingerprintStrategy default = %q, want %q", c.FingerprintStrategy, watcher.FingerprintStrategyDeviceAndInode)
	}
	if !c.StoreOffsets {
		t.Fatalf("StoreOffsets default should be true")
	}

	// Switch to checksum defaults
	c.SetDefaultFingerprint()
	if c.FingerprintStrategy != watcher.FingerprintStrategyChecksum {
		t.Fatalf("FingerprintStrategy after SetDefaultFingerprint = %q, want %q", c.FingerprintStrategy, watcher.FingerprintStrategyChecksum)
	}
	if c.FingerprintSize != watcher.DefaultFingerprintStrategySize {
		t.Fatalf("FingerprintSize after SetDefaultFingerprint = %d, want %d", c.FingerprintSize, watcher.DefaultFingerprintStrategySize)
	}
}

func TestConfigValidate_DeviceAndInode_AllowsEmptyIncludes(t *testing.T) {
	c := Config{}
	c.Default()
	c.Include = nil // empty includes allowed
	c.FingerprintStrategy = watcher.FingerprintStrategyDeviceAndInode

	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() with deviceAndInode and empty includes returned error: %v", err)
	}
}

func TestConfigValidate_Checksum_RequiresPositiveSize(t *testing.T) {
	c := Config{}
	c.Default()
	c.FingerprintStrategy = watcher.FingerprintStrategyChecksum
	c.FingerprintSize = 0 // invalid
	if err := c.Validate(); err == nil {
		t.Fatal("Validate() should error when checksum strategy has size <= 0")
	}

	c.FingerprintSize = 1 // valid
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error with valid size: %v", err)
	}
}

func TestConfigValidate_ChecksumSeparator_RequiresSizeAndSeparator(t *testing.T) {
	c := Config{}
	c.Default()
	c.FingerprintStrategy = watcher.FingerprintStrategyChecksumSeparator
	c.Separator = ""      // missing
	c.FingerprintSize = 0 // invalid size
	if err := c.Validate(); err == nil {
		t.Fatal("Validate() should error when checksumSeparator missing size and separator")
	}

	// Fix size but keep missing separator
	c.FingerprintSize = 8
	if err := c.Validate(); err == nil {
		t.Fatal("Validate() should error when checksumSeparator is missing separator")
	}

	// Fix separator as well
	c.Separator = "<END>"
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error after providing size and separator: %v", err)
	}
}

func TestConfigValidate_Multiline_ValidationPropagation(t *testing.T) {
	c := Config{}
	c.Default()
	c.SetDefaultFingerprint() // checksum is fine

	// Provide an invalid multiline config (missing both start/condition patterns with continue modes)
	ml := &tailer.MultilineReader{Mode: "continueThrough"}
	c.Multiline = ml
	if err := c.Validate(); err == nil {
		t.Fatal("Validate() should propagate error from Multiline.Validate() for invalid configuration")
	}

	// Provide a valid multiline config
	ml2 := &tailer.MultilineReader{Mode: "continueThrough", StartPattern: "^(INFO|ERROR)", ConditionPattern: "^\\s", Timeout: 200000000}
	c.Multiline = ml2
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() should succeed with valid multiline: %v", err)
	}
}
