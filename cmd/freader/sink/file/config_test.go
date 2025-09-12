package file

import "testing"

func TestConfigValidate(t *testing.T) {
	var c Config
	if err := c.Validate(); err == nil {
		t.Fatal("expected error when path is empty")
	}
	c.Path = "/tmp/out.log"
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error for valid path: %v", err)
	}
}
