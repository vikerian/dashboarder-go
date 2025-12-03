package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Načtení Konfigurace
	cfg := LoadConfig()
	// MQTT Client musí být inicializován DŘÍVE než Logger, pokud chceme logovat start!
	// To je problém slepice-vejce.
	// ŘEŠENÍ: Nejprve uděláme klienta, pak logger.

	opts := mqtt.NewClientOptions().AddBroker(cfg.MQTTBroker).SetClientID(cfg.MQTTClientID)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		// Fallback: Pokud nejde MQTT, logujeme jen na stdout a končíme
		slog.Error("Fatal MQTT Error", "err", token.Error())
		os.Exit(1)
	}

	// --- SETUP LOGGERU ---
	// 1. Writer pro MQTT
	mqttWriter := NewMqttLogWriter(client, "sensor-ingestor")

	// 2. MultiWriter: Píše do obou (Stdout + MQTT)
	multi := io.MultiWriter(os.Stdout, mqttWriter)

	// 3. Vytvoření loggeru s tímto multi-writerem
	logger := slog.New(slog.NewJSONHandler(multi, nil))
	slog.SetDefault(logger)

	logger.Info("Ingestor startuje (Loguji do MQTT i Stdout)")
	// 5. Spuštění Healthcheck serveru (pro Docker/K8s)
	go startHealthServer(cfg.HTTPPort, logger)

	logger.Info("Spouštím službu Sensor Ingestor", "config", cfg)

	// 3. Inicializace DB Connection Pool
	// pgxpool spravuje sadu otevřených spojení do DB. Je thread-safe.
	dbPool, err := pgxpool.New(context.Background(), cfg.PostgresURL)
	if err != nil {
		// Pokud se nelze připojit k DB při startu, nemá smysl pokračovat -> Crash.
		// Docker kontejner se restartuje a zkusí to znovu.
		logger.Error("Kritická chyba: Nelze se připojit k DB", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close() // Zajistí uzavření spojení při ukončení programu

	// 4. Inicializace Metadata Service
	metaService := NewMetadataService(dbPool, logger)

	// První, blokující načtení dat. Musíme mít data, než začneme poslouchat MQTT.
	if err := metaService.LoadSensors(context.Background()); err != nil {
		logger.Error("Kritická chyba: Nepodařilo se načíst metadata senzorů", "error", err)
		os.Exit(1)
	}

	// Spuštění automatického obnovování cache na pozadí (goroutina)
	// Vytváříme context, který zrušíme při shutdownu aplikace.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go metaService.StartAutoRefresh(ctx)

	// 6. Nastavení MQTT Klienta
	//opts := mqtt.NewClientOptions()
	//opts.AddBroker(cfg.MQTTBroker)
	//opts.SetClientID(cfg.MQTTClientID)

	// --- HLAVNÍ LOOP ZPRACOVÁNÍ ZPRÁV ---
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		// A. Zavoláme naši logiku (service.go)
		normalizedBytes, err := ProcessMessage(msg.Topic(), msg.Payload(), metaService)

		if err != nil {
			// Pokud nastala chyba (validace, neznámý topic), logujeme warning.
			// NEUKONČUJEME službu, jen zahodíme tuto jednu zprávu.
			logger.Warn("Zpráva odmítnuta", "topic", msg.Topic(), "důvod", err)
			return
		}

		// B. Odeslání validního JSONu dál (do Persisteru)
		token := client.Publish(cfg.OutputTopic, 0, false, normalizedBytes)
		token.Wait()

		if token.Error() != nil {
			logger.Error("Chyba při publikaci do MQTT", "error", token.Error())
		} else {
			// V Debug levelu můžeme vidět každou zprávu, v Info ne (aby logy nebyly obří)
			logger.Debug("Zpráva úspěšně zpracována a odeslána")
		}
	})

	// Odpojení s timeoutem 250ms při ukončení
	defer client.Disconnect(250)

	logger.Info("Připojeno k MQTT", "broker", cfg.MQTTBroker)

	// 7. Subscribe (Odběr zpráv)
	if token := client.Subscribe(cfg.InputTopic, 0, nil); token.Wait() && token.Error() != nil {
		logger.Error("Subscribe selhal", "topic", cfg.InputTopic, "error", token.Error())
		os.Exit(1)
	}
	logger.Info("Poslouchám na topicu", "topic", cfg.InputTopic)

	// 8. Graceful Shutdown (Čekání na signál ukončení)
	// Blokujeme hlavní vlákno, dokud nepřijde SIGINT (Ctrl+C) nebo SIGTERM (Docker stop).
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Ukončuji službu...")
	// Zde proběhnou defery (cancel contextu, disconnect mqtt, close db pool)
}

// startHealthServer spustí jednoduchý HTTP endpoint.
func startHealthServer(port string, logger *slog.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	logger.Info("Health server běží", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Error("Health server spadl", "error", err)
	}
}
