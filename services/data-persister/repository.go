package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Repository zapouzdřuje práci s databázemi.
// Zbytek aplikace (main) neví, jak se píše SQL, jen volá metody repozitáře.
type Repository struct {
	pgPool *pgxpool.Pool // Pool spojení do TimescaleDB
	redis  *redis.Client // Klient pro Valkey
}

// NewRepository vytvoří a ověří připojení k oběma databázím.
func NewRepository(ctx context.Context, cfg Config) (*Repository, error) {
	// 1. Připojení k Postgres
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return nil, fmt.Errorf("chyba konfigurace DB: %w", err)
	}
	// Ověření spojení (Ping)
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("DB není dostupná: %w", err)
	}

	// 2. Připojení k Valkey (Redis)
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.ValkeyAddr,
	})
	// Ověření spojení
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Valkey není dostupný: %w", err)
	}

	return &Repository{pgPool: pool, redis: rdb}, nil
}

// Close uzavře spojení při ukončení aplikace.
func (r *Repository) Close() {
	r.pgPool.Close()
	r.redis.Close()
}

// SaveMeasurement uloží data do obou úložišť (Hot Path & Cold Path).
func (r *Repository) SaveMeasurement(ctx context.Context, event SensorEvent) error {

	// A. Uložení do TimescaleDB (Historie)
	// Toto je naše "Cold Storage" nebo "Source of Truth".
	// INSERT je optimalizovaný pro TimescaleDB hypertable.
	query := `INSERT INTO sensor_data (time, sensor_id, value) VALUES ($1, $2, $3)`

	_, err := r.pgPool.Exec(ctx, query, event.Timestamp, event.SensorID, event.Value)
	if err != nil {
		return fmt.Errorf("chyba insertu do PG: %w", err)
	}

	// B. Uložení do Valkey (Aktuální stav)
	// Toto je "Hot Storage" pro Dashboard. Přepisujeme stále dokola poslední hodnotu.
	// Klíč: "sensor:last:{id}" (např. "sensor:last:5")
	key := fmt.Sprintf("sensor:last:%d", event.SensorID)

	// Ukládáme hodnotu s expirací 24h (aby zmizely mrtvé senzory z cache)
	err = r.redis.Set(ctx, key, event.Value, 24*time.Hour).Err()
	if err != nil {
		// Redis chyba není kritická pro integritu dat (máme je v PG),
		// ale měli bychom o ní vědět.
		return fmt.Errorf("chyba update Valkey: %w", err)
	}

	return nil
}
