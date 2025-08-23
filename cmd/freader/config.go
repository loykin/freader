package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/loykin/freader"
	"github.com/loykin/freader/cmd/freader/metrics"
	"github.com/loykin/freader/internal/collector"

	cmdclick "github.com/loykin/freader/cmd/freader/sink/clickhouse"
	cmdconsole "github.com/loykin/freader/cmd/freader/sink/console"
	cmdfile "github.com/loykin/freader/cmd/freader/sink/file"
	cmdos "github.com/loykin/freader/cmd/freader/sink/opensearch"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type SinkConfig struct {
	Type          string            `mapstructure:"type"` // "" (disabled), "console", "stdout", "stderr", "file", "clickhouse", "opensearch"
	Include       []string          `mapstructure:"include"`
	Exclude       []string          `mapstructure:"exclude"`
	BatchSize     int               `mapstructure:"batch-size"`
	BatchInterval time.Duration     `mapstructure:"batch-interval"`
	Host          string            `mapstructure:"host"`   // override host; default os.Hostname()
	Labels        map[string]string `mapstructure:"labels"` // optional key-value labels
	Console       cmdconsole.Config `mapstructure:"console"`
	ClickHouse    cmdclick.Config   `mapstructure:"clickhouse"`
	OpenSearch    cmdos.Config      `mapstructure:"opensearch"`
	File          cmdfile.Config    `mapstructure:"file"`
}

// Config holds all configuration options for the freader application
// It now uses a nested Collector config for the reader options.
type Config struct {
	// Optional config file path (flag/env only)
	ConfigFile string
	// Reader/collector configuration (nested)
	Collector collector.Config `mapstructure:"collector"`
	// Forwarding sink (nested and unified output)
	Sink SinkConfig `mapstructure:"sink"`
	// Metrics/Prometheus options
	Prometheus metrics.Config `mapstructure:"prometheus"`
}

// LoadFromViper binds flags to viper, reads file/env, and populates the Config fields via mapstructure.
func (c *Config) LoadFromViper(cmd *cobra.Command) error {
	v := viper.GetViper()
	v.SetEnvPrefix("FREADER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Bind all flags
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	// Determine config file path: --config flag or FREADER_CONFIG env; no auto-defaults
	if c.ConfigFile == "" {
		// Viper AutomaticEnv binds FREADER_CONFIG to key "config"
		c.ConfigFile = v.GetString("config")
	}
	if c.ConfigFile != "" {
		v.SetConfigFile(c.ConfigFile)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into this Config using mapstructure with proper tagname and duration hooks
	if err := v.Unmarshal(c); err != nil {
		return err
	}
	return nil
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	cfg := &Config{
		Sink: SinkConfig{
			Type:          "console", // default console sink; configure [sink.console.stream]
			Include:       []string{},
			Exclude:       []string{},
			BatchSize:     200,
			BatchInterval: 2 * time.Second,
			Labels:        map[string]string{},
			Console:       cmdconsole.Config{Stream: "stdout"},
		},
		Prometheus: metrics.Config{Enable: false, Addr: ":2112"},
	}
	// Initialize nested collector defaults
	cfg.Collector.Default()
	// Make the quick-start UX pleasant: watch bundled example logs and use checksum
	cfg.Collector.Include = []string{"./examples/embeded/log", "./examples/embeded/log/*.log"}
	cfg.Collector.Exclude = []string{}
	cfg.Collector.PollInterval = 2 * time.Second
	cfg.Collector.FingerprintStrategy = freader.FingerprintStrategyChecksum
	cfg.Collector.FingerprintSize = 64
	cfg.Collector.WorkerCount = 1
	return cfg
}

// SetupFlags adds all command line flags to the provided cobra command
func (c *Config) SetupFlags(cmd *cobra.Command) {
	// Config file
	cmd.Flags().StringVar(&c.ConfigFile, "config", c.ConfigFile, "Path to config file (yaml/json/toml)")

	// Collector flags (write directly into nested struct)
	cmd.Flags().StringSliceVarP(&c.Collector.Include, "include", "I", c.Collector.Include, "Include patterns or directories to monitor (e.g., ./log, /var/log/*.log)")
	cmd.Flags().StringSliceVarP(&c.Collector.Exclude, "exclude", "E", c.Collector.Exclude, "Exclude patterns (e.g., *.tmp, *.log)")
	cmd.Flags().DurationVarP(&c.Collector.PollInterval, "poll-interval", "i", c.Collector.PollInterval, "Interval to poll for file changes")
	cmd.Flags().StringVar(&c.Collector.Separator, "separator", c.Collector.Separator, "Record separator (string, supports multi-byte like \\\"\\r\\n\\\" or tokens like <END>)")
	cmd.Flags().IntVarP(&c.Collector.FingerprintSize, "fingerprint-size", "s", c.Collector.FingerprintSize, "Size of fingerprint for checksum strategy (or N separators for checksumSeperator)")
	cmd.Flags().StringVarP(&c.Collector.FingerprintStrategy, "fingerprint-strategy", "f", c.Collector.FingerprintStrategy,
		fmt.Sprintf("Fingerprint strategy (%s or %s)",
			freader.FingerprintStrategyChecksum,
			freader.FingerprintStrategyDeviceAndInode))
	cmd.Flags().IntVarP(&c.Collector.WorkerCount, "workers", "w", c.Collector.WorkerCount, "Number of worker goroutines")
	cmd.Flags().StringVar(&c.Collector.DBPath, "db-path", c.Collector.DBPath, "Path to offsets SQLite DB (when --store-offsets)")
	cmd.Flags().BoolVar(&c.Collector.StoreOffsets, "store-offsets", c.Collector.StoreOffsets, "Store and restore offsets across restarts")

	// Sink-related options are intentionally not exposed as command-line flags.
	// Configure sink forwarding (type, filters, batching, and backend credentials)
	// via config file (e.g., --config or FREADER_CONFIG) or environment variables
	// (FREADER_SINK, FREADER_SINK__CLICKHOUSE__ADDR, etc.).

	// Prometheus flags
	cmd.Flags().BoolVar(&c.Prometheus.Enable, "prometheus.enable", c.Prometheus.Enable, "Enable Prometheus metrics HTTP endpoint")
	cmd.Flags().StringVar(&c.Prometheus.Addr, "prometheus.addr", c.Prometheus.Addr, "Prometheus metrics listen address (e.g., :2112)")
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Sink validation
	switch c.Sink.Type {
	case "", "console", "file", "clickhouse", "opensearch":
		// ok
	default:
		return fmt.Errorf("invalid sink.type: %s", c.Sink.Type)
	}
	if c.Sink.Type != "" {
		if c.Sink.BatchSize <= 0 {
			return fmt.Errorf("sink.batch-size must be > 0")
		}
		if c.Sink.BatchInterval <= 0 {
			return fmt.Errorf("sink.batch-interval must be > 0")
		}
		// Delegate sink-specific validations to each sink config
		switch c.Sink.Type {
		case "console":
			if err := c.Sink.Console.Validate(); err != nil {
				return err
			}
		case "file":
			if err := c.Sink.File.Validate(); err != nil {
				return err
			}
		case "clickhouse":
			if err := c.Sink.ClickHouse.Validate(); err != nil {
				return err
			}
		case "opensearch":
			if err := c.Sink.OpenSearch.Validate(); err != nil {
				return err
			}
		}
	}

	// Basic validation for prometheus addr if enabled
	if c.Prometheus.Enable && c.Prometheus.Addr == "" {
		return fmt.Errorf("prometheus.addr must be set when prometheus.enable is true")
	}

	// Validate nested collector as well
	if err := c.Collector.Validate(); err != nil {
		return fmt.Errorf("invalid collector config: %w", err)
	}

	return nil
}
