package main

import "os"

// Config zapouzdřuje veškeré nastavení aplikace.
// Umožňuje snadno změnit chování aplikace bez rekompilace (změnou ENV proměnných v Dockeru).
type Config struct {
	// HTTPPort: Port, na kterém bude naslouchat REST API server.
	HTTPPort string

	// PostgresURL: Connection string pro TimescaleDB (čtení historie).
	PostgresURL string

	// ValkeyAddr: Adresa Redis/Valkey serveru (čtení live stavu).
	ValkeyAddr string
}

// LoadConfig načte konfiguraci. Pokud proměnná chybí, použije hardcoded default (pro lokální vývoj).
func LoadConfig() Config {
	return Config{
		HTTPPort:    getEnv("HTTP_PORT", "8080"),
		PostgresURL: getEnv("POSTGRES_URL", "postgres://postgres:postgres@timescaledb:5432/iot_db"),
		ValkeyAddr:  getEnv("VALKEY_ADDR", "valkeydb:6379"),
	}
}

// getEnv je pomocná funkce. Pokud klíč v OS neexistuje, vrátí fallback.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
