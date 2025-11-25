package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SensorMetadata drží informace o senzoru nutné pro zpracování zprávy.
// Slouží jako cache záznamu z tabulek 'sensors' a 'sensor_types'.
type SensorMetadata struct {
	ID int64 // ID z tabulky sensors (bude se ukládat do data tabulky)

	// Používáme pointery (*float64), protože limity v DB mohou být NULL.
	// nil = limit není nastaven.
	MinValue *float64
	MaxValue *float64
}

// MetadataService se stará o načítání a poskytování informací o senzorech.
// Implementuje thread-safe přístup k mapě (cache).
type MetadataService struct {
	db     *pgxpool.Pool // Connection pool do DB
	logger *slog.Logger

	// mu (RWMutex) chrání mapu 'cache' před souběžným zápisem a čtením.
	// V Go "map" NENÍ thread-safe. Pokud by jedna goroutina zapisovala a druhá četla, program spadne (panic).
	mu sync.RWMutex

	// Klíč mapy je MQTT Topic (string), hodnota jsou metadata.
	// Příklad: "/msh/internal_temp/ds1" -> {ID: 5, Min: -20, Max: 80}
	cache map[string]SensorMetadata
}

// NewMetadataService - konstruktor
func NewMetadataService(db *pgxpool.Pool, logger *slog.Logger) *MetadataService {
	return &MetadataService{
		db:     db,
		logger: logger,
		cache:  make(map[string]SensorMetadata),
	}
}

// LoadSensors provede SQL dotaz a aktualizuje lokální cache v paměti.
// Tato operace je "drahá" (IO, síť), proto ji děláme jen při startu nebo periodicky.
func (s *MetadataService) LoadSensors(ctx context.Context) error {
	s.logger.Info("Starting sensor metadata refresh from DB...")

	// SQL DOTAZ: Spojuje tabulku senzorů s jejich typy, abychom získali limity.
	// Filtrujeme jen aktivní senzory (is_active = true).
	query := `
		SELECT 
			s.mqtt_topic, 
			s.id, 
			st.min_value, 
			st.max_value
		FROM sensors s
		JOIN sensor_types st ON s.sensor_type_id = st.id
		WHERE s.is_active = true
	`

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("SQL query failed: %w", err)
	}
	defer rows.Close() // Důležité: Vždy zavřít rows pro uvolnění spojení v poolu

	// Vytvoříme novou, dočasnou mapu.
	// Důvod: Nechceme blokovat hlavní cache (zámkem) po celou dobu iterace přes DB.
	newCache := make(map[string]SensorMetadata)
	count := 0

	for rows.Next() {
		var topic string
		var meta SensorMetadata

		// Scan mapuje sloupce z SELECTu do proměnných.
		// Pokud je v DB hodnota NULL, pgx ji umí nahrát do pointeru (*float64).
		if err := rows.Scan(&topic, &meta.ID, &meta.MinValue, &meta.MaxValue); err != nil {
			s.logger.Error("Failed to scan row", "error", err)
			continue
		}

		newCache[topic] = meta
		count++
	}

	// KRITICKÁ SEKCE (Critical Section)
	// Zde na zlomek vteřiny zamkneme cache pro zápis a prohodíme pointery.
	// Pattern "Read-Copy-Update": Připravili jsme data bokem a teď je atomicky prohodíme.
	s.mu.Lock()
	s.cache = newCache
	s.mu.Unlock()

	s.logger.Info("Sensor metadata reloaded", "loaded_sensors", count)
	return nil
}

// GetMetadata je metoda, kterou volá Ingestor pro každou příchozí zprávu.
// Musí být extrémně rychlá.
func (s *MetadataService) GetMetadata(topic string) (SensorMetadata, bool) {
	// RLock (Read Lock) umožňuje více goroutinám číst najednou.
	// Blokuje pouze v případě, že někdo právě drží Lock (zápis).
	s.mu.RLock()
	defer s.mu.RUnlock()

	meta, ok := s.cache[topic]
	return meta, ok
}

// StartAutoRefresh spouští smyčku na pozadí, která každou minutu obnoví cache.
// Umožňuje přidat nový senzor do DB bez restartu této služby.
func (s *MetadataService) StartAutoRefresh(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Pokud main context skončí (shutdown), ukončíme i tuto goroutinu.
			return
		case <-ticker.C:
			// Tik -> Obnovit data
			if err := s.LoadSensors(ctx); err != nil {
				s.logger.Error("Failed to auto-refresh metadata", "error", err)
			}
		}
	}
}
