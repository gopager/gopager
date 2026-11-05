package platform

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// applicationHandler retains platform health routes and gates application work
// at the same readiness/shutdown boundary used by transaction participants.
func (h *RunningHandle) applicationHandler(api http.Handler) http.Handler {
	if api == nil {
		return HealthHandler(h.ReadStatus)
	}
	mux := http.NewServeMux()
	health := HealthHandler(h.ReadStatus)
	mux.Handle("/health/live", health)
	mux.Handle("/health/ready", health)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		ready := !h.stopping && h.status.Ready && h.root.Err() == nil
		if ready {
			h.operations.Add(1)
		}
		h.mu.Unlock()
		if !ready {
			writeAPIUnavailable(w)
			return
		}
		defer h.operations.Done()
		api.ServeHTTP(w, r)
	}))
	return mux
}

// writeAPIUnavailable answers 503 with the public JSON error envelope for
// CodeAuthorityUnavailable (retryable, fresh request ID) and nothing else.
func writeAPIUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	body := struct {
		SchemaVersion int            `json:"schemaVersion"`
		Code          ErrorCode      `json:"code"`
		Parameters    map[string]any `json:"parameters"`
		TextKey       string         `json:"textKey"`
		Fallback      string         `json:"fallback"`
		RequestID     string         `json:"requestId"`
		Retryable     bool           `json:"retryable"`
	}{
		SchemaVersion: 1, Code: CodeAuthorityUnavailable, Parameters: map[string]any{},
		TextKey: "error.AUTHORITY_UNAVAILABLE", Fallback: englishFallbacks[CodeAuthorityUnavailable],
		RequestID: uuid.NewString(), Retryable: true,
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("API availability response write failed", "code", CodeInternalError)
	}
}
