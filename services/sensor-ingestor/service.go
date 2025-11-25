package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// ProcessMessage zapouzdřuje logiku zpracování jedné zprávy.
// Vstupy: topic, raw payload a služba pro metadata.
// Výstup: JSON bytes nebo chyba.
func ProcessMessage(topic string, payload []byte, metaService *MetadataService) ([]byte, error) {

	// KROK 1: Identifikace (Lookup)
	// Podíváme se do paměti (cache), jestli tento topic známe.
	meta, found := metaService.GetMetadata(topic)
	if !found {
		// Pokud topic není v DB, považujeme zprávu za "odpad" nebo neznámou.
		// Vracíme error, aby volající věděl, že se nemá nic posílat dál.
		// Tím chráníme DB před insertem dat bez vazby (Integrity Constraint Violation).
		return nil, fmt.Errorf("neznámý MQTT topic (není v DB): %s", topic)
	}

	// KROK 2: Parsing
	// Předpokládáme, že payload je prosté číslo (např. "24.5").
	valStr := string(payload)
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return nil, fmt.Errorf("hodnota '%s' není platné číslo: %w", valStr, err)
	}

	// KROK 3: Business Validace (Limity)
	// Kontrolujeme min/max pouze pokud jsou v DB definovány (nejsou nil).

	// Kontrola MIN
	if meta.MinValue != nil && val < *meta.MinValue {
		// Příklad: Teplota -500°C je fyzikální nesmysl (chyba senzoru).
		return nil, fmt.Errorf("hodnota %.2f je pod minimálním limitem %.2f pro senzor ID %d", val, *meta.MinValue, meta.ID)
	}

	// Kontrola MAX
	if meta.MaxValue != nil && val > *meta.MaxValue {
		return nil, fmt.Errorf("hodnota %.2f je nad maximálním limitem %.2f pro senzor ID %d", val, *meta.MaxValue, meta.ID)
	}

	// KROK 4: Transformace na DTO (Data Transfer Object)
	// Vytváříme objekt, který obsahuje ID senzoru (ne string, ale int64).
	event := SensorEvent{
		SensorID:  meta.ID,
		Value:     val,
		Timestamp: time.Now().UTC(),
	}

	// Serializace do JSON pro odeslání do fronty
	return json.Marshal(event)
}
