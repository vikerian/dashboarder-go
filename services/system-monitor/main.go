package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// 1. Inicializace Loggeru
	// Používáme JSON formát pro snadné strojové čtení logů.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 2. Načtení Konfigurace
	cfg := LoadConfig()

	logger.Info("Startuji System Monitor", "interval", cfg.Interval)

	// 3. Konfigurace MQTT Klienta
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)

	// Vytvoření instance klienta
	client := mqtt.NewClient(opts)

	// Připojení k brokeru (blokující operace s Tokenem)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.Error("Selhalo připojení k MQTT", "error", token.Error())
		os.Exit(1) // Bez MQTT nemá smysl běžet
	}
	// Zajistíme odpojení při ukončení programu
	defer client.Disconnect(250)

	// 4. Nastavení časovače (Ticker)
	// Ticker bude posílat signál do kanálu ticker.C každých X sekund (podle configu).
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// 5. Handling systémových signálů (Graceful Shutdown)
	// Chceme, aby se aplikace ukončila slušně při CTRL+C nebo docker stop.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Pomocná funkce (closure) pro odesílání dat.
	// Zapouzdřuje logiku formátování a volání MQTT knihovny.
	publish := func(topic string, value float64) {
		// Převedeme float na string (např. 12.50)
		payload := fmt.Sprintf("%.2f", value)

		// Odeslání zprávy (QoS 0, Retained = false)
		token := client.Publish(topic, 0, false, payload)
		token.Wait() // Čekáme na potvrzení odeslání (lokální, ne od brokera u QoS 0)

		// Logujeme odeslání (v Debug levelu, aby to nespamovalo, pokud si nepřejeme)
		logger.Info("Metrika odeslána", "topic", topic, "val", payload)
	}

	// OKAMŽITÉ ODESLÁNÍ PŘI STARTU
	// Nechceme čekat např. 60 sekund na první tik časovače.
	// Spustíme to v anonymní goroutině, aby to neblokovalo start smyčky.
	go func() {
		logger.Info("Provádím prvotní měření...")
		stats, err := CollectStats(logger)
		if err == nil {
			publish("/msh/system/cpu", stats.CPULoad)
			publish("/msh/system/ram_used", stats.RamUsedMB)
			publish("/msh/system/ram_total", stats.RamTotalMB) // <-- TOTO CHYBĚLO
			publish("/msh/system/app_ram", stats.AppRamUsedMB)
			publish("/msh/system/disk_used", stats.DiskUsedGB)
			publish("/msh/system/disk_total", stats.DiskTotalGB) // <-- TOTO CHYBĚLO
		}
	}()

	// 6. Hlavní nekonečná smyčka
	logger.Info("Vstupuji do hlavní smyčky")
	for {
		select {
		// A) Přišel signál k ukončení (CTRL+C)
		case <-sigChan:
			logger.Info("Přijat signál ukončení, vypínám...")
			return // Vyskočí z main(), spustí se defery

		// B) Tiknul časovač (např. každou minutu)
		case <-ticker.C:
			// Sběr dat z HW (monitor.go)
			// Tato operace může chvíli trvat (měření CPU trvá min 1s).
			stats, err := CollectStats(logger)
			if err != nil {
				logger.Error("Chyba při měření", "error", err)
				continue // Zkusíme to zase příště
			}

			// Odeslání všech metrik do MQTT
			// Ingestor si je přebere podle topiců.
			publish("/msh/system/cpu", stats.CPULoad)

			publish("/msh/system/ram_used", stats.RamUsedMB)
			publish("/msh/system/ram_total", stats.RamTotalMB) // <-- ZDE JSME DOPLNILI TOTAL

			publish("/msh/system/app_ram", stats.AppRamUsedMB)

			publish("/msh/system/disk_used", stats.DiskUsedGB)
			publish("/msh/system/disk_total", stats.DiskTotalGB) // <-- ZDE JSME DOPLNILI TOTAL
		}
	}
}
