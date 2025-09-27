package dmesg

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Record represents a parsed dmesg log entry
// Example formats:
//
//	[    0.000000] Linux version 5.4.0-74-generic
//	[12345.678901] usb 1-1: new high-speed USB device number 2 using ehci-pci
//	<6>[    0.000000] Linux version 5.4.0-74-generic (with facility/priority)
type Record struct {
	Raw          string     `json:"raw"`
	Timestamp    float64    `json:"timestamp"`               // Seconds since boot
	Facility     int        `json:"facility,omitempty"`      // Syslog facility (if present)
	Priority     int        `json:"priority,omitempty"`      // Syslog priority (if present)
	Subsystem    string     `json:"subsystem"`               // Kernel subsystem (e.g., "usb", "net")
	Message      string     `json:"message"`                 // The actual log message
	BootTime     *time.Time `json:"boot_time,omitempty"`     // System boot time (if known)
	AbsoluteTime *time.Time `json:"absolute_time,omitempty"` // Calculated absolute time
}

// Parser handles dmesg log parsing
type Parser struct {
	// dmesgRegex matches: [optional_priority][timestamp] message
	dmesgRegex *regexp.Regexp
	// subsystemRegex extracts subsystem from message
	subsystemRegex *regexp.Regexp
	// bootTime for converting relative timestamps to absolute time
	bootTime *time.Time
}

// NewParser creates a new dmesg parser
func NewParser() *Parser {
	return &Parser{
		// Matches: <6>[12345.678901] or [12345.678901]
		dmesgRegex: regexp.MustCompile(`^(?:<(\d+)>)?\[\s*(\d+(?:\.\d+)?)]\s*(.*)$`),
		// Extracts subsystem: "usb 1-1:" -> "usb", "net eth0:" -> "net", "kernel:" -> "kernel"
		subsystemRegex: regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_-]*)\s*.*?:`),
	}
}

// SetBootTime sets the system boot time for absolute timestamp calculation
func (p *Parser) SetBootTime(bootTime time.Time) {
	p.bootTime = &bootTime
}

// Parse parses a single dmesg log line
func (p *Parser) Parse(line string) (*Record, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	matches := p.dmesgRegex.FindStringSubmatch(line)
	if len(matches) != 4 {
		// Not a standard dmesg format, treat as plain message
		return &Record{
			Raw:     line,
			Message: line,
		}, nil
	}

	record := &Record{
		Raw: line,
	}

	// Parse priority/facility if present
	if matches[1] != "" {
		if priority, err := strconv.Atoi(matches[1]); err == nil {
			record.Priority = priority & 0x07 // Lower 3 bits
			record.Facility = priority >> 3   // Upper bits
		}
	}

	// Parse timestamp
	if timestamp, err := strconv.ParseFloat(matches[2], 64); err == nil {
		record.Timestamp = timestamp

		// Calculate absolute time if boot time is known
		if p.bootTime != nil {
			duration := time.Duration(timestamp * float64(time.Second))
			absoluteTime := p.bootTime.Add(duration)
			record.AbsoluteTime = &absoluteTime
		}
	}

	// Parse message and extract subsystem
	message := strings.TrimSpace(matches[3])
	record.Message = message

	// Extract subsystem from message
	if subMatches := p.subsystemRegex.FindStringSubmatch(message); len(subMatches) > 1 {
		record.Subsystem = subMatches[1]
	} else {
		// Try to extract subsystem from beginning of message
		parts := strings.Fields(message)
		if len(parts) > 0 {
			// Common patterns: "usb", "net", "kernel", etc.
			first := strings.ToLower(parts[0])
			if isKnownSubsystem(first) {
				record.Subsystem = first
			} else {
				// Special cases for Linux version and other common patterns
				if strings.Contains(message, "Linux version") {
					record.Subsystem = "kernel"
				} else if strings.Contains(message, "systemd[") {
					record.Subsystem = "systemd"
				} else if strings.Contains(message, "docker") {
					record.Subsystem = "docker"
				}
			}
		}
	}

	return record, nil
}

// ParseJSON parses a dmesg line and returns JSON
func (p *Parser) ParseJSON(line string) ([]byte, error) {
	record, err := p.Parse(line)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return json.Marshal(record)
}

// isKnownSubsystem checks if a string is a known kernel subsystem
func isKnownSubsystem(s string) bool {
	knownSubsystems := map[string]bool{
		"kernel":     true,
		"usb":        true,
		"net":        true,
		"pci":        true,
		"acpi":       true,
		"cpu":        true,
		"memory":     true,
		"disk":       true,
		"filesystem": true,
		"block":      true,
		"scsi":       true,
		"ata":        true,
		"sound":      true,
		"input":      true,
		"thermal":    true,
		"power":      true,
		"bluetooth":  true,
		"wifi":       true,
		"ethernet":   true,
		"bridge":     true,
		"firewall":   true,
		"systemd":    true,
		"docker":     true,
		"kvm":        true,
		"xen":        true,
	}
	return knownSubsystems[s]
}

// GetPriorityName returns human-readable priority name
func (r *Record) GetPriorityName() string {
	priorities := []string{
		"emergency", "alert", "critical", "error",
		"warning", "notice", "info", "debug",
	}
	if r.Priority >= 0 && r.Priority < len(priorities) {
		return priorities[r.Priority]
	}
	return "unknown"
}

// GetFacilityName returns human-readable facility name
func (r *Record) GetFacilityName() string {
	facilities := []string{
		"kernel", "user", "mail", "daemon", "auth", "syslog",
		"lpr", "news", "uucp", "cron", "authpriv", "ftp",
		"ntp", "security", "console", "solaris-cron",
		"local0", "local1", "local2", "local3",
		"local4", "local5", "local6", "local7",
	}
	if r.Facility >= 0 && r.Facility < len(facilities) {
		return facilities[r.Facility]
	}
	return "unknown"
}
