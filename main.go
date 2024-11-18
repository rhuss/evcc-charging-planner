// main.go
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v2"
)

type Config struct {
	MQTT struct {
		Broker   string `yaml:"broker"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"mqtt"`
	Vehicles []VehicleConfig `yaml:"vehicles"`
}

type VehicleConfig struct {
	Name     string          `yaml:"name"`
	SOC      int             `yaml:"soc"` // Changed from target_soc to soc
	Schedule []ScheduleEntry `yaml:"schedule"`
}

type ScheduleEntry struct {
	Day  string `yaml:"day"`  // Changed from weekday to day
	Time string `yaml:"time"` // Changed from end_time to time
	SOC  *int   `yaml:"soc,omitempty"`
}

type Event struct {
	Vehicle string `json:"vehicle"`
	Mode    string `json:"mode"`
	Type    string `json:"type"`
}

func main() {
	// Parse command-line options
	configPath := flag.String("config", "", "Path to the YAML configuration file")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("Please provide a configuration file path with -config option")
	}

	// Read the YAML configuration file using os.ReadFile
	configData, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Error reading configuration file: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		log.Fatalf("Error parsing configuration file: %v", err)
	}

	// Connect to MQTT broker
	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.MQTT.Broker)
	opts.SetUsername(config.MQTT.Username)
	opts.SetPassword(config.MQTT.Password)
	opts.SetAutoReconnect(true)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Error connecting to MQTT broker: %v", token.Error())
	}
	defer client.Disconnect(250)

	// Subscribe to the MQTT topic
	topic := "evcc/events"
	token := client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		handleMessage(client, msg, &config)
	})
	if token.Wait() && token.Error() != nil {
		log.Fatalf("Error subscribing to topic %s: %v", topic, token.Error())
	}

	// Keep the program running
	select {}
}

func handleMessage(client mqtt.Client, msg mqtt.Message, config *Config) {
	// Parse the message payload
	var event Event
	err := json.Unmarshal(msg.Payload(), &event)
	if err != nil {
		log.Printf("Error parsing event JSON: %v", err)
		return
	}

	// Check if the vehicle is in the configuration
	var vehicleConfig *VehicleConfig
	for i, v := range config.Vehicles {
		if v.Name == event.Vehicle {
			vehicleConfig = &config.Vehicles[i]
			break
		}
	}

	if vehicleConfig == nil {
		// Vehicle not in configuration; ignore
		return
	}

	// Calculate the next charging time and target SOC
	nextChargeTime, targetSOC, err := calculateNextChargeTime(vehicleConfig.Schedule, time.Now(), vehicleConfig.SOC)
	if err != nil {
		log.Printf("Error calculating next charge time: %v", err)
		return
	}

	// Prepare the payload
	payloadMap := map[string]interface{}{
		"value": targetSOC,
		"time":  nextChargeTime.Format(time.RFC3339),
	}
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		log.Printf("Error marshaling payload JSON: %v", err)
		return
	}

	// Publish to the MQTT topic
	publishTopic := fmt.Sprintf("evcc/events/%s/planSoc", vehicleConfig.Name)
	token := client.Publish(publishTopic, 0, false, payloadBytes)
	if token.Wait() && token.Error() != nil {
		log.Printf("Error publishing to topic %s: %v", publishTopic, token.Error())
		return
	}

	log.Printf("Published planSoc for vehicle %s: %s", vehicleConfig.Name, string(payloadBytes))
}

func calculateNextChargeTime(schedule []ScheduleEntry, now time.Time, globalSOC int) (time.Time, int, error) {
	var candidateTimes []struct {
		Time time.Time
		SOC  int
	}

	location := time.Local // Use local time zone

	for _, entry := range schedule {
		// Determine the weekdays for this entry
		weekdays, err := parseDays(entry.Day)
		if err != nil {
			return time.Time{}, 0, err
		}

		// Parse the time in local time zone
		parsedTime, err := time.ParseInLocation("15:04", entry.Time, location)
		if err != nil {
			return time.Time{}, 0, err
		}

		// Use per-day SOC if provided, else use global SOC
		targetSOC := globalSOC
		if entry.SOC != nil {
			targetSOC = *entry.SOC
		}

		for _, weekday := range weekdays {
			// Calculate the candidate date in local time zone
			daysUntilWeekday := (int(weekday) - int(now.Weekday()) + 7) % 7
			candidateDate := now.AddDate(0, 0, daysUntilWeekday)
			candidateTime := time.Date(candidateDate.Year(), candidateDate.Month(), candidateDate.Day(), parsedTime.Hour(), parsedTime.Minute(), 0, 0, location)
			if candidateTime.Before(now) {
				// If the candidate time is before now, add 7 days
				candidateTime = candidateTime.AddDate(0, 0, 7)
			}
			candidateTimes = append(candidateTimes, struct {
				Time time.Time
				SOC  int
			}{Time: candidateTime, SOC: targetSOC})
		}
	}

	// Find the earliest candidate time
	if len(candidateTimes) == 0 {
		return time.Time{}, 0, errors.New("no candidate times found")
	}
	sort.Slice(candidateTimes, func(i, j int) bool {
		return candidateTimes[i].Time.Before(candidateTimes[j].Time)
	})
	return candidateTimes[0].Time, candidateTimes[0].SOC, nil
}

func parseDays(s string) ([]time.Weekday, error) {
	switch strings.ToLower(s) {
	case "sunday":
		return []time.Weekday{time.Sunday}, nil
	case "monday":
		return []time.Weekday{time.Monday}, nil
	case "tuesday":
		return []time.Weekday{time.Tuesday}, nil
	case "wednesday":
		return []time.Weekday{time.Wednesday}, nil
	case "thursday":
		return []time.Weekday{time.Thursday}, nil
	case "friday":
		return []time.Weekday{time.Friday}, nil
	case "saturday":
		return []time.Weekday{time.Saturday}, nil
	case "workday":
		return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}, nil
	case "weekend":
		return []time.Weekday{time.Saturday, time.Sunday}, nil
	default:
		return nil, fmt.Errorf("invalid day: %s", s)
	}
}
