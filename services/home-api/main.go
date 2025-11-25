package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Nastavení logování na JSON (standard pro kontejnery)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 2. Načtení konfigurace
	cfg := LoadConfig()
	logger.Info("Startuji Home API", "port", cfg.HTTPPort)

	ctx := context.Background()

	// 3. Připojení k Databázi (Postgres/TimescaleDB)
	// pgxpool vytvoří sadu spojení, které se recyklují (Thread-safe).
	dbPool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("Kritická chyba: Nelze se připojit k DB", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close() // Zajistí uzavření při ukončení main()

	// 4. Připojení k Valkey (Redis)
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.ValkeyAddr,
	})
	// Rychlý Ping pro ověření, že Redis žije
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Error("Kritická chyba: Nelze se připojit k Valkey", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()

	// 5. Inicializace komponent (Wiring)
	// Vytvoříme službu s připojenými DB
	svc := NewService(dbPool, rdb)
	// Vytvoříme API handler, který používá službu
	api := NewAPIHandler(svc, logger)

	// 6. Nastavení Routeru
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Jednoduchý healthcheck pro Docker
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// 7. Spuštění HTTP serveru
	// Handler obalíme CorsMiddlewarem, aby fungovalo volání z frontendu.
	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: CorsMiddleware(mux),
	}

	logger.Info("HTTP server naslouchá", "address", server.Addr)

	// ListenAndServe je blokující volání - zde program "visí" a obsluhuje requesty.
	if err := server.ListenAndServe(); err != nil {
		logger.Error("Server spadl", "error", err)
		os.Exit(1)
	}
}
