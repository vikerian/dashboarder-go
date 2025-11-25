package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// 1. Inicializace Loggeru (pro vlastní diagnostiku collectoru)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 2. Načtení Konfigurace (z ENV)
	cfg := LoadConfig()
	logger.Info("Startuji Log Collector", "config", cfg)

	// 3. Příprava adresáře pro logy
	// Používáme cestu z konfigurace (cfg.LogDir).
	// MkdirAll vytvoří celou cestu, pokud neexistuje (např. /var/log/iot-app).
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		logger.Error("Kritická chyba: Nelze vytvořit adresář pro logy", "dir", cfg.LogDir, "error", err)
		os.Exit(1)
	}

	// 4. MQTT Handler (Logika zpracování zprávy)
	messageHandler := func(client mqtt.Client, msg mqtt.Message) {
		topic := msg.Topic()     // např. "logs/sensor-ingestor/info"
		payload := msg.Payload() // JSON log zpráva

		// Rozparsujeme topic, abychom zjistili název služby.
		// Očekávaný formát: logs/{service_name}/{level} nebo jen logs/{service_name}
		parts := strings.Split(topic, "/")

		// Validace: Musíme mít alespoň 2 části (root a service)
		if len(parts) < 2 {
			logger.Warn("Ignoruji topic s neplatným formátem", "topic", topic)
			return
		}

		// Název služby je druhá část topicu (index 1)
		serviceName := parts[1]

		// Zápis do souboru.
		// Předáváme cfg.LogDir, aby funkce věděla, kam psát.
		if err := appendLogToFile(cfg.LogDir, serviceName, payload); err != nil {
			logger.Error("Chyba zápisu do souboru", "service", serviceName, "error", err)
		}
	}

	// 5. Připojení k MQTT
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)     // Z konfigu
	opts.SetClientID(cfg.MQTTClientID) // Z konfigu
	opts.SetDefaultPublishHandler(messageHandler)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.Error("Nelze se připojit k MQTT", "error", token.Error())
		os.Exit(1)
	}
	defer client.Disconnect(250)

	logger.Info("Připojeno k MQTT brokeru")

	// 6. Subscribe
	// Posloucháme na topicu definovaném v konfigu (default "logs/#")
	if token := client.Subscribe(cfg.LogTopic, 0, nil); token.Wait() && token.Error() != nil {
		logger.Error("Chyba při subscribe", "topic", cfg.LogTopic, "error", token.Error())
		os.Exit(1)
	}
	logger.Info("Log Collector naslouchá", "topic", cfg.LogTopic)

	// 7. Wait Loop (Graceful Shutdown)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Ukončuji Log Collector...")
}

// appendLogToFile připojí řádek na konec souboru.
// dir: Cesta k adresáři s logy (z configu)
// serviceName: Název služby (použije se jako název souboru)
// data: Obsah logu
func appendLogToFile(dir string, serviceName string, data []byte) error {
	// Sestavíme plnou cestu: /var/log/iot-app/nazev-sluzby.log
	// filepath.Join řeší správné lomítka pro daný OS.
	filename := filepath.Join(dir, fmt.Sprintf("%s.log", serviceName))

	// Otevřeme soubor v režimu Append (připojit na konec).
	// Pokud neexistuje, vytvoříme ho (0644 = rw-r--r--).
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	// Důležité: Zavřít soubor po dokončení zápisu.
	defer f.Close()

	// Zapíšeme data
	if _, err := f.Write(data); err != nil {
		return err
	}
	// Přidáme nový řádek, aby logy nebyly "slepence"
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}

	return nil
}
