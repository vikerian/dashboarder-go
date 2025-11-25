package main

import "os"

// Config drží veškeré nastavení, které aplikace potřebuje k běhu.
// Oddělení konfigurace od kódu (Code vs Config) je základem 12-Factor App metodiky.
// Umožňuje nám nasadit stejný Docker image na dev, test i prod prostředí,
// jen změnou ENV proměnných.
type Config struct {
	// HTTPPort: Port, na kterém bude naslouchat náš webový server (např. "3000").
	HTTPPort string

	// APIURL: Adresa backendové služby (Home API).
	// Dashboard se nepřipojuje k databázi přímo! Funguje jen jako "Frontend",
	// který zobrazuje data získaná z API.
	// Příklad v Docker síti: "http://home-api:8080"
	APIURL string
}

// LoadConfig načte konfiguraci z operačního systému (ENV variables).
// Pokud proměnná není nastavena, použije se fallback (defaultní hodnota).
func LoadConfig() Config {
	return Config{
		HTTPPort: getEnv("HTTP_PORT", "3000"),
		APIURL:   getEnv("API_URL", "http://home-api:8080"),
	}
}

// getEnv je pomocná funkce.
// Go standardní knihovna `os.Getenv` vrací prázdný string, pokud proměnná neexistuje.
// My ale často potřebujeme defaultní hodnotu pro lokální vývoj, proto tento wrapper.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
