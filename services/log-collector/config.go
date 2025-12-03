package main

import (
	"os"
)

// Config drží veškeré nastavení pro službu Log Collector.
// Všechny hodnoty jsou načítány z Environment proměnných, což umožňuje
// flexibilní nasazení (Docker, K8s, Localhost) bez změny kódu.
type Config struct {
	// MQTTBroker: Adresa brokera (např. tcp://mosquitto:1883)
	MQTTBroker string

	// MQTTClientID: Unikátní ID klienta.
	MQTTClientID string

	// LogTopic: Topic, na kterém posloucháme logy (např. "logs/#")
	LogTopic string

	// LogDir: Cesta k adresáři, kam budeme ukládat soubory s logy.
	// V Dockeru to bude typicky namapovaný volume.
	LogDir string
}

// LoadConfig načte konfiguraci z OS. Pokud proměnná chybí, použije default.
func LoadConfig() Config {
	return Config{
		MQTTBroker:   getEnv("MQTT_BROKER", "tcp://mosquitto:1883"),
		MQTTClientID: getEnv("MQTT_CLIENT_ID", "log-collector"),

		// Defaultně posloucháme vše pod logs/
		LogTopic: getEnv("LOG_TOPIC", "logs/#"),

		// Defaultní cesta uvnitř kontejneru
		LogDir: getEnv("LOG_DIR", "/var/log/iot-app"),
	}
}

// getEnv je pomocná funkce pro bezpečné čtení ENV.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
