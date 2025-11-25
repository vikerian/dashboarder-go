package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// APIHandler sdružuje metody pro obsluhu HTTP požadavků.
// Drží referenci na Service (logika) a Logger.
type APIHandler struct {
	svc    *Service
	logger *slog.Logger
}

// NewAPIHandler vytváří novou instanci handleru.
func NewAPIHandler(svc *Service, logger *slog.Logger) *APIHandler {
	return &APIHandler{svc: svc, logger: logger}
}

// RegisterRoutes mapuje URL cesty na konkrétní Go funkce.
// Využíváme nový router v Go 1.22+, který podporuje metody a wildcardy.
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	// Endpoint pro seznam senzorů (Dashboard)
	mux.HandleFunc("GET /api/sensors", h.handleListSensors)

	// Endpoint pro detail senzoru (Graf).
	// {id} je tzv. Path Value - proměnná v URL.
	mux.HandleFunc("GET /api/sensors/{id}/history", h.handleGetHistory)
}

// handleListSensors: GET /api/sensors
func (h *APIHandler) handleListSensors(w http.ResponseWriter, r *http.Request) {
	// Získání kontextu z requestu (pro timeouty a cancelation)
	ctx := r.Context()

	// Volání business logiky
	sensors, err := h.svc.GetAllSensors(ctx)
	if err != nil {
		h.logger.Error("Chyba při získávání senzorů", "error", err)
		http.Error(w, "Interní chyba serveru", http.StatusInternalServerError)
		return
	}

	// Nastavení hlavičky, že vracíme JSON
	w.Header().Set("Content-Type", "application/json")

	// Serializace (Encoding) struktury do JSONu přímo do HTTP odpovědi.
	// json.NewEncoder je efektivnější než json.Marshal pro streamování dat.
	if err := json.NewEncoder(w).Encode(sensors); err != nil {
		h.logger.Error("Chyba při zápisu JSON odpovědi", "error", err)
	}
}

// handleGetHistory: GET /api/sensors/{id}/history?range=24h
func (h *APIHandler) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	// 1. Extrakce ID z URL (Go 1.22 feature)
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Neplatné ID senzoru (musí být číslo)", http.StatusBadRequest)
		return
	}

	// 2. Extrakce parametru 'range' z query stringu
	// Příklad URL: .../history?range=1h
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "24h" // Defaultní hodnota, pokud parametr chybí
	}

	// 3. Volání business logiky
	points, err := h.svc.GetHistory(r.Context(), id, rangeParam)
	if err != nil {
		h.logger.Error("Chyba při získávání historie", "id", id, "error", err)
		http.Error(w, "Chyba při načítání dat", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

// CorsMiddleware je "obalová" funkce (Middleware).
// Přidává HTTP hlavičky, které povolí prohlížeči (např. React appce na localhost:3000)
// volat toto API běžící na jiném portu/doméně.
func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Povolíme přístup odkudkoliv (*) - v produkci zde má být konkrétní doména.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Pokud jde o "Preflight" request (prohlížeč se ptá "můžu?"), odpovíme OK a končíme.
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Jinak předáme řízení dál našemu handleru.
		next.ServeHTTP(w, r)
	})
}
