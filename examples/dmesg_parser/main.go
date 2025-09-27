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
	"github.com/loykin/freader/pkg/parser/dmesg"
)

// This example demonstrates how to use freader's Collector to tail kernel logs
// and parse each line using the dmesg parser, then print it as JSON.
func main() {
	logDir := filepath.Join("examples", "dmesg_parser", "log")
	_ = os.MkdirAll(logDir, 0o755)
	logFile := filepath.Join(logDir, "dmesg.log")

	// If the file is empty or does not exist, seed with sample dmesg lines
	if _, err := os.Stat(logFile); err != nil {
		seed(logFile)
	}

	stopMetrics, err := freader.StartMetrics(":2113")
	if err != nil {
		slog.Warn("metrics not started", "err", err)
	}
	defer func() {
		if stopMetrics != nil {
			_ = stopMetrics()
		}
	}()

	// Create dmesg parser with boot time (optional)
	parser := dmesg.NewParser()
	// Set boot time for absolute timestamp calculation
	bootTime := time.Now().Add(-2 * time.Hour) // Simulate system booted 2 hours ago
	parser.SetBootTime(bootTime)

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
			record, err := parser.Parse(line)
			if err != nil {
				slog.Warn("failed to parse dmesg line", "error", err, "line", line)
				return
			}
			if record == nil {
				return // Empty line
			}

			// Print parsed record as JSON
			b, _ := json.MarshalIndent(record, "", "  ")
			fmt.Println(string(b))

			// Example: Filter by subsystem
			if record.Subsystem == "usb" {
				fmt.Printf("🔌 USB Event: %s\n", record.Message)
			}

			// Example: Alert on errors
			if record.GetPriorityName() == "error" {
				fmt.Printf("❌ ERROR: %s (subsystem: %s)\n", record.Message, record.Subsystem)
			}

			// Example: Show absolute time if available
			if record.AbsoluteTime != nil {
				fmt.Printf("🕐 Absolute time: %s\n", record.AbsoluteTime.Format(time.RFC3339))
			}
			fmt.Println("---")
		},
	}

	collector, err := freader.NewCollector(cfg)
	if err != nil {
		slog.Error("failed to create collector", "error", err)
		os.Exit(1)
	}

	collector.Start()
	defer collector.Stop()

	fmt.Println("Monitoring dmesg logs for 5 seconds...")
	fmt.Println("Adding more sample data...")

	// Add more sample data after a delay
	go func() {
		time.Sleep(1 * time.Second)
		addMoreDmesgData(logFile)
	}()

	// Keep running for a while to demonstrate parsing
	fmt.Println("Waiting for logs to be processed...")
	t := time.NewTimer(8 * time.Second)
	<-t.C
	fmt.Println("Example completed!")
}

func seed(path string) {
	lines := []string{
		"[    0.000000] Linux version 5.15.0-56-generic (buildd@lcy02-amd64-044)",
		"[    0.000000] Command line: BOOT_IMAGE=/boot/vmlinuz root=UUID=abc123",
		"[    1.234567] ACPI: Added _OSI(Module Device)",
		"[   10.123456] pci 0000:00:1f.3: [8086:a348] type 00 class 0x040300",
		"[   15.678901] scsi 0:0:0:0: Direct-Access     ATA      Samsung SSD 850  2B6Q PQ: 0 ANSI: 5",
		"<6>[   20.000000] systemd[1]: Started Load Kernel Modules.",
		"<4>[   25.111111] thermal thermal_zone0: failed to read out thermal zone (-61)",
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func addMoreDmesgData(path string) {
	additionalLines := []string{
		"[  100.500000] usb 1-1: new high-speed USB device number 2 using ehci-pci",
		"[  101.234567] usb 1-1: New USB device found, idVendor=0781, idProduct=5567",
		"[  102.000000] net eth0: link up, 1000Mbps, full-duplex",
		"<3>[  103.456789] kernel: Out of memory: Kill process 1234 (chrome) score 300",
		"[  200.000000] docker0: port 1(veth123abc) entered blocking state",
		"[  201.111111] ata1.00: exception Emask 0x0 SAct 0x0 SErr 0x0 action 0x6 frozen",
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	for _, line := range additionalLines {
		_, _ = file.WriteString(line + "\n")
		time.Sleep(300 * time.Millisecond) // Simulate real-time log generation
	}
}
