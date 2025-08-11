package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/loykin/freader/pkg/collector"
	"github.com/loykin/freader/pkg/metrics"

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

  # Output to a file instead of stdout
  freader --output-type file --output /tmp/output.log

  # Use inode-based file tracking instead of checksum
  freader --fingerprint-strategy inode`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
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
	// Setup output
	output, cleanup, err := config.GetOutput()
	if err != nil {
		return err
	}
	defer cleanup()

	// Optionally start Prometheus metrics endpoint
	var metricsStop = func() error { return nil }
	if config.PrometheusEnable {
		// Register our metrics explicitly to the default registry to avoid library init-time side effects
		if err := metrics.Register(prometheus.DefaultRegisterer); err != nil {
			return fmt.Errorf("failed to register prometheus metrics: %w", err)
		}
		metricsServer, err := metrics.Start(config.PrometheusAddr)
		if err != nil {
			return fmt.Errorf("failed to start prometheus endpoint: %w", err)
		}
		metricsStop = metricsServer.Stop
	}

	// Create configuration
	cfg := collector.Config{}
	cfg.Default()
	// Use include-only filtering
	cfg.Include = config.Include
	cfg.FingerprintSize = config.FingerprintSize
	cfg.PollInterval = config.PollInterval
	cfg.FingerprintStrategy = config.FingerprintStrategy
	cfg.WorkerCount = config.WorkerCount
	cfg.Exclude = config.Exclude
	cfg.OnLineFunc = func(line string) {
		_, err := fmt.Fprintln(output, line)
		if err != nil {
			slog.Error(err.Error())
			return
		}
	}

	// Create collector
	c, err := collector.NewCollector(cfg)
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
