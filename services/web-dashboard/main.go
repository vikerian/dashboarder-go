package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	// 1. Inicializace Loggeru
	// Používáme strukturovaný JSON logger, což je standard pro kontejnerizované aplikace (Docker/K8s).
	// Umožňuje snadné parsování logů nástroji jako ELK stack nebo Grafana Loki.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 2. Načtení Konfigurace
	cfg := LoadConfig()
	logger.Info("Startuji Web Dashboard", "port", cfg.HTTPPort, "api_url", cfg.APIURL)

	// 3. Inicializace komponent (Dependency Injection)
	// Vytvoříme klienta, který umí komunikovat s API.
	client := NewAPIClient(cfg.APIURL)

	// Vytvoříme handler a předáme mu klienta a logger.
	// Pokud handler vrátí chybu (např. nenajde šablony), ukončíme program.
	handler, err := NewWebHandler(client, logger)
	if err != nil {
		logger.Error("Kritická chyba: Nepodařilo se načíst HTML šablony", "error", err)
		os.Exit(1)
	}

	// 4. Nastavení Routování (ServeMux)
	// ServeMux je HTTP router ze standardní knihovny.
	mux := http.NewServeMux()

	// Mapování URL cest na metody handleru
	mux.HandleFunc("GET /", handler.HandleIndex)

	// {id} je "wildcard" (parametr cesty), dostupný od Go 1.22.
	mux.HandleFunc("GET /sensor/{id}", handler.HandleDetail)

	// Healthcheck endpoint pro Docker (aby věděl, že služba žije)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// 5. Spuštění HTTP serveru
	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort, // např. ":3000"
		Handler: mux,
	}

	logger.Info("Web server naslouchá", "address", server.Addr)

	// ListenAndServe spustí smyčku serveru. Je to blokující volání (program zde "visí").
	// Pokud server spadne (vrátí error), logujeme to a ukončíme proces s kódem 1.
	if err := server.ListenAndServe(); err != nil {
		logger.Error("Server nečekaně spadl", "error", err)
		os.Exit(1)
	}
}
