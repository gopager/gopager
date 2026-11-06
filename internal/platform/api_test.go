package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestApplicationRegistrationPreservesHealthAndReadiness(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &RunningHandle{root: ctx, status: Status{Live: true, Ready: true}}
	var calls atomic.Int64
	handler := h.applicationHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/health/live", "/health/ready"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || calls.Load() != 0 {
			t.Fatalf("health route %s was overridden: %d, calls %d", path, recorder.Code, calls.Load())
		}
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Post(server.URL+"/api/v1/auth/login", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent || calls.Load() != 1 {
		t.Fatalf("HTTP API was not served: %d, calls %d", response.StatusCode, calls.Load())
	}
	h.mu.Lock()
	h.status.Ready = false
	h.mu.Unlock()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/private", nil))
	if recorder.Code != http.StatusServiceUnavailable || calls.Load() != 1 ||
		!strings.Contains(recorder.Body.String(), `"code":"AUTHORITY_UNAVAILABLE"`) {
		t.Fatalf("unavailable API did not fail closed: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestApplicationRegistrationRejectsNewWorkAfterCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	h := &RunningHandle{root: ctx, status: Status{Live: true, Ready: true}}
	handler := h.applicationHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("cancelled platform dispatched application work")
	}))
	cancel()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/private", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("cancelled API status = %d", recorder.Code)
	}
}
