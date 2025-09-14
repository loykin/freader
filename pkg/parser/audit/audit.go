package audit

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	au "github.com/elastic/go-libaudit/v2/auparse"
)

// Record represents a parsed Linux auditd log entry.
// It captures the common header fields and key/value pairs.
//
// Example line:
//
//	type=SYSCALL msg=audit(1700000000.123:456): arch=c000003e syscall=59 success=yes ...
//
// We aim to be tolerant of small variations and will parse what we can.
type Record struct {
	Raw       string            `json:"raw"`
	Type      string            `json:"type"`
	EpochSec  int64             `json:"epoch_sec,omitempty"`
	EpochNSec int64             `json:"epoch_nsec,omitempty"`
	Serial    int64             `json:"serial,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

var (
	// Matches typical audit header: type=FOO msg=audit(1700000000.123:456):
	headRe = regexp.MustCompile(`^type=([A-Z_]+)\s+msg=audit\((\d+)\.(\d+):(\d+)\):\s*(.*)$`)
	// Some lines omit msg=audit() and just start with type=...
	altHeadRe = regexp.MustCompile(`^type=([A-Z_]+)\s+(.*)$`)
	// ensure we actually reference the auparse package so the dependency is used
	_ = au.AuditMessage{}
)

// Parse parses a single audit log line.
// Returns (record, true, nil) when successfully parsed; (zero, false, nil) when the line
// does not look like an audit log; and error if a hard parsing error occurred.
func Parse(line string) (Record, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Record{}, false, nil
	}

	if m := headRe.FindStringSubmatch(line); m != nil {
		rec := Record{Raw: line, Type: m[1], Fields: map[string]string{}}
		sec, _ := strconv.ParseInt(m[2], 10, 64)
		nsecStr := m[3]
		// Convert fractional seconds to nanoseconds; audit uses 3 digits (ms) commonly,
		// but handle arbitrary precision by right-padding/truncating to 9 digits.
		if len(nsecStr) < 9 {
			nsecStr = nsecStr + strings.Repeat("0", 9-len(nsecStr))
		} else if len(nsecStr) > 9 {
			nsecStr = nsecStr[:9]
		}
		nsec, _ := strconv.ParseInt(nsecStr, 10, 64)
		serial, _ := strconv.ParseInt(m[4], 10, 64)
		rest := m[5]
		rec.EpochSec = sec
		rec.EpochNSec = nsec
		rec.Serial = serial
		parseKeyValuesInto(rec.Fields, rest)
		return rec, true, nil
	}

	if m := altHeadRe.FindStringSubmatch(line); m != nil {
		rec := Record{Raw: line, Type: m[1], Fields: map[string]string{}}
		parseKeyValuesInto(rec.Fields, m[2])
		return rec, true, nil
	}

	return Record{}, false, nil
}

// parseKeyValuesInto parses key=value tokens, where value can be quoted and may contain spaces.
// Example: key1=val1 key2="hello world" key3='x y' key4=\"quoted\"
func parseKeyValuesInto(dst map[string]string, s string) {
	// Local tolerant tokenizer (we reference auparse types elsewhere to keep the dependency active).
	tokens := tokenizeKV(s)
	for _, t := range tokens {
		if eq := strings.IndexByte(t, '='); eq > 0 {
			k := t[:eq]
			v := t[eq+1:]
			v = strings.TrimSpace(v)
			if len(v) >= 2 {
				if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
					v = v[1 : len(v)-1]
				}
			}
			// Unescape common sequences
			v = strings.ReplaceAll(v, `\"`, `"`)
			dst[k] = v
		}
	}
}

// tokenizeKV splits a string by spaces, keeping quoted substrings intact.
func tokenizeKV(s string) []string {
	var out []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	esc := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if esc {
			b.WriteByte(ch)
			esc = false
			continue
		}
		switch ch {
		case '\\':
			esc = true
		case ' ':
			if inSingle || inDouble {
				b.WriteByte(ch)
			} else {
				flush()
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			b.WriteByte(ch)
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			b.WriteByte(ch)
		default:
			b.WriteByte(ch)
		}
	}
	flush()
	return out
}

// JSON returns a compact JSON representation of the record (best-effort).
func (r Record) JSON() string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
