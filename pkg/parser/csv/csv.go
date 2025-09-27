package csv

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Record represents a parsed CSV log entry
type Record struct {
	Raw      string                 `json:"raw"`
	Fields   map[string]interface{} `json:"fields"`
	LineNum  int                    `json:"line_number"`
	ParsedAt time.Time              `json:"parsed_at"`
}

// Parser handles CSV log parsing
type Parser struct {
	headers         []string
	delimiter       rune
	hasHeaders      bool
	autoDetectTypes bool
	timestampField  string
	timestampFormat string
	lineCount       int
}

// Config holds CSV parser configuration
type Config struct {
	Delimiter       rune     `json:"delimiter"`         // Default: ','
	HasHeaders      bool     `json:"has_headers"`       // First line contains headers
	Headers         []string `json:"headers"`           // Custom headers if no header line
	AutoDetectTypes bool     `json:"auto_detect_types"` // Auto-detect number/boolean types
	TimestampField  string   `json:"timestamp_field"`   // Field name containing timestamp
	TimestampFormat string   `json:"timestamp_format"`  // Time format (Go layout)
}

// NewParser creates a new CSV parser with configuration
func NewParser(config Config) *Parser {
	delimiter := config.Delimiter
	if delimiter == 0 {
		delimiter = ','
	}

	return &Parser{
		headers:         config.Headers,
		delimiter:       delimiter,
		hasHeaders:      config.HasHeaders,
		autoDetectTypes: config.AutoDetectTypes,
		timestampField:  config.TimestampField,
		timestampFormat: config.TimestampFormat,
		lineCount:       0,
	}
}

// Parse parses a single CSV line
func (p *Parser) Parse(line string) (*Record, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	p.lineCount++

	// Use csv.Reader for proper CSV parsing
	reader := csv.NewReader(strings.NewReader(line))
	reader.Comma = p.delimiter
	reader.TrimLeadingSpace = true

	fields, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to parse CSV line %d: %w", p.lineCount, err)
	}

	// Handle headers on first line
	if p.lineCount == 1 && p.hasHeaders {
		p.headers = fields
		return nil, nil // Skip header line
	}

	// Generate default headers if not provided
	if len(p.headers) == 0 {
		p.headers = make([]string, len(fields))
		for i := range p.headers {
			p.headers[i] = fmt.Sprintf("field_%d", i+1)
		}
	}

	// Create field map
	fieldMap := make(map[string]interface{})
	for i, value := range fields {
		var fieldName string
		if i < len(p.headers) {
			fieldName = p.headers[i]
		} else {
			fieldName = fmt.Sprintf("extra_field_%d", i+1)
		}

		// Auto-detect types if enabled
		if p.autoDetectTypes {
			fieldMap[fieldName] = p.detectType(value)
		} else {
			fieldMap[fieldName] = value
		}
	}

	record := &Record{
		Raw:      line,
		Fields:   fieldMap,
		LineNum:  p.lineCount,
		ParsedAt: time.Now(),
	}

	// Parse timestamp if configured
	if p.timestampField != "" && p.timestampFormat != "" {
		if timestampValue, exists := fieldMap[p.timestampField]; exists {
			if timestampStr, ok := timestampValue.(string); ok {
				if parsedTime, err := time.Parse(p.timestampFormat, timestampStr); err == nil {
					fieldMap[p.timestampField+"_parsed"] = parsedTime
				}
			}
		}
	}

	return record, nil
}

// ParseJSON parses a CSV line and returns JSON
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

// detectType attempts to convert string to appropriate type
func (p *Parser) detectType(value string) interface{} {
	value = strings.TrimSpace(value)

	// Empty string
	if value == "" {
		return ""
	}

	// Boolean
	switch strings.ToLower(value) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	}

	// Integer
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		// Check if it fits in int
		if intVal >= int64(int(^uint(0)>>1)*-1) && intVal <= int64(int(^uint(0)>>1)) {
			return int(intVal)
		}
		return intVal
	}

	// Float
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}

	// Timestamp auto-detection (common formats)
	commonTimeFormats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006/01/02 15:04:05",
		"01/02/2006 15:04:05",
		"2006-01-02",
		"01/02/2006",
	}

	for _, format := range commonTimeFormats {
		if parsedTime, err := time.Parse(format, value); err == nil {
			return parsedTime
		}
	}

	// Default to string
	return value
}

// GetHeaders returns the current headers
func (p *Parser) GetHeaders() []string {
	return append([]string(nil), p.headers...) // Return copy
}

// SetHeaders sets custom headers
func (p *Parser) SetHeaders(headers []string) {
	p.headers = append([]string(nil), headers...) // Make copy
}

// Reset resets the parser state (useful for parsing new files)
func (p *Parser) Reset() {
	p.lineCount = 0
	if p.hasHeaders {
		p.headers = nil // Will be set from first line
	}
}

// GetFieldValue returns a specific field value from a record
func (r *Record) GetFieldValue(fieldName string) (interface{}, bool) {
	value, exists := r.Fields[fieldName]
	return value, exists
}

// GetStringField returns a field as string
func (r *Record) GetStringField(fieldName string) string {
	if value, exists := r.Fields[fieldName]; exists {
		return fmt.Sprintf("%v", value)
	}
	return ""
}

// GetIntField returns a field as int
func (r *Record) GetIntField(fieldName string) (int, error) {
	value, exists := r.Fields[fieldName]
	if !exists {
		return 0, fmt.Errorf("field %s not found", fieldName)
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("field %s is not numeric", fieldName)
	}
}

// GetFloatField returns a field as float64
func (r *Record) GetFloatField(fieldName string) (float64, error) {
	value, exists := r.Fields[fieldName]
	if !exists {
		return 0, fmt.Errorf("field %s not found", fieldName)
	}

	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("field %s is not numeric", fieldName)
	}
}
