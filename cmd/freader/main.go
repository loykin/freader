package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/loykin/freader"
	cmdmetrics "github.com/loykin/freader/cmd/freader/metrics"
	"github.com/loykin/freader/pkg/parser/audit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
)

func main() {
	config := DefaultConfig()

	rootCmd := &cobra.Command{
		Use:   "freader",
		Short: "A file reader that monitors and collects log data",
		Long: `freader is a tool that monitors files for changes and collects their content.
It can watch multiple directories, detect file changes, and output the content to stdout, stderr, or a file.

Examples:
  # Monitor the ./log directory and output to stdout
  freader

  # Monitor multiple directories with custom poll interval
  freader --include ./log,/var/log --poll-interval 5s

  # Use device+inode-based file tracking instead of checksum
  freader --fingerprint-strategy deviceAndInode

  # Use a config file (TOML/YAML/JSON)
  freader --config ./config/config.toml

  # Or set the environment variable (same effect as --config)
  FREADER_CONFIG=./config/config.toml freader

Notes:
  - Sink backends (ClickHouse/OpenSearch) and their credentials are configured via
    config file or environment variables only (not CLI flags). See config/config.toml.
`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration from Viper (flags, env, optional file) before validation
			if err := config.LoadFromViper(cmd); err != nil {
				return err
			}
			return config.Validate()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollector(config)
		},
	}

	// Setup flags from config
	config.SetupFlags(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func runCollector(config *Config) error {
	// Optionally start Prometheus metrics endpoint
	var metricsStop = func() error { return nil }
	if config.Prometheus.Enable {
		// Register library metrics and sink metrics before exposing the endpoint
		if err := freader.RegisterMetrics(prometheus.DefaultRegisterer); err != nil {
			return fmt.Errorf("failed to register default metrics: %w", err)
		}
		if err := cmdmetrics.Register(prometheus.DefaultRegisterer); err != nil {
			return fmt.Errorf("failed to register sink metrics: %w", err)
		}
		stopFn, err := freader.StartMetrics(config.Prometheus.Addr)
		if err != nil {
			return fmt.Errorf("failed to start prometheus endpoint: %w", err)
		}
		metricsStop = stopFn
	}

	// Start optional external sink (clickhouse/opensearch)
	sink, err := buildSink(config)
	if err != nil {
		return fmt.Errorf("failed to build sink: %w", err)
	}
	if sink != nil {
		defer func() { _ = sink.Stop() }()
	}

	// Prepare collector configuration from nested config
	cfg := config.Collector

	// Optional parser transform
	transform := func(s string) (string, bool) { return s, true }
	if config.Parser.Type == "auditd" {
		format := config.Parser.Format
		if format == "" {
			format = "json"
		}
		drop := config.Parser.DropNonMatching
		transform = func(s string) (string, bool) {
			rec, ok, _ := audit.Parse(s)
			if !ok {
				if drop {
					return "", false
				}
				return s, true
			}
			if format == "json" || format == "json-compact" {
				return rec.JSON(), true
			}
			// raw falls back to original line
			return s, true
		}
	}

	cfg.OnLineFunc = func(line string) {
		out, ok := transform(line)
		if !ok {
			return
		}
		if sink != nil {
			// When a sink is configured (stdout/opensearch/clickhouse), it is the single output path.
			// Do not duplicate to local output.
			sink.Enqueue(out)
			return
		}
		// No sink configured: fallback print to stdout
		fmt.Println(out)
	}

	// Create collector
	c, err := freader.NewCollector(cfg)
	if err != nil {
		_ = metricsStop()
		return errors.New("error creating collector: " + err.Error())
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start the collector
	c.Start()

	// Wait for interrupt signal
	fmt.Println("Running... Press Ctrl+C to stop")
	<-sigCh

	fmt.Println("Shutting down...")
	c.Stop()
	_ = metricsStop()

	return nil
}
