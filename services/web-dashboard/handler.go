package main

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
)

// WebHandler řídí zpracování HTTP požadavků.
// Drží oddělené šablony pro Index a Detail, aby nedocházelo ke kolizím názvů bloků.
type WebHandler struct {
	client     *APIClient         // Klient pro volání backend API
	logger     *slog.Logger       // Logger
	indexTmpl  *template.Template // Šablona pro Dashboard (přehled)
	detailTmpl *template.Template // Šablona pro Graf (historie)
}

// SystemWidgetData je pomocná struktura (ViewModel).
// Slouží k tomu, abychom v Go kódu seskupili rozházené senzory do jednoho logického celku
// pro zobrazení "System Status" widgetu v HTML.
type SystemWidgetData struct {
	CPUPercent float64 // Vytížení CPU

	RamUsed  float64 // Použitá RAM (MB)
	RamTotal float64 // Celková RAM (MB)
	RamFree  float64 // Dopočítaná volná RAM (Total - Used)

	DiskUsed  float64 // Použitý Disk (GB)
	DiskTotal float64 // Celkový Disk (GB)
	DiskFree  float64 // Dopočítané volné místo (Total - Used)

	HasData bool // Příznak: True, pokud jsme našli alespoň nějaká systémová data.
}

// NewWebHandler inicializuje handler a parsuje HTML šablony.
func NewWebHandler(client *APIClient, logger *slog.Logger) (*WebHandler, error) {

	// 1. DEFINICE POMOCNÝCH FUNKCÍ (FuncMap)
	// Tyto funkce můžeme volat přímo v HTML kódu (např. {{ .Value | deref }}).
	funcMap := template.FuncMap{

		// "deref": Bezpečně získá hodnotu z pointeru.
		// Pokud je pointer nil (senzor neposlal data), vrátí 0.0.
		"deref": func(f *float64) float64 {
			if f == nil {
				return 0.0
			}
			return *f
		},

		// "to_json": Serializuje Go strukturu na JSON string.
		// Klíčové pro předání dat do JavaScriptu (Chart.js).
		// template.JS říká šabloně: "Neescapuj uvozovky, toto je bezpečný skript".
		"to_json": func(v interface{}) template.JS {
			a, err := json.Marshal(v)
			if err != nil {
				return template.JS("[]") // Fallback při chybě
			}
			return template.JS(a)
		},
	}

	// 2. NAČTENÍ ŠABLON (Izolace)
	// Pro každou stránku vytváříme samostatnou instanci Template.
	// Tím řešíme problém, kdy obě stránky definují blok {{define "content"}}.

	// A) Index (Dashboard)
	indexTmpl := template.New("layout.html").Funcs(funcMap)
	indexTmpl, err := indexTmpl.ParseFiles(
		filepath.Join("templates", "layout.html"),
		filepath.Join("templates", "index.html"),
	)
	if err != nil {
		return nil, err
	}

	// B) Detail (Graf)
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

// HandleIndex: Hlavní stránka (Dashboard)
func (h *WebHandler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	// 1. Získání surových dat z API (seznam všech senzorů)
	sensors, err := h.client.GetSensors()
	if err != nil {
		h.logger.Error("Chyba při volání API", "error", err)
		http.Error(w, "Backend API je nedostupné", http.StatusBadGateway)
		return
	}

	// 2. LOGIKA AGREGACE DAT PRO SYSTEM WIDGET
	// Projdeme seznam senzorů a "vytaháme" z něj ty systémové podle MQTT topicu.
	sysData := SystemWidgetData{}

	for _, s := range sensors {
		// Získáme hodnotu (dereference), pokud existuje.
		val := 0.0
		if s.CurrentValue != nil {
			val = *s.CurrentValue
		}

		// Rozhodování podle Topiců (tyto topicy jsme definovali v DB).
		switch s.Topic {
		case "/msh/system/cpu":
			sysData.CPUPercent = val
			sysData.HasData = true // Našli jsme CPU, zapneme zobrazení widgetu
		case "/msh/system/ram_used":
			sysData.RamUsed = val
		case "/msh/system/ram_total":
			sysData.RamTotal = val
		case "/msh/system/disk_used":
			sysData.DiskUsed = val
		case "/msh/system/disk_total":
			sysData.DiskTotal = val
		}
	}

	// 3. DOPOČTY (Business Logic ve View Layeru)
	// Grafy potřebují "Used" a "Free". Senzory posílají "Used" a "Total".
	// Musíme dopočítat zbytek.
	if sysData.RamTotal > 0 {
		sysData.RamFree = sysData.RamTotal - sysData.RamUsed
	}
	if sysData.DiskTotal > 0 {
		sysData.DiskFree = sysData.DiskTotal - sysData.DiskUsed
	}

	// 4. Příprava dat pro šablonu
	data := map[string]interface{}{
		"Title":      "IoT Dashboard",
		"Sensors":    sensors, // Seznam všech senzorů (pro spodní část stránky)
		"SystemInfo": sysData, // Data pro koláčové grafy
		"Page":       "index",
	}

	// 5. Renderování
	err = h.indexTmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		h.logger.Error("Chyba renderování indexu", "error", err)
	}
}

// HandleDetail: Stránka s grafem historie
func (h *WebHandler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	// Extrakce ID z URL
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	// Range parametr
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}

	// Stažení historie
	points, err := h.client.GetHistory(id, rng)
	if err != nil {
		h.logger.Error("Chyba API historie", "error", err)
		http.Error(w, "Chyba načítání dat", http.StatusInternalServerError)
		return
	}

	// Dohledání metadat senzoru (jméno, jednotka)
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

	err = h.detailTmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		h.logger.Error("Chyba renderování detailu", "error", err)
	}
}
