// main.go
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/evcc-io/evcc/util"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/evcc-io/evcc/provider/mqtt"
	"gopkg.in/yaml.v2"
)

type Config struct {
	MQTT     MqttConfig      `yaml:"mqtt"`
	Vehicles []VehicleConfig `yaml:"vehicles"`
	Log      string          `yaml:"log"`
	Timezone string          `yaml:"timezone"` // New field for timezone
}

type MqttConfig struct {
	Broker     string       `json:"broker"`
	User       string       `json:"user"`
	Password   string       `json:"password"`
	ClientID   string       `json:"clientID"`
	Insecure   bool         `json:"insecure"`
	CaCert     string       `json:"caCert"`
	ClientCert string       `json:"clientCert"`
	ClientKey  string       `json:"clientKey"`
	Topics     TopicsConfig `yaml:"topics"`
}

type TopicsConfig struct {
	Events  string `yaml:"events"`
	PlanSoc string `yaml:"planSoc"`
}

type VehicleConfig struct {
	Name     string          `yaml:"name"`
	SOC      int             `yaml:"soc"`
	Schedule []scheduleEntry `yaml:"schedule"`
}

type scheduleEntry struct {
	Day  string `yaml:"day"`
	Time string `yaml:"time"`
	SOC  *int   `yaml:"soc,omitempty"`
}

// Event payload
type Event struct {
	Vehicle string `json:"vehicle"`
	Mode    string `json:"mode"`
	Type    string `json:"type"`
}

var log *util.Logger

func init() {
	log = util.NewLogger("charging_planner")
	util.LogLevel("info", nil)
}

func main() {

	// Parse command-line options
	configPath := flag.String("config", "", "Path to the YAML configuration file")
	flag.Parse()

	if *configPath == "" {
		log.FATAL.Println("Please provide a configuration file path with -config option")
		os.Exit(1)
	}

	// Read the YAML configuration file using os.ReadFile
	configData, err := os.ReadFile(*configPath)
	if err != nil {
		log.FATAL.Printf("Error reading configuration file: %v", err)
		os.Exit(1)
	}

	config := Config{
		MQTT: MqttConfig{
			Topics: TopicsConfig{
				Events:  "evcc/events",
				PlanSoc: "evcc/vehicles/%s/planSoc/set",
			},
		},
	}
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		log.FATAL.Printf("Error parsing configuration file: %v", err)
		os.Exit(1)
	}

	if config.Log != "" {
		util.LogLevel(config.Log, nil)
	}

	timezone, err := getTimezoneFromConfig(config)
	if err != nil || timezone == nil {
		log.FATAL.Printf("Can't parse timezone from config: %v", err)
		os.Exit(1)
	}
	mConfig := &config.MQTT
	client, err := mqtt.NewClient(log, mConfig.Broker, mConfig.User, mConfig.Password, mConfig.ClientID, 1, mConfig.Insecure, mConfig.CaCert, mConfig.ClientCert, mConfig.ClientKey)
	if err != nil {
		log.FATAL.Printf("Error connecting to MQTT broker: %v", err)
		os.Exit(1)
	}
	defer client.Client.Disconnect(250)

	// Subscribe to the MQTT topic
	topic := config.MQTT.Topics.Events
	err = client.Listen(topic, createMessageHandler(client, &config, timezone))
	if err != nil {
		log.FATAL.Printf("Error subscribing to topic %s: %v", topic, err)
		os.Exit(1)
	}

	// Keep the program running
	select {}
}

func getTimezoneFromConfig(config Config) (*time.Location, error) {
	// Load timezone
	if config.Timezone != "" {
		log.TRACE.Printf("Using timezone: %s", config.Timezone)
		return time.LoadLocation(config.Timezone)
	}
	log.TRACE.Printf("Using local timezone: %s", time.Local)
	return time.Local, nil
}

func createMessageHandler(client *mqtt.Client, config *Config, timezone *time.Location) func(string) {
	if timezone == nil {
		panic("no timezone given")
	}
	return func(msg string) {
		// Parse the message payload
		var event Event
		err := json.Unmarshal([]byte(msg), &event)
		if err != nil {
			log.ERROR.Printf("Error parsing event JSON: %v", err)
			return
		}

		if event.Type != "connect" {
			log.DEBUG.Printf("Ignoring non-connect event %v", event)
			return
		}

		vehicleConfig := extractVehicleConfig(config, event)
		if vehicleConfig == nil {
			log.DEBUG.Printf("Ignoring event for %s (no configuration)", event.Vehicle)
			return
		}

		// Calculate the next charging time and target SOC
		now := time.Now().In(timezone)
		nextChargeTime, targetSOC, err := calculateNextChargeTime(vehicleConfig.Schedule, now, vehicleConfig.SOC, timezone)
		if err != nil {
			log.ERROR.Printf("Error calculating next charge time for %s: %v", event.Vehicle, err)
			return
		}
		log.DEBUG.Printf("Calculated next charging time for %s: %v", event.Vehicle, nextChargeTime)

		err = sendChargingEndTime(targetSOC, nextChargeTime, config.MQTT.Topics.PlanSoc, vehicleConfig.Name, client)
		if err != nil {
			log.ERROR.Printf("Error setting target time %s (target-soc: %d): %v", nextChargeTime, targetSOC, err)
		}
	}
}

func sendChargingEndTime(targetSOC int, nextChargeTime time.Time, planSocTopic string, vehicleId string, client *mqtt.Client) error {
	payloadBytes, err := createSetPlanPayload(targetSOC, nextChargeTime)
	if err != nil {
		return err
	}

	// Publish to the MQTT topic
	publishTopic := fmt.Sprintf(planSocTopic, vehicleId)
	err = client.Publish(publishTopic, false, payloadBytes)
	if err != nil {
		return fmt.Errorf("error publishing to topic %s: %w", publishTopic, err)
	}

	log.DEBUG.Printf("Published to %s for vehicle '%s': %s", publishTopic, vehicleId, string(payloadBytes))
	return nil
}

func createSetPlanPayload(targetSOC int, nextChargeTime time.Time) ([]byte, error) {
	// Prepare the payload
	payloadMap := map[string]interface{}{
		"value": targetSOC,
		"time":  nextChargeTime.Format(time.RFC3339),
	}
	log.TRACE.Printf("Map to serialize: %v", payloadMap)
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, fmt.Errorf("error marshaling payload JSON: %w", err)
	}
	log.TRACE.Printf("Marshaled JSON: %s", string(payloadBytes))
	return payloadBytes, nil
}

func extractVehicleConfig(config *Config, event Event) *VehicleConfig {
	// Check if the vehicle is in the configuration
	for i, v := range config.Vehicles {
		if v.Name == event.Vehicle {
			return &config.Vehicles[i]
		}
	}
	return nil
}

func calculateNextChargeTime(schedule []scheduleEntry, now time.Time, globalSOC int, timezone *time.Location) (time.Time, int, error) {
	var candidateTimes []struct {
		Time time.Time
		SOC  int
	}

	log.TRACE.Printf("Timezone: %s", timezone.String())
	for _, entry := range schedule {
		// Determine the weekdays for this entry
		weekdays, err := parseDays(entry.Day)
		if err != nil {
			return time.Time{}, 0, err
		}

		// Parse the time in the specified timezone
		parsedTime, err := time.ParseInLocation("15:04", entry.Time, timezone)
		if err != nil {
			return time.Time{}, 0, err
		}

		// Use per-day SOC if provided, else use global SOC
		targetSOC := globalSOC
		if entry.SOC != nil {
			targetSOC = *entry.SOC
		}

		for _, weekday := range weekdays {
			// Calculate the candidate date in the specified timezone
			daysUntilWeekday := (int(weekday) - int(now.Weekday()) + 7) % 7
			candidateDate := now.AddDate(0, 0, daysUntilWeekday)
			candidateTime := time.Date(candidateDate.Year(), candidateDate.Month(), candidateDate.Day(), parsedTime.Hour(), parsedTime.Minute(), 0, 0, timezone)
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
