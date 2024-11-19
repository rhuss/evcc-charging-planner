// main_test.go
package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"testing"
	"time"
)

func TestParseDays(t *testing.T) {
	tests := []struct {
		input    string
		expected []time.Weekday
	}{
		{"Monday", []time.Weekday{time.Monday}},
		{"workday", []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}},
		{"weekend", []time.Weekday{time.Saturday, time.Sunday}},
		{"invalid", nil},
	}

	for _, test := range tests {
		result, err := parseDays(test.input)
		if err != nil && test.expected != nil {
			t.Errorf("parseDays(%s) returned error: %v", test.input, err)
		}
		if err == nil && test.expected == nil {
			t.Errorf("parseDays(%s) expected error but got none", test.input)
		}
		if err == nil && !equalWeekdaySlices(result, test.expected) {
			t.Errorf("parseDays(%s) = %v; want %v", test.input, result, test.expected)
		}
	}
}

func TestCalculateNextChargeTime(t *testing.T) {
	// Define the schedule as per the updated configuration
	schedule := []scheduleEntry{
		{Day: "Monday", Time: "07:00", SOC: intPtr(70)},  // Specific day with SOC
		{Day: "Wednesday", Time: "07:00"},                // Specific day without SOC
		{Day: "Friday", Time: "07:00", SOC: intPtr(90)},  // Specific day with SOC
		{Day: "workday", Time: "08:00", SOC: intPtr(80)}, // Workdays default
		{Day: "weekend", Time: "09:00", SOC: intPtr(60)}, // Weekend default
	}

	// Global SOC for the vehicle
	globalSOC := 80

	// Define test cases covering each day of the week
	testCases := []struct {
		currentTime  string // Use string representation
		expectedTime string // Use string representation
		expectedSOC  int
		description  string
	}{
		{
			currentTime:  "2023-03-04 10:00", // Saturday
			expectedTime: "2023-03-05 09:00", // Sunday at 9
			expectedSOC:  60,
			description:  "Saturday should use weekend schedule",
		},
		{
			currentTime:  "2023-03-05 10:00", // Sunday
			expectedTime: "2023-03-06 07:00", // Monday at 07:00
			expectedSOC:  70,
			description:  "Monday specific schedule should take precedence over workday",
		},
		{
			currentTime:  "2023-03-06 10:00", // Monday
			expectedTime: "2023-03-07 08:00", // Tuesday
			expectedSOC:  80,
			description:  "Tuesday should use workday schedule",
		},
		{
			currentTime:  "2023-03-07 10:00", // Tuesday
			expectedTime: "2023-03-08 07:00", // Wednesday at 07:00
			expectedSOC:  80,
			description:  "Wednesday specific schedule without SOC should use global SOC",
		},
		{
			currentTime:  "2023-03-08 10:00", // Wednesday
			expectedTime: "2023-03-09 08:00", // Thursday at 08:00
			expectedSOC:  80,
			description:  "Thursday should use workday schedule",
		},
		{
			currentTime:  "2023-03-09 10:00", // Thursday
			expectedTime: "2023-03-10 07:00", // Friday at 07:00
			expectedSOC:  90,
			description:  "Friday specific schedule should take precedence over workday",
		},
		{
			currentTime:  "2023-03-10 10:00", // Friday
			expectedTime: "2023-03-11 09:00", // Saturday at 09:00
			expectedSOC:  60,
			description:  "Saturday should use weekend schedule",
		},
	}

	// Run each test case
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Parse currentTime and expectedTime in local time zone
			location := time.Local
			currentTime, err := time.ParseInLocation("2006-01-02 15:04", tc.currentTime, location)
			if err != nil {
				t.Fatalf("Invalid currentTime format: %v", err)
			}
			expectedTime, err := time.ParseInLocation("2006-01-02 15:04", tc.expectedTime, location)
			if err != nil {
				t.Fatalf("Invalid expectedTime format: %v", err)
			}

			tz := time.Local
			nextTime, targetSOC, err := calculateNextChargeTime(schedule, currentTime, globalSOC, tz)
			if err != nil {
				t.Errorf("calculateNextChargeTime returned error: %v", err)
			}
			if !nextTime.Equal(expectedTime) {
				t.Errorf("At time %v, expected next charge time %v, got %v", currentTime, expectedTime, nextTime)
			}
			if targetSOC != tc.expectedSOC {
				t.Errorf("At time %v, expected SOC %d, got %d", currentTime, tc.expectedSOC, targetSOC)
			}
		})
	}
}

func TestCreateSetPlanPayload(t *testing.T) {
	// Define test cases
	testCases := []struct {
		targetSOC      int
		nextChargeTime string // Input time as string in local time
	}{
		{
			targetSOC:      80,
			nextChargeTime: "2023-03-10 07:00",
		},
		{
			targetSOC:      60,
			nextChargeTime: "2023-03-11 09:00",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("SOC %d at %s", tc.targetSOC, tc.nextChargeTime), func(t *testing.T) {
			// Parse nextChargeTime string into time.Time in local time zone
			location := time.Local
			parsedTime, err := time.ParseInLocation("2006-01-02 15:04", tc.nextChargeTime, location)
			if err != nil {
				t.Fatalf("Invalid nextChargeTime format: %v", err)
			}

			// Call createSetPlanPayload
			payloadBytes, err := createSetPlanPayload(tc.targetSOC, parsedTime)
			if err != nil {
				t.Errorf("Error creating set plan payload: %v", err)
			}

			// Unmarshal payload to verify contents
			var payloadMap map[string]interface{}
			err = json.Unmarshal(payloadBytes, &payloadMap)
			if err != nil {
				t.Errorf("Error unmarshaling payload JSON: %v", err)
			}

			// Check 'value' field
			if payloadMap["value"] != float64(tc.targetSOC) {
				t.Errorf("Expected value %d, got %v", tc.targetSOC, payloadMap["value"])
			}

			// Check 'time' field
			payloadTimeStr, ok := payloadMap["time"].(string)
			if !ok {
				t.Errorf("Expected 'time' field to be a string")
				return
			}

			// Parse the time string from the payload
			payloadTime, err := time.Parse(time.RFC3339, payloadTimeStr)
			if err != nil {
				t.Errorf("Error parsing time from payload: %v", err)
				return
			}

			// Verify that the payload time matches the input time
			if !payloadTime.Equal(parsedTime) {
				t.Errorf("Expected time %v, got %v", parsedTime, payloadTime)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name          string
		configContent string
		expectError   bool
	}{
		{
			name: "Happy Path",
			configContent: `
mqtt:
  broker: "tcp://localhost:1883"
  username: "user"
  password: "pass"
  topics:
    events: "bla/blub"
    planSoc: "foo/bar"

vehicles:
  - name: "ioniq6"
    soc: 80
    schedule:
      - day: "Monday"
        time: "07:00"
        soc: 70
      - day: "Wednesday"
        time: "07:00"
      - day: "Friday"
        time: "07:00"
        soc: 90
      - day: "workday"
        time: "08:00"
      - day: "weekend"
        time: "09:00"
        soc: 60
`,
			expectError: false,
		},
		{
			name: "Invalid YAML",
			configContent: `
mqtt:
  broker: "tcp://localhost:1883"
  username "user"  # Missing colon
  password: "pass"

vehicles:
  - name: "ioniq6"
    soc: 80
`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary file
			tmpFile, err := os.CreateTemp("", "config-*.yaml")
			if err != nil {
				t.Fatalf("Failed to create temporary file: %v", err)
			}
			// Ensure the file is removed after the test
			defer os.Remove(tmpFile.Name())

			// Write the config content to the temporary file
			if _, err := tmpFile.Write([]byte(tc.configContent)); err != nil {
				t.Fatalf("Failed to write to temporary file: %v", err)
			}
			// Close the file so it can be read
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temporary file: %v", err)
			}

			// Attempt to read and parse the configuration
			configData, err := os.ReadFile(tmpFile.Name())
			if err != nil {
				t.Fatalf("Error reading configuration file: %v", err)
			}

			var config Config
			err = yaml.Unmarshal(configData, &config)
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else {
					t.Logf("Received expected error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing configuration: %v", err)
				} else {
					// Additional checks can be performed here to verify the config content
					t.Logf("Parsed configuration successfully: %+v", config)
				}
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func equalWeekdaySlices(a, b []time.Weekday) bool {
	if len(a) != len(b) {
		return false
	}
	weekdayMap := make(map[time.Weekday]bool)
	for _, day := range a {
		weekdayMap[day] = true
	}
	for _, day := range b {
		if !weekdayMap[day] {
			return false
		}
	}
	return true
}
