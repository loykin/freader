package clickhouse

import "fmt"

// Config holds ClickHouse sink connection settings.
type Config struct {
	Addr     string `mapstructure:"addr"` // http(s)://host:8123 or native host:9000
	Database string `mapstructure:"database"`
	Table    string `mapstructure:"table"` // table or db.table
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

func (c Config) Validate() error {
	if c.Addr == "" || c.Table == "" {
		return fmt.Errorf("sink.clickhouse requires addr and table")
	}
	return nil
}
