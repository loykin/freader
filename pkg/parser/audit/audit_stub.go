//go:build !linux

package audit

// Record represents a parsed Linux auditd log entry.
// On non-Linux platforms, this is a minimal stub to keep API compatibility.
type Record struct {
	Raw       string            `json:"raw"`
	Type      string            `json:"type"`
	EpochSec  int64             `json:"epoch_sec,omitempty"`
	EpochNSec int64             `json:"epoch_nsec,omitempty"`
	Serial    int64             `json:"serial,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// Parse on non-Linux platforms always reports that the line is not an audit log.
func Parse(_ string) (Record, bool, error) {
	return Record{}, false, nil
}

// JSON returns a simple JSON-like string for compatibility; since Parse never
// matches on non-Linux, this is rarely used.
func (r Record) JSON() string {
	// Keep implementation minimal to avoid extra dependencies on non-Linux.
	return "{" + "\"raw\":\"" + r.Raw + "\"}"
}
