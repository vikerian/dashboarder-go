package main

import (
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MqttLogWriter implementuje rozhraní io.Writer.
// Vše, co se do něj zapíše, se odešle do MQTT.
type MqttLogWriter struct {
	client      mqtt.Client
	topicPrefix string
}

// NewMqttLogWriter vytvoří novou instanci writeru.
// topicPrefix bude např. "logs/sensor-ingestor"
func NewMqttLogWriter(client mqtt.Client, serviceName string) *MqttLogWriter {
	return &MqttLogWriter{
		client:      client,
		topicPrefix: fmt.Sprintf("logs/%s", serviceName),
	}
}

// Write je metoda vyžadovaná rozhraním io.Writer.
// slog ji zavolá pokaždé, když chce něco zalogovat.
func (w *MqttLogWriter) Write(p []byte) (n int, err error) {
	// POKROČILÉ: Logování by nemělo blokovat aplikaci.
	// Správně by se toto mělo posílat do kanálu (buffered channel) a odesílat goroutinou.
	// Pro výuku to pošleme přímo, ale bez čekání na potvrzení (Wait).

	// Payload musíme zkopírovat, protože 'p' se může změnit.
	payload := make([]byte, len(p))
	copy(payload, p)

	// Odeslání do MQTT
	// Topic: logs/sensor-ingestor
	// Token.Wait() NEVOLÁME, aby logování nezpomalovalo aplikaci (fire-and-forget).
	w.client.Publish(w.topicPrefix, 0, false, payload)

	return len(p), nil
}
