package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Service drží spojení na databáze a obsahuje metody pro získání dat.
// Implementuje vzor "Repository" nebo "Data Access Object".
type Service struct {
	db    *pgxpool.Pool // Pool pro SQL dotazy (TimescaleDB)
	redis *redis.Client // Klient pro Key-Value store (Valkey)
}

// NewService je konstruktor (Dependency Injection).
func NewService(db *pgxpool.Pool, rdb *redis.Client) *Service {
	return &Service{db: db, redis: rdb}
}

// GetAllSensors vrací seznam všech senzorů obohacený o aktuální hodnoty.
// Kombinuje data z relační DB (metadata) a NoSQL (live value).
func (s *Service) GetAllSensors(ctx context.Context) ([]SensorDTO, error) {
	// 1. SQL DOTAZ: Získáme statická metadata.
	// Používáme JOIN, abychom k senzoru (sensors) dotáhli jeho typ a jednotku (sensor_types).
	query := `
		SELECT s.id, s.mqtt_topic, s.friendly_name, st.name, st.unit
		FROM sensors s
		JOIN sensor_types st ON s.sensor_type_id = st.id
		WHERE s.is_active = true
		ORDER BY s.id ASC
	`

	// Query vrací iterátor (rows). Nezapomenout zavřít!
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("selhal SQL dotaz na senzory: %w", err)
	}
	defer rows.Close() // Uvolnění connection zpět do poolu

	var sensors []SensorDTO

	// Iterujeme přes výsledky řádek po řádku
	for rows.Next() {
		var dto SensorDTO
		// Scan mapuje sloupce z SELECT do struktury Go
		if err := rows.Scan(&dto.ID, &dto.Topic, &dto.Name, &dto.Type, &dto.Unit); err != nil {
			return nil, err
		}

		// 2. REDIS LOOKUP: Pro každý senzor zkusíme najít jeho aktuální hodnotu.
		// Klíč musí odpovídat tomu, co ukládá Persister (např. "sensor:last:5").
		key := fmt.Sprintf("sensor:last:%d", dto.ID)

		// Voláme Redis. Pokud klíč neexistuje (Redis Nil), vrací error, který ignorujeme (hodnota zůstane nil).
		valStr, err := s.redis.Get(ctx, key).Result()
		if err == nil {
			// Převedeme string z Redisu na float
			val, _ := strconv.ParseFloat(valStr, 64)
			// Uložíme do pointeru (dto.CurrentValue už nebude nil)
			dto.CurrentValue = &val
		}

		sensors = append(sensors, dto)
	}
	return sensors, nil
}

// GetHistory vrací historická data pro grafy.
// durationStr: Formát Go duration, např "1h", "24h", "7d".
func (s *Service) GetHistory(ctx context.Context, sensorID int64, durationStr string) ([]HistoryPoint, error) {
	// 1. Validace vstupu: Převedeme string na time.Duration
	dur, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("neplatný formát času (např. 1h, 30m): %w", err)
	}

	// Vypočítáme čas "od kdy" chceme data (nyní minus duration)
	startTime := time.Now().UTC().Add(-dur)

	// 2. SQL DOTAZ: TimescaleDB je optimalizovaná na časové řady.
	// Index na (sensor_id, time) zajistí, že tento dotaz bude bleskový i při milionech řádků.
	query := `
		SELECT time, value
		FROM sensor_data
		WHERE sensor_id = $1 AND time >= $2
		ORDER BY time ASC
	`

	rows, err := s.db.Query(ctx, query, sensorID, startTime)
	if err != nil {
		return nil, fmt.Errorf("chyba načítání historie: %w", err)
	}
	defer rows.Close()

	// OPTIMALIZACE PAMĚTI:
	// make([]T, 0, capacity) předalokuje pole.
	// Odhadujeme např. 100 bodů. Pokud jich bude víc, Go pole zvětší, ale ušetříme alokace na začátku.
	points := make([]HistoryPoint, 0, 100)

	for rows.Next() {
		var p HistoryPoint
		if err := rows.Scan(&p.Time, &p.Value); err != nil {
			return nil, err
		}
		points = append(points, p)
	}

	return points, nil
}
