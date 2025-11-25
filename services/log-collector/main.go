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

// KONFIGURACE
// V reálné aplikaci by byla v config.go, pro stručnost zde.
const (
	Broker   = "tcp://mqtt:1883"
	ClientID = "log-collector"
	LogTopic = "logs/#"           // Posloucháme všechno pod logs/
	LogDir   = "/var/log/iot-app" // Adresář uvnitř kontejneru, kam budeme psát
)

func main() {
	// 1. Inicializace vlastního loggeru (pouze na stdout, abychom viděli, že collector běží)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Startuji Log Collector", "dir", LogDir)

	// 2. Příprava adresáře pro logy
	// Pokud adresář neexistuje, vytvoříme ho (včetně podadresářů).
	// Permissin 0755: Vlastník může psát, ostatní číst/spouštět.
	if err := os.MkdirAll(LogDir, 0755); err != nil {
		logger.Error("Nelze vytvořit adresář pro logy", "error", err)
		os.Exit(1)
	}

	// 3. MQTT Handler (Callback)
	// Tato funkce se spustí pro KAŽDOU přijatou logovací zprávu z jakékoliv služby.
	messageHandler := func(client mqtt.Client, msg mqtt.Message) {
		// Topic vypadá např. takto: "logs/sensor-ingestor/info"
		topic := msg.Topic()
		payload := msg.Payload()

		// A. Parsování názvu služby z topicu
		// Rozdělíme string podle lomítka "/"
		parts := strings.Split(topic, "/")
		if len(parts) < 2 {
			logger.Warn("Ignoruji zprávu se špatným formátem topicu", "topic", topic)
			return
		}

		// parts[0] = "logs"
		// parts[1] = "sensor-ingestor" (Název služby)
		serviceName := parts[1]

		// B. Zápis do souboru
		// Funkce zapíše řádek do příslušného souboru.
		if err := appendLogToFile(serviceName, payload); err != nil {
			logger.Error("Chyba při zápisu do souboru", "service", serviceName, "error", err)
		}
	}

	// 4. Připojení k MQTT
	opts := mqtt.NewClientOptions().AddBroker(Broker).SetClientID(ClientID)
	opts.SetDefaultPublishHandler(messageHandler)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.Error("MQTT Connection failed", "error", token.Error())
		os.Exit(1)
	}
	defer client.Disconnect(250)

	// 5. Subscribe
	if token := client.Subscribe(LogTopic, 0, nil); token.Wait() && token.Error() != nil {
		logger.Error("Subscribe failed", "error", token.Error())
		os.Exit(1)
	}
	logger.Info("Poslouchám logy", "topic", LogTopic)

	// 6. Wait loop
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// appendLogToFile otevře (nebo vytvoří) soubor a připíše na konec nový řádek.
// Používáme pattern "Open-Write-Close" pro každý zápis.
// Pro extrémní high-performance by bylo lepší držet handlery otevřené,
// ale pro logování to stačí a je to bezpečnější z hlediska rotace logů (rsyslog).
func appendLogToFile(serviceName string, data []byte) error {
	// Sestavení cesty: /var/log/iot-app/sensor-ingestor.log
	filename := filepath.Join(LogDir, fmt.Sprintf("%s.log", serviceName))

	// Otevření souboru:
	// O_APPEND: Psát na konec.
	// O_CREATE: Vytvořit, pokud neexistuje.
	// O_WRONLY: Jen pro zápis.
	// 0644: Práva souboru (rw-r--r--).
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Zapíšeme data a přidáme nový řádek (\n), protože MQTT payload ho mít nemusí.
	if _, err := f.Write(data); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}

	return nil
}
