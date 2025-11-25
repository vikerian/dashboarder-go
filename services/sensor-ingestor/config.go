package main

import (
	"os"
)

// Config drží konfiguraci celé mikroslužby.
// Používáme princip 12-Factor App - konfigurace je oddělená od kódu v ENV proměnných.
type Config struct {
	// MQTT Konfigurace
	MQTTBroker   string
	MQTTClientID string
	InputTopic   string // Topic s wildcards (např. /msh/#), který posloucháme
	OutputTopic  string // Topic, kam posíláme validovaná data (např. events/data)

	// Databázová Konfigurace
	// Ingestor potřebuje přístup do DB pouze pro čtení (SELECT) metadat a limitů senzorů.
	PostgresURL string

	// App Konfigurace
	LogLevel string
	HTTPPort string
}

// LoadConfig načte nastavení. Pokud proměnná chybí, použije bezpečný default.
func LoadConfig() Config {
	return Config{
		MQTTBroker:   getEnv("MQTT_BROKER", "tcp://mosquitto:1883"),
		MQTTClientID: getEnv("MQTT_CLIENT_ID", "sensor-ingestor"),

		// Posloucháme všechny pod-topicy v /msh/ hierarchii
		InputTopic:  getEnv("INPUT_TOPIC", "/msh/#"),
		OutputTopic: getEnv("OUTPUT_TOPIC", "events/data"),

		// Defaultní connection string (upravit dle docker-compose)
		PostgresURL: getEnv("POSTGRES_URL", "postgres://postgres:postgres@timescaledb:5432/iot_db"),

		LogLevel: getEnv("LOG_LEVEL", "info"),
		HTTPPort: getEnv("HTTP_PORT", "8080"),
	}
}

// getEnv je pomocná funkce pro DRY (Don't Repeat Yourself).
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
