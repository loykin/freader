package console

import "fmt"

// Config holds console sink options.
type Config struct {
	Stream string `mapstructure:"stream"` // stdout or stderr
}

// Validate ensures the console sink configuration is correct when used.
func (c Config) Validate() error {
	if c.Stream == "" {
		return nil // default is stdout
	}
	if c.Stream != "stdout" && c.Stream != "stderr" {
		return fmt.Errorf("sink.console.stream must be 'stdout' or 'stderr'")
	}
	return nil
}
