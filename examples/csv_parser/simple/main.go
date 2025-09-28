package main

import (
	"encoding/json"
	"fmt"

	"github.com/loykin/freader/pkg/parser/csv"
)

func main() {
	fmt.Println("=== CSV Parser Simple Test ===")

	// Test 1: Access log with headers
	fmt.Println("\n--- Test 1: Access Log with Headers ---")
	parser1 := csv.NewParser(csv.Config{
		Delimiter:       ',',
		HasHeaders:      true,
		AutoDetectTypes: true,
	})

	// Header line (should be skipped)
	result, _ := parser1.Parse("timestamp,ip,method,path,status,bytes")
	fmt.Printf("Header result: %v\n", result)

	// Data line
	dataLine := "2023-12-01 10:00:01,192.168.1.100,GET,/api/users,200,1024"
	record, err := parser1.Parse(dataLine)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Input: %s\n", dataLine)
		fmt.Printf("IP: %s\n", record.GetStringField("ip"))

		status, _ := record.GetIntField("status")
		fmt.Printf("Status: %d\n", status)

		bytes, _ := record.GetIntField("bytes")
		fmt.Printf("Bytes: %d\n", bytes)

		jsonData, _ := json.MarshalIndent(record, "", "  ")
		fmt.Printf("JSON:\n%s\n", string(jsonData))
	}

	// Test 2: Sensor data without headers
	fmt.Println("\n--- Test 2: Sensor Data without Headers ---")
	parser2 := csv.NewParser(csv.Config{
		Delimiter:       ',',
		HasHeaders:      false,
		Headers:         []string{"timestamp", "sensor_id", "temperature", "humidity", "active"},
		AutoDetectTypes: true,
	})

	sensorLine := "2023-12-01T10:00:00Z,sensor_001,22.5,65.2,true"
	record2, err := parser2.Parse(sensorLine)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Input: %s\n", sensorLine)
		fmt.Printf("Sensor ID: %s\n", record2.GetStringField("sensor_id"))

		temp, _ := record2.GetFloatField("temperature")
		fmt.Printf("Temperature: %.1f°C\n", temp)

		humidity, _ := record2.GetFloatField("humidity")
		fmt.Printf("Humidity: %.1f%%\n", humidity)

		active, _ := record2.GetFieldValue("active")
		fmt.Printf("Active: %v (type: %T)\n", active, active)

		jsonData, _ := json.MarshalIndent(record2, "", "  ")
		fmt.Printf("JSON:\n%s\n", string(jsonData))
	}

	// Test 3: Tab-separated values
	fmt.Println("\n--- Test 3: Tab-Separated Values ---")
	parser3 := csv.NewParser(csv.Config{
		Delimiter:       '\t',
		HasHeaders:      false,
		Headers:         []string{"category", "name", "value", "active"},
		AutoDetectTypes: true,
	})

	tsvLine := "performance\tcpu_usage\t85.5\ttrue"
	record3, err := parser3.Parse(tsvLine)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Input: %s\n", tsvLine)
		fmt.Printf("Category: %s\n", record3.GetStringField("category"))

		value, _ := record3.GetFloatField("value")
		fmt.Printf("Value: %.1f\n", value)

		jsonData, _ := json.MarshalIndent(record3, "", "  ")
		fmt.Printf("JSON:\n%s\n", string(jsonData))
	}

	// Test 4: Quoted fields with commas
	fmt.Println("\n--- Test 4: Quoted Fields ---")
	parser4 := csv.NewParser(csv.Config{
		Delimiter:       ',',
		HasHeaders:      false,
		Headers:         []string{"name", "role", "location", "active"},
		AutoDetectTypes: true,
	})

	quotedLine := `"John Doe","Software Engineer","San Francisco, CA",true`
	record4, err := parser4.Parse(quotedLine)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Input: %s\n", quotedLine)
		fmt.Printf("Name: %s\n", record4.GetStringField("name"))
		fmt.Printf("Role: %s\n", record4.GetStringField("role"))
		fmt.Printf("Location: %s\n", record4.GetStringField("location"))

		jsonData, _ := json.MarshalIndent(record4, "", "  ")
		fmt.Printf("JSON:\n%s\n", string(jsonData))
	}

	fmt.Println("\n=== Test Completed ===")
}
