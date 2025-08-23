package metrics

// Config holds metrics endpoint options.
type Config struct {
	Enable bool   `mapstructure:"enable"`
	Addr   string `mapstructure:"addr"`
}
