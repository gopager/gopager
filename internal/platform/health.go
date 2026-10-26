package platform

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func HealthHandler(readStatus func() Status) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		writeHealth(w, r, "live", readStatus().Live)
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		writeHealth(w, r, "ready", readStatus().Ready)
	})
	return mux
}

func writeHealth(w http.ResponseWriter, r *http.Request, field string, value bool) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if field == "ready" && !value {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if err := json.NewEncoder(w).Encode(map[string]bool{field: value}); err != nil {
		slog.Error("health response write failed", "code", CodeInternalError)
	}
}
