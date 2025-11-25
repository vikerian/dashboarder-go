package main

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
)

// WebHandler funguje jako Controller v MVC architektuře.
// Drží reference na služby a připravené HTML šablony.
type WebHandler struct {
	client *APIClient   // Klient pro komunikaci s backend API
	logger *slog.Logger // Strukturovaný logger

	// ZMĚNA ARCHITEKTURY: Místo jedné proměnné 'tmpl' máme dvě oddělené.
	// Důvod: Obě stránky (index.html i detail.html) definují blok {{define "content"}}.
	// Kdybychom je načetli do jedné sady (ParseGlob), jedna by přepsala druhou.
	// Proto musíme mít pro každou stránku vlastní izolovanou instanci šablony.
	indexTmpl  *template.Template
	detailTmpl *template.Template
}

// NewWebHandler je konstruktor. Zde probíhá inicializace a parsování šablon (jen jednou při startu).
func NewWebHandler(client *APIClient, logger *slog.Logger) (*WebHandler, error) {

	// 1. DEFINICE POMOCNÝCH FUNKCÍ (FuncMap)
	// Go šablony jsou záměrně jednoduché ("logic-less"). Složitější operace
	// musíme definovat jako Go funkce a zpřístupnit je v HTML.
	funcMap := template.FuncMap{

		// Funkce "deref": Bezpečný výpis pointeru.
		// V Go šabloně {{.Value}} na pointer vypíše adresu paměti (0x...).
		// My chceme hodnotu. Pokud je pointer nil, vrátíme 0.0, aby UI nespadlo.
		"deref": func(f *float64) float64 {
			if f == nil {
				return 0.0
			}
			return *f
		},

		// Funkce "to_json": Klíčová pro předání dat do JavaScriptu.
		// Go šablony automaticky "escapují" HTML znaky (prevence XSS).
		// To by ale rozbilo JSON strukturu (změnilo uvozovky na &#34;).
		// Typ template.JS říká šabloně: "Tomuto věř, toto je bezpečný JavaScript kód".
		"to_json": func(v interface{}) template.JS {
			a, err := json.Marshal(v)
			if err != nil {
				// V případě chyby vrátíme prázdné pole, aby JS graf nespadl.
				return template.JS("[]")
			}
			return template.JS(a)
		},
	}

	// 2. NAČTENÍ ŠABLONY PRO INDEX (Dashboard)
	// Vytvoříme novou šablonu, pojmenujeme ji podle layoutu a naučíme ji funkce.
	indexTmpl := template.New("layout.html").Funcs(funcMap)
	// ParseFiles načte POUZE vyjmenované soubory.
	// Kombinujeme layout (hlavička/patička) a index (obsah).
	indexTmpl, err := indexTmpl.ParseFiles(
		filepath.Join("templates", "layout.html"),
		filepath.Join("templates", "index.html"),
	)
	if err != nil {
		return nil, err
	}

	// 3. NAČTENÍ ŠABLONY PRO DETAIL (Graf)
	// Opět vytvoříme zcela novou instanci. Tím zajistíme izolaci.
	// Zde definice "content" z detail.html nepřepíše tu z index.html.
	detailTmpl := template.New("layout.html").Funcs(funcMap)
	detailTmpl, err = detailTmpl.ParseFiles(
		filepath.Join("templates", "layout.html"),
		filepath.Join("templates", "detail.html"),
	)
	if err != nil {
		return nil, err
	}

	return &WebHandler{
		client:     client,
		logger:     logger,
		indexTmpl:  indexTmpl,
		detailTmpl: detailTmpl,
	}, nil
}

// HandleIndex obsluhuje hlavní stránku (GET /).
func (h *WebHandler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	// 1. Získání dat (Model)
	sensors, err := h.client.GetSensors()
	if err != nil {
		h.logger.Error("Chyba při volání API pro senzory", "error", err)
		// Vrátíme 502 Bad Gateway, protože chyba není u nás, ale na backendu.
		http.Error(w, "Backend API je nedostupné", http.StatusBadGateway)
		return
	}

	// 2. Příprava dat pro View (ViewModel)
	data := map[string]interface{}{
		"Title":   "IoT Dashboard",
		"Sensors": sensors,
		"Page":    "index", // Pro aktivní položku v menu
	}

	// 3. Renderování
	// DŮLEŽITÉ: Používáme h.indexTmpl.
	// Voláme "layout.html", což je název definovaný uvnitř souboru layout (define "layout.html").
	// Layout pak zavolá {{template "content"}} který je definován v index.html.
	err = h.indexTmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		h.logger.Error("Chyba renderování index šablony", "error", err)
	}
}

// HandleDetail obsluhuje stránku s grafem (GET /sensor/{id}).
func (h *WebHandler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	// 1. Extrakce ID z URL (Go 1.22 feature PathValue)
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	// Získání parametru range (výchozí 24h)
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}

	// 2. Získání dat z API
	points, err := h.client.GetHistory(id, rng)
	if err != nil {
		h.logger.Error("Chyba při volání API pro historii", "id", id, "error", err)
		http.Error(w, "Chyba načítání dat", http.StatusInternalServerError)
		return
	}

	// Potřebujeme i jméno senzoru. V produkci by API mělo mít endpoint /api/sensor/{id}.
	// Pro výuku si zjednodušeně stáhneme seznam všech a najdeme ten náš.
	allSensors, _ := h.client.GetSensors()
	var currentSensor SensorDTO
	for _, s := range allSensors {
		if s.ID == id {
			currentSensor = s
			break
		}
	}

	data := map[string]interface{}{
		"Title":  "Detail Senzoru",
		"Sensor": currentSensor,
		"Points": points,
		"Page":   "detail",
		"Range":  rng,
	}

	// 3. Renderování
	// DŮLEŽITÉ: Používáme h.detailTmpl. Zde "content" pochází z detail.html.
	err = h.detailTmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		h.logger.Error("Chyba renderování detail šablony", "error", err)
	}
}
