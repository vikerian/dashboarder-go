package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// --- DATOVÉ MODELY (DTO) ---
// Tyto struktury definují formát JSON dat, která nám vrací Home API.
// Musí přesně odpovídat tomu, co API posílá.

// SensorDTO reprezentuje jeden senzor v seznamu.
type SensorDTO struct {
	ID    int64  `json:"id"`
	Topic string `json:"topic"`
	Name  string `json:"name"` // Friendly name (např. "Teplota Obývák")
	Type  string `json:"type"` // Typ (např. "temperature")
	Unit  string `json:"unit"` // Jednotka (např. "°C")

	// CurrentValue je pointer (*float64), protože hodnota může být null (nil).
	// Pokud senzor ještě neposlal data, nechceme zobrazit 0, ale "nic".
	CurrentValue *float64 `json:"current_value"`
}

// HistoryPoint reprezentuje jeden bod v grafu (čas a hodnota).
type HistoryPoint struct {
	Time  time.Time `json:"t"`
	Value float64   `json:"v"`
}

// APIClient zapouzdřuje logiku HTTP volání na backend.
// Zbytek aplikace (Handlery) díky tomu neřeší URL adresy, JSON decoding ani status kódy.
type APIClient struct {
	BaseURL    string       // Adresa API (např. http://home-api:8080)
	httpClient *http.Client // Instance http klienta (umožňuje nastavit timeouty)
}

// NewAPIClient vytváří instanci klienta.
// Důležité: Vždy nastavujeme Timeout! Defaultní http.Client v Go nemá timeout,
// takže pokud by API neodpovídalo, Dashboard by "visel" navěky a došla by paměť.
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second, // Pokud API neodpoví do 5s, request selže.
		},
	}
}

// GetSensors zavolá endpoint GET /api/sensors a vrátí seznam objektů.
func (c *APIClient) GetSensors() ([]SensorDTO, error) {
	// Sestavení URL
	url := c.BaseURL + "/api/sensors"

	// Provedení GET požadavku
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("chyba sítě při volání API: %w", err)
	}
	// Důležité: Body musíme vždy zavřít, jinak tečou file descriptory (memory leak).
	defer resp.Body.Close()

	// Kontrola HTTP Status kódu (očekáváme 200 OK)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API vrátilo chybný status: %d", resp.StatusCode)
	}

	// Deserializace JSON odpovědi do Go struktury.
	// Používáme json.Decoder, který čte stream přímo z Body (efektivnější než ReadAll).
	var sensors []SensorDTO
	if err := json.NewDecoder(resp.Body).Decode(&sensors); err != nil {
		return nil, fmt.Errorf("chyba při parsování JSONu: %w", err)
	}

	return sensors, nil
}

// GetHistory zavolá endpoint GET /api/sensors/{id}/history
func (c *APIClient) GetHistory(sensorID int64, rangeStr string) ([]HistoryPoint, error) {
	// Formátování URL s parametry
	url := fmt.Sprintf("%s/api/sensors/%d/history?range=%s", c.BaseURL, sensorID, rangeStr)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	var points []HistoryPoint
	if err := json.NewDecoder(resp.Body).Decode(&points); err != nil {
		return nil, err
	}

	return points, nil
}
