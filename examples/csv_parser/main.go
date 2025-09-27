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
	"github.com/loykin/freader/pkg/parser/csv"
)

// This example demonstrates how to use freader's Collector to tail CSV log files
// and parse each line using the CSV parser with various configurations.
func main() {
	logDir := filepath.Join("examples", "csv_parser", "log")
	_ = os.MkdirAll(logDir, 0o755)

	// Create different CSV files to demonstrate various parser configurations
	accessLogFile := filepath.Join(logDir, "access.csv")
	sensorDataFile := filepath.Join(logDir, "sensors.csv")
	customDelimiterFile := filepath.Join(logDir, "data.tsv")

	// Seed files if they don't exist
	if _, err := os.Stat(accessLogFile); err != nil {
		seedAccessLog(accessLogFile)
	}
	if _, err := os.Stat(sensorDataFile); err != nil {
		seedSensorData(sensorDataFile)
	}
	if _, err := os.Stat(customDelimiterFile); err != nil {
		seedCustomDelimiter(customDelimiterFile)
	}

	stopMetrics, err := freader.StartMetrics(":2114")
	if err != nil {
		slog.Warn("metrics not started", "err", err)
	}
	defer func() {
		if stopMetrics != nil {
			_ = stopMetrics()
		}
	}()

	// Create different parsers for different file types
	parsers := map[string]*csv.Parser{
		"access.csv": csv.NewParser(csv.Config{
			Delimiter:       ',',
			HasHeaders:      true,
			AutoDetectTypes: true,
			TimestampField:  "timestamp",
			TimestampFormat: "2006-01-02 15:04:05",
		}),
		"sensors.csv": csv.NewParser(csv.Config{
			Delimiter:       ',',
			HasHeaders:      false,
			Headers:         []string{"timestamp", "sensor_id", "temperature", "humidity", "active"},
			AutoDetectTypes: true,
			TimestampField:  "timestamp",
			TimestampFormat: "2006-01-02T15:04:05Z",
		}),
		"data.tsv": csv.NewParser(csv.Config{
			Delimiter:       '\t', // Tab-separated
			HasHeaders:      true,
			AutoDetectTypes: true,
		}),
	}

	cfg := freader.Config{
		Include:             []string{filepath.Join(logDir, "*")},
		Exclude:             nil,
		Separator:           "\n",
		PollInterval:        200 * time.Millisecond,
		FingerprintStrategy: freader.FingerprintStrategyChecksum,
		FingerprintSize:     1024,
		StoreOffsets:        false,
		DBPath:              "",
		Multiline:           nil,
		OnLineFunc: func(line string) {
			// Skip empty lines
			if strings.TrimSpace(line) == "" {
				return
			}

			// Determine which parser to use based on file content patterns
			var parser *csv.Parser
			var fileType string

			// Simple heuristic to determine file type
			if strings.Contains(line, "GET") || strings.Contains(line, "POST") {
				parser = parsers["access.csv"]
				fileType = "access.csv"
			} else if strings.Contains(line, "sensor_") {
				parser = parsers["sensors.csv"]
				fileType = "sensors.csv"
			} else if strings.Contains(line, "\t") {
				parser = parsers["data.tsv"]
				fileType = "data.tsv"
			} else {
				// Default to access log parser
				parser = parsers["access.csv"]
				fileType = "default"
			}

			record, err := parser.Parse(line)
			if err != nil {
				slog.Warn("failed to parse CSV line", "error", err, "line", line, "type", fileType)
				return
			}
			if record == nil {
				return // Header line or empty
			}

			fmt.Printf("📄 File Type: %s\n", fileType)

			// Print parsed record as JSON
			b, _ := json.MarshalIndent(record, "", "  ")
			fmt.Println(string(b))

			// Demonstrate field access based on file type
			switch fileType {
			case "access.csv":
				status, err := record.GetIntField("status")
				if err == nil && status >= 400 {
					fmt.Printf("🚨 HTTP Error: %d - %s %s\n",
						status, record.GetStringField("method"), record.GetStringField("path"))
				}

				bytes, err := record.GetIntField("bytes")
				if err == nil && bytes > 10000 {
					fmt.Printf("📊 Large Response: %d bytes to %s\n", bytes, record.GetStringField("ip"))
				}

			case "sensors.csv":
				temp, err := record.GetFloatField("temperature")
				if err == nil {
					if temp > 30.0 {
						fmt.Printf("🌡️  High Temperature Alert: %.1f°C from %s\n",
							temp, record.GetStringField("sensor_id"))
					}
				}

				humidity, err := record.GetFloatField("humidity")
				if err == nil && humidity < 30.0 {
					fmt.Printf("💧 Low Humidity Warning: %.1f%% from %s\n",
						humidity, record.GetStringField("sensor_id"))
				}

			case "data.tsv":
				value, err := record.GetFloatField("value")
				if err == nil && value > 100.0 {
					fmt.Printf("📈 High Value Alert: %.2f in %s\n",
						value, record.GetStringField("category"))
				}
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

	fmt.Println("Monitoring CSV logs for 8 seconds...")
	fmt.Println("Adding more sample data...")

	// Add more sample data after delays
	go func() {
		time.Sleep(1 * time.Second)
		addMoreAccessLog(accessLogFile)
	}()

	go func() {
		time.Sleep(3 * time.Second)
		addMoreSensorData(sensorDataFile)
	}()

	go func() {
		time.Sleep(5 * time.Second)
		addMoreCustomData(customDelimiterFile)
	}()

	// Keep running for a while to demonstrate parsing
	t := time.NewTimer(8 * time.Second)
	<-t.C
	fmt.Println("CSV parsing example completed!")
}

func seedAccessLog(path string) {
	lines := []string{
		"timestamp,ip,method,path,status,bytes,duration",
		"2023-12-01 10:00:01,192.168.1.100,GET,/api/users,200,1024,0.045",
		"2023-12-01 10:00:02,192.168.1.101,POST,/api/login,401,512,0.012",
		"2023-12-01 10:00:03,192.168.1.100,GET,/api/data,500,256,1.234",
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func seedSensorData(path string) {
	lines := []string{
		"2023-12-01T10:00:00Z,sensor_001,22.5,65.2,true",
		"2023-12-01T10:01:00Z,sensor_002,24.1,58.7,true",
		"2023-12-01T10:02:00Z,sensor_003,19.8,72.3,false",
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func seedCustomDelimiter(path string) {
	lines := []string{
		"category\tname\tvalue\tactive",
		"performance\tcpu_usage\t85.5\ttrue",
		"performance\tmemory_usage\t67.2\ttrue",
		"network\tbandwidth\t156.8\tfalse",
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func addMoreAccessLog(path string) {
	additionalLines := []string{
		"2023-12-01 10:01:01,192.168.1.102,GET,/api/health,200,128,0.003",
		"2023-12-01 10:01:02,192.168.1.103,POST,/api/upload,413,0,0.001",
		"2023-12-01 10:01:03,192.168.1.104,GET,/large/file.zip,200,15728640,5.678",
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	for _, line := range additionalLines {
		_, _ = file.WriteString(line + "\n")
		time.Sleep(400 * time.Millisecond)
	}
}

func addMoreSensorData(path string) {
	additionalLines := []string{
		"2023-12-01T10:03:00Z,sensor_001,31.2,45.8,true",  // High temp
		"2023-12-01T10:04:00Z,sensor_004,18.5,25.1,true",  // Low humidity
		"2023-12-01T10:05:00Z,sensor_002,35.7,38.9,false", // High temp, low humidity
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	for _, line := range additionalLines {
		_, _ = file.WriteString(line + "\n")
		time.Sleep(400 * time.Millisecond)
	}
}

func addMoreCustomData(path string) {
	additionalLines := []string{
		"storage\tdisk_usage\t156.7\ttrue", // High value
		"security\tfailed_logins\t12.0\ttrue",
		"performance\tresponse_time\t234.5\tfalse", // High value
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	for _, line := range additionalLines {
		_, _ = file.WriteString(line + "\n")
		time.Sleep(400 * time.Millisecond)
	}
}
