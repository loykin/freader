//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loykin/freader"
	"github.com/loykin/freader/pkg/parser/audit"
)

// This example demonstrates how to use freader's Collector to tail an audit log
// file and parse each line using the audit log parser, then print it as JSON.
func main() {
	logDir := filepath.Join("examples", "audit_log", "log")
	_ = os.MkdirAll(logDir, 0o755)
	logFile := filepath.Join(logDir, "audit.log")

	// If the file is empty or does not exist, seed with a few lines
	if _, err := os.Stat(logFile); err != nil {
		seed(logFile)
	}

	stopMetrics, err := freader.StartMetrics(":2112")
	if err != nil {
		slog.Warn("metrics not started", "err", err)
	}
	defer func() {
		if stopMetrics != nil {
			_ = stopMetrics()
		}
	}()

	cfg := freader.Config{
		Include:             []string{filepath.Join(logDir, "*.log")},
		Exclude:             nil,
		Separator:           "\n",
		PollInterval:        200 * time.Millisecond,
		FingerprintStrategy: freader.FingerprintStrategyChecksum,
		FingerprintSize:     1024,
		StoreOffsets:        false,
		DBPath:              "",
		Multiline:           nil,
		OnLineFunc: func(line string) {
			rec, ok, _ := audit.Parse(line)
			if !ok {
				// Not an audit line; print raw
				fmt.Println(line)
				return
			}
			b, _ := json.Marshal(rec)
			fmt.Println(string(b))
		},
	}

	collector, err := freader.NewCollector(cfg)
	if err != nil {
		slog.Error("failed to create collector", "error", err)
		os.Exit(1)
	}

	collector.Start()
	defer collector.Stop()

	// Keep running; in a real app, you might hook signals to exit.
	// Here we just follow the file for a short time and then exit.
	t := time.NewTimer(2 * time.Second)
	<-t.C
}

func seed(path string) {
	lines := []string{
		`type=PROCTITLE msg=audit(1700000000.001:123): proctitle=2F62696E2F7368002F62696E2F7368002D63`,
		`type=PATH msg=audit(1700000000.001:123): item=0 name="/usr/bin/sh" inode=123 dev=08:01 mode=0100755 ouid=0 ogid=0 rdev=00:00 nametype=NORMAL`,
		`type=SYSCALL msg=audit(1700000000.001:123): arch=c000003e syscall=59 success=yes exit=0 a0=55ac4 a1=55ac4 a2=55ac4 a3=7ffc items=2 ppid=1 pid=4242 uid=0 gid=0 euid=0 suid=0 fsuid=0 egid=0 sgid=0 fsgid=0 comm="sh" exe="/usr/bin/sh" key=(null)`,
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
