package main

import "time"

// SensorDTO (Data Transfer Object) slouží pro odeslání seznamu senzorů na frontend.
// Oddělujeme databázový model od API modelu (decoupling).
type SensorDTO struct {
	// ID: Unikátní identifikátor senzoru (koresponduje s DB Primary Key).
	ID int64 `json:"id"`

	// Topic: MQTT topic, odkud data přišla (užitečné pro debug v UI).
	Topic string `json:"topic"`

	// Name: Čitelný název pro uživatele (např. "Obývák Teplota").
	Name string `json:"name"`

	// Type: Typ měření (např. "temperature"), určuje ikonku v UI.
	Type string `json:"type"`

	// Unit: Jednotka (např. "°C"), zobrazí se vedle hodnoty.
	Unit string `json:"unit"`

	// CurrentValue: Poslední známá hodnota (Live Data).
	// DŮLEŽITÉ: Používáme *float64 (pointer).
	// Důvod: Hodnota může být NULL (pokud senzor ještě nic neposlal nebo data expirovala).
	// Kdybychom použili float64, výchozí hodnota by byla 0.0, což je matoucí (je to 0 stupňů nebo chyba?).
	CurrentValue *float64 `json:"current_value"`
}

// HistoryPoint reprezentuje jeden bod v grafu.
// Používáme velmi krátké názvy klíčů ("t", "v"), abychom šetřili přenosové pásmo (bandwidth).
// Při tisících bodech v grafu každý ušetřený znak v JSONu hraje roli.
type HistoryPoint struct {
	Time  time.Time `json:"t"` // Časová značka osy X
	Value float64   `json:"v"` // Hodnota na ose Y
}
