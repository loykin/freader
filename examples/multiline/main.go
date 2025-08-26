package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/loykin/freader"
)

func main() {
	// Simple logger to stdout
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})))

	// Flags
	var (
		path    string
		java    bool
		sep     string
		mode    string
		timeout time.Duration
	)
	// Leave path empty by default; we'll choose a sensible default based on -java
	flag.StringVar(&path, "path", "", "file or glob to read (if empty, auto-selects examples/multiline/logs/{generic|java} based on -java)")
	flag.BoolVar(&java, "java", false, "use Java-style multiline patterns (stack traces)")
	flag.StringVar(&sep, "sep", "\n", "record separator (supports multi-byte like \r\n or tokens like <END>)")
	flag.StringVar(&mode, "mode", "continueThrough", "multiline mode: continuePast|continueThrough|haltBefore|haltWith")
	flag.DurationVar(&timeout, "timeout", 500*time.Millisecond, "multiline timeout for flushing grouped records")
	flag.Parse()

	// Determine default path if not provided
	if path == "" {
		if java {
			path = filepath.Join("./examples/multiline/logs/java", "*.log")
		} else {
			path = filepath.Join("./examples/multiline/logs/generic", "*.log")
		}
	}

	cfg := freader.Config{}
	cfg.Default()
	cfg.Include = []string{path}
	cfg.Separator = sep

	// Resolve mode from flag and bind to exported constants (ensures all constants are referenced)
	var selectedMode string
	switch mode {
	case "continuePast":
		selectedMode = freader.MultilineReaderModeContinuePast
	case "continueThrough":
		selectedMode = freader.MultilineReaderModeContinueThrough
	case "haltBefore":
		selectedMode = freader.MultilineReaderModeHaltBefore
	case "haltWith":
		selectedMode = freader.MultilineReaderModeHaltWith
	default:
		selectedMode = freader.MultilineReaderModeContinueThrough
	}

	// Configure multiline
	if java {
		cfg.Multiline = &freader.MultilineReader{
			Mode:             selectedMode,
			StartPattern:     "^(ERROR|WARN|INFO|Exception)",
			ConditionPattern: "^(\\s|at\\s|Caused by:)",
			Timeout:          timeout,
		}
	} else {
		cfg.Multiline = &freader.MultilineReader{
			Mode:             selectedMode,
			StartPattern:     "^(INFO|WARN|ERROR)",
			ConditionPattern: "^\\s",
			Timeout:          timeout,
		}
	}

	// Print collected (grouped) lines
	cfg.OnLineFunc = func(line string) {
		fmt.Println(line)
	}

	c, err := freader.NewCollector(cfg)
	if err != nil {
		slog.Error("failed to create collector", "error", err)
		os.Exit(1)
	}

	slog.Info("starting multiline example", "path", path, "java", java, "mode", mode, "timeout", timeout.String(), "separator", sep)
	c.Start()
	// Run until interrupted (Ctrl+C) or for a short demo duration if FREADER_DEMO_SECONDS set
	if secs := os.Getenv("FREADER_DEMO_SECONDS"); secs != "" {
		// limited demo run
		d, err := time.ParseDuration(secs + "s")
		if err != nil {
			d = 3 * time.Second
		}
		time.Sleep(d)
		c.Stop()
		return
	}
	// Otherwise, wait for SIGINT/SIGTERM and stop gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	slog.Info("stopping multiline example...")
	c.Stop()
}
