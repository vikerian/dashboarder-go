package main

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
)

// WebHandler slouží jako "Controller". Připravuje data a renderuje HTML.
type WebHandler struct {
	client *APIClient
	logger *slog.Logger
	tmpl   *template.Template
}

// NewWebHandler inicializuje šablony a registruje pomocné funkce.
func NewWebHandler(client *APIClient, logger *slog.Logger) (*WebHandler, error) {

	// 1. DEFINICE VLASTNÍCH FUNKCÍ PRO ŠABLONY (FuncMap)
	// Go šablony jsou bezpečné, ale omezené. Neumí samy od sebe např. dereferencovat pointery
	// nebo formátovat data složitým způsobem. Musíme jim to naučit.
	funcMap := template.FuncMap{
		// Název funkce v HTML bude "deref".
		// Vstup: pointer na float64 (*float64).
		// Výstup: hodnota float64.
		"deref": func(f *float64) float64 {
			// Bezpečnostní kontrola: Pokud je pointer nil (data z API nepřišla),
			// nesmíme se pokusit o dereferenci (*f), jinak aplikace spadne (panic).
			// Místo toho vrátíme 0.0.
			if f == nil {
				return 0.0
			}
			// Pokud data existují, vrátíme hodnotu, na kterou pointer ukazuje.
			return *f
		},
		"to_json": func(v interface{}) template.JS {
			a, err := json.Marshal(v)
			if err != nil {
				// V případě chyby vrátíme prázdné pole, aby JS nespadl
				return template.JS("[]")
			}
			return template.JS(a)
		},
	}

	// 2. NAČTENÍ A PARSOVÁNÍ ŠABLON
	// Tady se děje magie v přesném pořadí:
	// A) template.New("base"): Vytvoříme prázdný kontejner pro šablony.
	// B) .Funcs(funcMap): Řekneme kontejneru: "Nauč se tyto funkce (deref)".
	//    TOTO SE MUSÍ STÁT PŘED PARSOVÁNÍM SOUBORŮ!
	// C) .ParseGlob(...): Teprve teď načteme soubory z disku. Šablony v nich už mohou používat "deref".
	tmpl, err := template.New("base").Funcs(funcMap).ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		return nil, err
	}

	return &WebHandler{
		client: client,
		logger: logger,
		tmpl:   tmpl,
	}, nil
}

// HandleIndex: Dashboard (Přehled)
func (h *WebHandler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	// Stažení dat (pole pointerů ve struktuře SensorDTO)
	sensors, err := h.client.GetSensors()
	if err != nil {
		h.logger.Error("Chyba načítání dat", "error", err)
		http.Error(w, "Backend nedostupný", http.StatusBadGateway)
		return
	}

	data := map[string]interface{}{
		"Title":   "IoT Dashboard",
		"Sensors": sensors,
		"Page":    "index",
	}

	// Renderování konkrétní šablony "layout.html".
	// Uvnitř layout.html se volá {{ template "content" . }}, což vloží obsah z index.html.
	// Pokud bychom volali ExecuteTemplate s "index.html", chyběla by nám hlavička/patička.
	err = h.tmpl.ExecuteTemplate(w, "layout.html", data)

	if err != nil {
		h.logger.Error("Chyba renderování", "error", err)
	}
}

// HandleDetail: Graf historie
func (h *WebHandler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	// Získání ID z URL (Go 1.22 feature)
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}

	// Stažení bodů pro graf
	points, err := h.client.GetHistory(id, rng)
	if err != nil {
		h.logger.Error("Chyba API historie", "error", err)
		http.Error(w, "Chyba", 500)
		return
	}

	// Získání metadat senzoru pro nadpis grafu
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

	h.tmpl.ExecuteTemplate(w, "layout.html", data)
}
