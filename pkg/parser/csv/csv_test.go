package csv

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSVParser_Parse_WithHeaders(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:       ',',
		HasHeaders:      true,
		AutoDetectTypes: true,
	})

	// First line should be skipped (headers)
	headerLine := "timestamp,level,message,count"
	result, err := parser.Parse(headerLine)
	require.NoError(t, err)
	assert.Nil(t, result) // Headers are skipped

	// Data line should be parsed
	dataLine := "2023-12-01 10:00:00,INFO,Test message,42"
	result, err = parser.Parse(dataLine)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, dataLine, result.Raw)
	assert.Equal(t, 2, result.LineNum) // Second line
	// Check if timestamp was auto-detected as time.Time or kept as string
	timestampField := result.GetStringField("timestamp")
	assert.True(t, strings.Contains(timestampField, "2023-12-01 10:00:00"))
	assert.Equal(t, "INFO", result.GetStringField("level"))
	assert.Equal(t, "Test message", result.GetStringField("message"))

	// Should auto-detect integer
	count, err := result.GetIntField("count")
	require.NoError(t, err)
	assert.Equal(t, 42, count)
}

func TestCSVParser_Parse_WithoutHeaders(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:       ',',
		HasHeaders:      false,
		AutoDetectTypes: true,
	})

	dataLine := "2023-12-01 10:00:00,INFO,Test message,42"
	result, err := parser.Parse(dataLine)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should generate default headers
	timestampField := result.GetStringField("field_1")
	assert.True(t, strings.Contains(timestampField, "2023-12-01 10:00:00"))
	assert.Equal(t, "INFO", result.GetStringField("field_2"))
	assert.Equal(t, "Test message", result.GetStringField("field_3"))

	count, err := result.GetIntField("field_4")
	require.NoError(t, err)
	assert.Equal(t, 42, count)
}

func TestCSVParser_Parse_CustomHeaders(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:       ',',
		HasHeaders:      false,
		Headers:         []string{"time", "severity", "msg", "num"},
		AutoDetectTypes: true,
	})

	dataLine := "2023-12-01 10:00:00,ERROR,Something failed,123"
	result, err := parser.Parse(dataLine)
	require.NoError(t, err)
	require.NotNil(t, result)

	timeField := result.GetStringField("time")
	assert.True(t, strings.Contains(timeField, "2023-12-01 10:00:00"))
	assert.Equal(t, "ERROR", result.GetStringField("severity"))
	assert.Equal(t, "Something failed", result.GetStringField("msg"))

	num, err := result.GetIntField("num")
	require.NoError(t, err)
	assert.Equal(t, 123, num)
}

func TestCSVParser_TypeDetection(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:       ',',
		HasHeaders:      false,
		Headers:         []string{"str", "int", "float", "bool_true", "bool_false", "empty"},
		AutoDetectTypes: true,
	})

	dataLine := "hello,123,45.67,true,false,"
	result, err := parser.Parse(dataLine)
	require.NoError(t, err)
	require.NotNil(t, result)

	// String
	assert.Equal(t, "hello", result.Fields["str"])

	// Integer
	assert.Equal(t, 123, result.Fields["int"])

	// Float
	assert.Equal(t, 45.67, result.Fields["float"])

	// Booleans
	assert.Equal(t, true, result.Fields["bool_true"])
	assert.Equal(t, false, result.Fields["bool_false"])

	// Empty
	assert.Equal(t, "", result.Fields["empty"])
}

func TestCSVParser_TimestampParsing(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:       ',',
		HasHeaders:      false,
		Headers:         []string{"timestamp", "message"},
		TimestampField:  "timestamp",
		TimestampFormat: "2006-01-02 15:04:05",
	})

	dataLine := "2023-12-01 10:30:45,Test message"
	result, err := parser.Parse(dataLine)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have parsed timestamp
	parsedTime, exists := result.Fields["timestamp_parsed"]
	require.True(t, exists)

	expectedTime := time.Date(2023, 12, 1, 10, 30, 45, 0, time.UTC)
	assert.Equal(t, expectedTime, parsedTime)
}

func TestCSVParser_DifferentDelimiters(t *testing.T) {
	tests := []struct {
		name      string
		delimiter rune
		input     string
		expected  []string
	}{
		{
			name:      "Comma delimiter",
			delimiter: ',',
			input:     "a,b,c",
			expected:  []string{"a", "b", "c"},
		},
		{
			name:      "Semicolon delimiter",
			delimiter: ';',
			input:     "a;b;c",
			expected:  []string{"a", "b", "c"},
		},
		{
			name:      "Tab delimiter",
			delimiter: '\t',
			input:     "a\tb\tc",
			expected:  []string{"a", "b", "c"},
		},
		{
			name:      "Pipe delimiter",
			delimiter: '|',
			input:     "a|b|c",
			expected:  []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(Config{
				Delimiter:  tt.delimiter,
				HasHeaders: false,
				Headers:    []string{"field1", "field2", "field3"},
			})

			result, err := parser.Parse(tt.input)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.expected[0], result.GetStringField("field1"))
			assert.Equal(t, tt.expected[1], result.GetStringField("field2"))
			assert.Equal(t, tt.expected[2], result.GetStringField("field3"))
		})
	}
}

func TestCSVParser_QuotedFields(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:  ',',
		HasHeaders: false,
		Headers:    []string{"field1", "field2", "field3"},
	})

	// Test quoted fields with commas inside
	input := `"hello, world","normal field","another, quoted, field"`
	result, err := parser.Parse(input)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "hello, world", result.GetStringField("field1"))
	assert.Equal(t, "normal field", result.GetStringField("field2"))
	assert.Equal(t, "another, quoted, field", result.GetStringField("field3"))
}

func TestCSVParser_ParseJSON(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:       ',',
		HasHeaders:      false,
		Headers:         []string{"name", "age", "active"},
		AutoDetectTypes: true,
	})

	input := "John Doe,30,true"
	jsonBytes, err := parser.ParseJSON(input)
	require.NoError(t, err)

	var result Record
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	assert.Equal(t, input, result.Raw)
	assert.Equal(t, "John Doe", result.Fields["name"])
	assert.Equal(t, float64(30), result.Fields["age"]) // JSON numbers are float64
	assert.Equal(t, true, result.Fields["active"])
}

func TestCSVParser_Reset(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:  ',',
		HasHeaders: true,
	})

	// Parse headers
	_, err := parser.Parse("col1,col2,col3")
	require.NoError(t, err)

	// Parse data
	result, err := parser.Parse("a,b,c")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.LineNum)

	// Reset parser
	parser.Reset()

	// Parse headers again
	_, err = parser.Parse("newcol1,newcol2")
	require.NoError(t, err)

	// Parse data - should start from line 1 again
	result, err = parser.Parse("x,y")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.LineNum)
	assert.Equal(t, "x", result.GetStringField("newcol1"))
}

func TestCSVParser_ErrorHandling(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:  ',',
		HasHeaders: false,
		Headers:    []string{"field1"},
	})

	// Test malformed CSV
	_, err := parser.Parse(`"unclosed quote`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse CSV line")
}

func TestRecord_FieldAccessors(t *testing.T) {
	record := &Record{
		Fields: map[string]interface{}{
			"string_field": "hello",
			"int_field":    42,
			"float_field":  3.14,
			"bool_field":   true,
		},
	}

	// Test GetStringField
	assert.Equal(t, "hello", record.GetStringField("string_field"))
	assert.Equal(t, "42", record.GetStringField("int_field"))
	assert.Equal(t, "", record.GetStringField("nonexistent"))

	// Test GetIntField
	intVal, err := record.GetIntField("int_field")
	require.NoError(t, err)
	assert.Equal(t, 42, intVal)

	floatAsInt, err := record.GetIntField("float_field")
	require.NoError(t, err)
	assert.Equal(t, 3, floatAsInt)

	_, err = record.GetIntField("nonexistent")
	assert.Error(t, err)

	// Test GetFloatField
	floatVal, err := record.GetFloatField("float_field")
	require.NoError(t, err)
	assert.Equal(t, 3.14, floatVal)

	intAsFloat, err := record.GetFloatField("int_field")
	require.NoError(t, err)
	assert.Equal(t, 42.0, intAsFloat)

	_, err = record.GetFloatField("nonexistent")
	assert.Error(t, err)
}

func TestCSVParser_RealWorldExample(t *testing.T) {
	parser := NewParser(Config{
		Delimiter:       ',',
		HasHeaders:      true,
		AutoDetectTypes: true,
	})

	// Simulate server access log in CSV format
	lines := []string{
		"timestamp,ip,method,path,status,bytes,duration",
		"2023-12-01 10:00:01,192.168.1.100,GET,/api/users,200,1024,0.045",
		"2023-12-01 10:00:02,192.168.1.101,POST,/api/login,401,512,0.012",
		"2023-12-01 10:00:03,192.168.1.100,GET,/api/data,500,256,1.234",
	}

	var results []*Record
	for _, line := range lines {
		result, err := parser.Parse(line)
		require.NoError(t, err)
		if result != nil { // Skip header line
			results = append(results, result)
		}
	}

	require.Len(t, results, 3)

	// Check first record
	first := results[0]
	assert.Equal(t, "192.168.1.100", first.GetStringField("ip"))
	assert.Equal(t, "GET", first.GetStringField("method"))

	status, err := first.GetIntField("status")
	require.NoError(t, err)
	assert.Equal(t, 200, status)

	duration, err := first.GetFloatField("duration")
	require.NoError(t, err)
	assert.Equal(t, 0.045, duration)
}
