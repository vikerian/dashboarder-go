package main

import (
	"context"
	"fmt"
	"log/slog" // Nutný import pro logování
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Service zapouzdřuje logiku získávání dat.
type Service struct {
	db    *pgxpool.Pool
	redis *redis.Client
	// Přidáme logger do service, abychom mohli debugovat dotazy
	logger *slog.Logger
}

// Upravený konstruktor - přijímá i logger
func NewService(db *pgxpool.Pool, rdb *redis.Client, logger *slog.Logger) *Service {
	return &Service{
		db:     db,
		redis:  rdb,
		logger: logger,
	}
}

// GetAllSensors vrací seznam senzorů + aktuální hodnoty z Redisu.
func (s *Service) GetAllSensors(ctx context.Context) ([]SensorDTO, error) {
	s.logger.Info("DEBUG: Začínám GetAllSensors")

	// 1. SQL DOTAZ (Metadata)
	query := `
		SELECT s.id, s.mqtt_topic, s.friendly_name, st.name, st.unit
		FROM sensors s
		JOIN sensor_types st ON s.sensor_type_id = st.id
		WHERE s.is_active = true
		ORDER BY s.id ASC
	`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		s.logger.Error("CHYBA: SQL dotaz na senzory selhal", "error", err)
		return nil, fmt.Errorf("db query failed: %w", err)
	}
	defer rows.Close()

	var sensors []SensorDTO
	for rows.Next() {
		var dto SensorDTO
		if err := rows.Scan(&dto.ID, &dto.Topic, &dto.Name, &dto.Type, &dto.Unit); err != nil {
			s.logger.Error("CHYBA: Scan řádku selhal", "error", err)
			return nil, err
		}

		// 2. REDIS LOOKUP (Live Data)
		key := fmt.Sprintf("sensor:last:%d", dto.ID)

		// Získáme hodnotu z Redisu
		valStr, err := s.redis.Get(ctx, key).Result()

		if err == redis.Nil {
			// Klíč neexistuje = Senzor ještě neposlal data, nebo Persister nezapisuje do Redisu.
			s.logger.Warn("DEBUG: Redis klíč nenalezen", "key", key)
		} else if err != nil {
			// Jiná chyba Redisu (spojení atd.)
			s.logger.Error("CHYBA: Redis GET selhal", "key", key, "error", err)
		} else {
			// Úspěch
			val, parseErr := strconv.ParseFloat(valStr, 64)
			if parseErr == nil {
				dto.CurrentValue = &val
				// Logujeme jen občas nebo pro debug
				// s.logger.Debug("DEBUG: Načtena hodnota z Redisu", "key", key, "val", val)
			} else {
				s.logger.Error("CHYBA: Nelze parsovat hodnotu z Redisu", "val", valStr)
			}
		}

		sensors = append(sensors, dto)
	}

	s.logger.Info("DEBUG: GetAllSensors dokončeno", "count", len(sensors))
	return sensors, nil
}

// GetHistory vrací data pro graf. Zde často vzniká chyba s časem.
func (s *Service) GetHistory(ctx context.Context, sensorID int64, durationStr string) ([]HistoryPoint, error) {
	s.logger.Info("DEBUG: Začínám GetHistory", "sensor_id", sensorID, "range", durationStr)

	// 1. Validace času
	dur, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid duration format: %w", err)
	}

	// Výpočet startovního času.
	// DŮLEŽITÉ: Používáme UTC, protože v DB jsou data v UTC.
	endTime := time.Now().UTC()
	startTime := endTime.Add(-dur)

	s.logger.Info("DEBUG: SQL Parametry",
		"start_time_utc", startTime.Format(time.RFC3339),
		"end_time_utc", endTime.Format(time.RFC3339),
		"sensor_id", sensorID,
	)

	// 2. SQL Select
	query := `
		SELECT time, value
		FROM sensor_data
		WHERE sensor_id = $1 AND time >= $2
		ORDER BY time ASC
	`

	rows, err := s.db.Query(ctx, query, sensorID, startTime)
	if err != nil {
		s.logger.Error("CHYBA: SQL History selhal", "error", err)
		return nil, fmt.Errorf("history query failed: %w", err)
	}
	defer rows.Close()

	points := make([]HistoryPoint, 0, 100)

	for rows.Next() {
		var p HistoryPoint
		if err := rows.Scan(&p.Time, &p.Value); err != nil {
			s.logger.Error("CHYBA: Scan historie selhal", "error", err)
			return nil, err
		}
		points = append(points, p)
	}

	s.logger.Info("DEBUG: GetHistory dokončeno", "points_count", len(points))

	if len(points) == 0 {
		// Varování: Pokud DB vrací 0 bodů, buď tam nejsou data, nebo je špatně časové okno.
		s.logger.Warn("DEBUG: DB vrátila prázdný výsledek! Zkontroluj čas senzorů vs serveru.")
	}

	return points, nil
}
