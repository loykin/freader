package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loykin/freader/pkg/watcher"

	"github.com/spf13/cobra"
)

// Config holds all configuration options for the freader application
type Config struct {
	// File monitoring options
	Include             []string
	Exclude             []string
	PollInterval        time.Duration
	FingerprintSize     int
	FingerprintStrategy string
	WorkerCount         int

	// Output options
	OutputFile string
	OutputType string

	// Metrics/Prometheus options
	PrometheusEnable bool
	PrometheusAddr   string
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		Include:             []string{"./log"},
		Exclude:             []string{},
		PollInterval:        2 * time.Second,
		FingerprintSize:     1024,
		FingerprintStrategy: "checksum",
		WorkerCount:         1,
		OutputType:          "stdout",
		PrometheusEnable:    false,
		PrometheusAddr:      ":2112",
	}
}

// SetupFlags adds all command line flags to the provided cobra command
func (c *Config) SetupFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVarP(&c.Include, "include", "I", c.Include, "Include patterns or directories to monitor (e.g., ./log, /var/log/*.log)")
	cmd.Flags().StringSliceVarP(&c.Exclude, "exclude", "E", c.Exclude, "Exclude patterns (e.g., *.tmp, *.log)")
	cmd.Flags().DurationVarP(&c.PollInterval, "poll-interval", "i", c.PollInterval, "Interval to poll for file changes")
	cmd.Flags().IntVarP(&c.FingerprintSize, "fingerprint-size", "s", c.FingerprintSize, "Size of fingerprint for checksum strategy")
	cmd.Flags().StringVarP(&c.FingerprintStrategy, "fingerprint-strategy", "f", c.FingerprintStrategy,
		fmt.Sprintf("Fingerprint strategy (%s or %s)",
			watcher.FingerprintStrategyChecksum,
			watcher.FingerprintStrategyDeviceAndInode))
	cmd.Flags().IntVarP(&c.WorkerCount, "workers", "w", c.WorkerCount, "Number of worker goroutines")
	cmd.Flags().StringVarP(&c.OutputFile, "output", "o", c.OutputFile, "Output file path (required when output-type is 'file')")
	cmd.Flags().StringVarP(&c.OutputType, "output-type", "t", c.OutputType, "Output type (stdout, stderr, or file)")

	// Prometheus flags
	cmd.Flags().BoolVar(&c.PrometheusEnable, "prometheus.enable", c.PrometheusEnable, "Enable Prometheus metrics HTTP endpoint")
	cmd.Flags().StringVar(&c.PrometheusAddr, "prometheus.addr", c.PrometheusAddr, "Prometheus metrics listen address (e.g., :2112)")
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check if output type is valid
	switch c.OutputType {
	case "stdout", "stderr", "file":
		// Valid output types
	default:
		return fmt.Errorf("invalid output type: %s", c.OutputType)
	}

	// Check if output file is specified when output type is file
	if c.OutputType == "file" && c.OutputFile == "" {
		return fmt.Errorf("output file path must be specified when output type is 'file'")
	}

	// Basic validation for prometheus addr if enabled
	if c.PrometheusEnable && c.PrometheusAddr == "" {
		return fmt.Errorf("prometheus.addr must be set when prometheus.enable is true")
	}

	return nil
}

// GetOutput returns the appropriate io.Writer based on the output configuration
func (c *Config) GetOutput() (io.Writer, func(), error) {
	var output io.Writer
	var cleanup func()

	switch c.OutputType {
	case "stdout":
		output = os.Stdout
		cleanup = func() {}
	case "stderr":
		output = os.Stderr
		cleanup = func() {}
	case "file":
		file, err := os.Create(c.OutputFile)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating output file: %v", err)
		}
		output = file
		cleanup = func() { _ = file.Close() }
	default:
		return nil, nil, fmt.Errorf("invalid output type: %s", c.OutputType)
	}

	return output, cleanup, nil
}
