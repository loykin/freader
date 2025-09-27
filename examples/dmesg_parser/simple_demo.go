package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/loykin/freader/pkg/parser/dmesg"
)

func main() {
	fmt.Println("=== dmesg Parser Simple Test ===")

	parser := dmesg.NewParser()
	bootTime := time.Now().Add(-2 * time.Hour)
	parser.SetBootTime(bootTime)

	testLines := []string{
		"[    0.000000] Linux version 5.15.0-56-generic (buildd@lcy02-amd64-044)",
		"[  100.500000] usb 1-1: new high-speed USB device number 2 using ehci-pci",
		"<6>[   20.000000] systemd[1]: Started Load Kernel Modules.",
		"<3>[  103.456789] kernel: Out of memory: Kill process 1234 (chrome) score 300",
		"[  200.000000] docker0: port 1(veth123abc) entered blocking state",
	}

	for i, line := range testLines {
		fmt.Printf("\n--- Test %d ---\n", i+1)
		fmt.Printf("Input: %s\n", line)

		record, err := parser.Parse(line)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		if record == nil {
			fmt.Println("Result: nil (empty line)")
			continue
		}

		fmt.Printf("Timestamp: %.6f seconds\n", record.Timestamp)
		fmt.Printf("Subsystem: %s\n", record.Subsystem)
		fmt.Printf("Priority: %s\n", record.GetPriorityName())
		fmt.Printf("Facility: %s\n", record.GetFacilityName())
		fmt.Printf("Message: %s\n", record.Message)

		if record.AbsoluteTime != nil {
			fmt.Printf("Absolute Time: %s\n", record.AbsoluteTime.Format(time.RFC3339))
		}

		// JSON 출력
		jsonData, _ := json.MarshalIndent(record, "", "  ")
		fmt.Printf("JSON:\n%s\n", string(jsonData))
	}

	fmt.Println("\n=== Test Completed ===")
}
