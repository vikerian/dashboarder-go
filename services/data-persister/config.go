package main

import "os"

// Config drží nastavení připojení pro MQTT, Postgres a Valkey.
type Config struct {
	MQTTBroker   string
	MQTTClientID string
	InputTopic   string

	// Connection string pro Postgres (TimescaleDB)
	// Formát: postgres://user:password@host:port/dbname
	PostgresURL string

	// Adresa pro Valkey (Redis)
	// Formát: host:port (např. valkey:6379)
	ValkeyAddr string

	LogLevel string
}

func LoadConfig() Config {
	return Config{
		MQTTBroker:   getEnv("MQTT_BROKER", "tcp://mqtt:1883"),
		MQTTClientID: getEnv("MQTT_CLIENT_ID", "data-persister"),
		InputTopic:   getEnv("INPUT_TOPIC", "events/+"), // Zde Ingestor posílá data

		PostgresURL: getEnv("POSTGRES_URL", "postgres://postgres:postgres@timescaledb:5432/iot_db"),
		ValkeyAddr:  getEnv("VALKEY_ADDR", "valkey:6379"),

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
