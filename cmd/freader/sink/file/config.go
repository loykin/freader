package file

import "fmt"

// Config holds forwarding configuration and nested backend settings.
type Config struct {
	Path string `mapstructure:"path"`
}

func (c Config) Validate() error {
	if c.Path == "" {
		return fmt.Errorf("sink.file.path must be set when sink.type is 'file'")
	}
	return nil
}
