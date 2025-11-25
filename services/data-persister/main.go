package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// 1. Setup Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := LoadConfig()
	logger.Info("Startuji Data Persister", "config", cfg)

	// 2. Inicializace Repozitáře (DB + Redis)
	ctx := context.Background()
	repo, err := NewRepository(ctx, cfg)
	if err != nil {
		logger.Error("Kritická chyba připojení k databázím", "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	logger.Info("Databáze připojeny")

	// 3. MQTT Klient Setup
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)

	// --- HLAVNÍ LOGIKA ---
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		// A. Deserializace JSONu
		var event SensorEvent
		if err := json.Unmarshal(msg.Payload(), &event); err != nil {
			logger.Error("Neplatný JSON formát", "payload", string(msg.Payload()), "error", err)
			return
		}

		// B. Uložení (vytvoříme context s timeoutem, aby DB operace nevisela věčně)
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := repo.SaveMeasurement(saveCtx, event); err != nil {
			logger.Error("Chyba při ukládání dat", "sensor_id", event.SensorID, "error", err)
		} else {
			// Úspěch (Logujeme jen debug, v produkci by to bylo moc spamu)
			logger.Debug("Data uložena", "sensor_id", event.SensorID, "val", event.Value)
		}
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.Error("MQTT connection failed", "error", token.Error())
		os.Exit(1)
	}
	defer client.Disconnect(250)

	// 4. Subscribe (posloucháme na výstupu z Ingestoru)
	if token := client.Subscribe(cfg.InputTopic, 0, nil); token.Wait() && token.Error() != nil {
		logger.Error("Subscribe failed", "error", token.Error())
		os.Exit(1)
	}
	logger.Info("Poslouchám na topicu", "topic", cfg.InputTopic)

	// 5. Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Vypínám službu...")
}
