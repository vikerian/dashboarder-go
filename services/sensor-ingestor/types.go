package main

import "time"

// SensorEvent je finální struktura, kterou posíláme dál do systému (do fronty events/...).
// Oproti původní verzi zde už není string 'SensorID', ale int64.
// DŮVOD: Odpovídá primárnímu klíči v DB tabulce 'sensors'. Šetří místo a zrychluje indexaci.
type SensorEvent struct {
	// SensorID: Primární klíč senzoru z databáze (FK).
	SensorID int64 `json:"sensor_id"`

	// Value: Naměřená hodnota (teplota, tlak...).
	Value float64 `json:"value"`

	// Timestamp: Čas měření. Vždy v UTC pro konzistenci napříč časovými pásmy.
	Timestamp time.Time `json:"timestamp"`
}
