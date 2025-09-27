package dmesg

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDmesgParser_Parse(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    string
		expected *Record
	}{
		{
			name:  "Standard dmesg format",
			input: "[    0.000000] Linux version 5.4.0-74-generic",
			expected: &Record{
				Raw:       "[    0.000000] Linux version 5.4.0-74-generic",
				Timestamp: 0.000000,
				Message:   "Linux version 5.4.0-74-generic",
				Subsystem: "kernel",
			},
		},
		{
			name:  "USB subsystem message",
			input: "[12345.678901] usb 1-1: new high-speed USB device number 2 using ehci-pci",
			expected: &Record{
				Raw:       "[12345.678901] usb 1-1: new high-speed USB device number 2 using ehci-pci",
				Timestamp: 12345.678901,
				Message:   "usb 1-1: new high-speed USB device number 2 using ehci-pci",
				Subsystem: "usb",
			},
		},
		{
			name:  "With priority/facility",
			input: "<6>[    1.234567] kernel: CPU0: Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
			expected: &Record{
				Raw:       "<6>[    1.234567] kernel: CPU0: Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
				Timestamp: 1.234567,
				Priority:  6,
				Facility:  0,
				Message:   "kernel: CPU0: Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
				Subsystem: "kernel",
			},
		},
		{
			name:  "Network subsystem",
			input: "[  100.500000] net eth0: link up, 1000Mbps, full-duplex",
			expected: &Record{
				Raw:       "[  100.500000] net eth0: link up, 1000Mbps, full-duplex",
				Timestamp: 100.5,
				Message:   "net eth0: link up, 1000Mbps, full-duplex",
				Subsystem: "net",
			},
		},
		{
			name:  "Non-standard format",
			input: "Some random log message without timestamp",
			expected: &Record{
				Raw:     "Some random log message without timestamp",
				Message: "Some random log message without timestamp",
			},
		},
		{
			name:     "Empty line",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(tt.input)
			require.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.Raw, result.Raw)
			assert.Equal(t, tt.expected.Timestamp, result.Timestamp)
			assert.Equal(t, tt.expected.Priority, result.Priority)
			assert.Equal(t, tt.expected.Facility, result.Facility)
			assert.Equal(t, tt.expected.Message, result.Message)
			assert.Equal(t, tt.expected.Subsystem, result.Subsystem)
		})
	}
}

func TestDmesgParser_ParseJSON(t *testing.T) {
	parser := NewParser()

	input := "[12345.678901] usb 1-1: new high-speed USB device number 2 using ehci-pci"
	jsonBytes, err := parser.ParseJSON(input)
	require.NoError(t, err)

	var result Record
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	assert.Equal(t, input, result.Raw)
	assert.Equal(t, 12345.678901, result.Timestamp)
	assert.Equal(t, "usb", result.Subsystem)
}

func TestDmesgParser_WithBootTime(t *testing.T) {
	parser := NewParser()
	bootTime := time.Date(2023, 12, 1, 10, 0, 0, 0, time.UTC)
	parser.SetBootTime(bootTime)

	input := "[  100.500000] kernel: test message"
	result, err := parser.Parse(input)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have calculated absolute time
	require.NotNil(t, result.AbsoluteTime)
	expected := bootTime.Add(time.Duration(100.5 * float64(time.Second)))
	assert.Equal(t, expected, *result.AbsoluteTime)
}

func TestRecord_GetPriorityName(t *testing.T) {
	tests := []struct {
		priority int
		expected string
	}{
		{0, "emergency"},
		{1, "alert"},
		{2, "critical"},
		{3, "error"},
		{4, "warning"},
		{5, "notice"},
		{6, "info"},
		{7, "debug"},
		{99, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			record := &Record{Priority: tt.priority}
			assert.Equal(t, tt.expected, record.GetPriorityName())
		})
	}
}

func TestRecord_GetFacilityName(t *testing.T) {
	tests := []struct {
		facility int
		expected string
	}{
		{0, "kernel"},
		{1, "user"},
		{3, "daemon"},
		{4, "auth"},
		{16, "local0"},
		{23, "local7"},
		{99, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			record := &Record{Facility: tt.facility}
			assert.Equal(t, tt.expected, record.GetFacilityName())
		})
	}
}

func TestDmesgParser_RealWorldExamples(t *testing.T) {
	parser := NewParser()

	realWorldLogs := []string{
		"[    0.000000] Linux version 5.15.0-56-generic (buildd@lcy02-amd64-044)",
		"[    0.000000] Command line: BOOT_IMAGE=/boot/vmlinuz root=UUID=abc123",
		"[    1.234567] ACPI: Added _OSI(Module Device)",
		"[   10.123456] pci 0000:00:1f.3: [8086:a348] type 00 class 0x040300",
		"[   15.678901] scsi 0:0:0:0: Direct-Access     ATA      Samsung SSD 850  2B6Q PQ: 0 ANSI: 5",
		"<6>[   20.000000] systemd[1]: Started Load Kernel Modules.",
		"<4>[   25.111111] thermal thermal_zone0: failed to read out thermal zone (-61)",
		"[  100.500000] docker0: port 1(veth123abc) entered blocking state",
	}

	for i, log := range realWorldLogs {
		t.Run(fmt.Sprintf("real_world_%d", i), func(t *testing.T) {
			result, err := parser.Parse(log)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, log, result.Raw)
			assert.NotEmpty(t, result.Message)
			assert.GreaterOrEqual(t, result.Timestamp, 0.0) // Allow 0.0 for boot messages

			// Should extract some subsystem for most entries
			if strings.Contains(log, "systemd") {
				assert.Equal(t, "systemd", result.Subsystem)
			} else if strings.Contains(log, "docker0:") {
				assert.Equal(t, "docker0", result.Subsystem) // docker0 is interface name
			}
		})
	}
}
