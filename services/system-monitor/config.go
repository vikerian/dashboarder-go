package main

import (
	"os"
	"time"
)

type Config struct {
	MQTTBroker   string
	MQTTClientID string

	// Interval měření (např. "60s", "1m")
	Interval time.Duration
}

func LoadConfig() Config {
	intervalStr := getEnv("MONITOR_INTERVAL", "60s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 60 * time.Second
	}

	return Config{
		MQTTBroker:   getEnv("MQTT_BROKER", "tcp://mqtt:1883"),
		MQTTClientID: getEnv("MQTT_CLIENT_ID", "system-monitor"),
		Interval:     interval,
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
