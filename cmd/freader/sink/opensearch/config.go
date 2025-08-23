package opensearch

import "fmt"

// Config holds OpenSearch sink connection settings.
type Config struct {
	URL      string `mapstructure:"url"` // http(s)://host:9200
	Index    string `mapstructure:"index"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

func (c Config) Validate() error {
	if c.URL == "" || c.Index == "" {
		return fmt.Errorf("sink.opensearch requires url and index")
	}
	return nil
}
