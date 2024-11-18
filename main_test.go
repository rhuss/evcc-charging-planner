// main_test.go
package main

import (
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
	schedule := []ScheduleEntry{
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

			nextTime, targetSOC, err := calculateNextChargeTime(schedule, currentTime, globalSOC)
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
