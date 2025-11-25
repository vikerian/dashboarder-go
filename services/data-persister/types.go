package main

import "time"

// SensorEvent je struktura příchozí zprávy z MQTT (z topicu events/...).
// Musí odpovídat JSONu, který generuje služba 'sensor-ingestor'.
type SensorEvent struct {
	SensorID  int64     `json:"sensor_id"` // ID senzoru (Foreign Key do DB)
	Value     float64   `json:"value"`     // Naměřená hodnota
	Timestamp time.Time `json:"timestamp"` // Čas měření (UTC)
}
