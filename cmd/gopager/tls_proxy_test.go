package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
)

// TestTLSReverseProxyServesLoginAndLogoutOverProductionWiring is the one end-to-end test of the
// TLS reverse proxy path: the production handler (composeApplicationAPI) sits behind an HTTPS
// edge and receives plain HTTP, with forged forwarding headers on every request. It follows a
// browser through login, a CSRF-protected mutation and logout. The transport policy itself is
// tested per case in identity.TestHTTPAndTLSProxySessionProtection; feature tests use plain HTTP.
func TestTLSReverseProxyServesLoginAndLogoutOverProductionWiring(t *testing.T) {
	t.Parallel()
	store := newManagementStore(t)
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	target, err := url.Parse("http://" + backend.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
	t.Cleanup(proxy.Close)
	origin := "https://" + proxy.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), mustComposeCore(t, store),
		managementNetworkFixture{id: uuid.New(), agency: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			t.Error("application was not reached over plain HTTP")
		}
		handler.ServeHTTP(w, r)
	})
	backend.Start()
	proxy.StartTLS()
	client := proxy.Client()
	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	send := func(method, path, body, requestOrigin, csrf string, status int) ([]byte, []*http.Cookie) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), method, origin+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if requestOrigin != "" {
			request.Header.Set("Origin", requestOrigin)
		}
		if csrf != "" {
			request.Header.Set(identity.CSRFHeaderName, csrf)
		}
		request.Header.Set("Forwarded", "proto=http;host=forged.example")
		request.Header.Set("X-Forwarded-Proto", "http")
		request.Header.Set("X-Forwarded-Host", "forged.example")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(response.Body)
		if err := errors.Join(readErr, response.Body.Close()); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, response.StatusCode, status, content)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s response is cacheable", method, path)
		}
		return content, response.Cookies()
	}
	login := `{"schemaVersion":1,"username":"operator","password":"initial-password"}`

	send(http.MethodPost, "/api/v1/auth/login", login, "https://foreign.example", "", http.StatusForbidden)
	content, cookies := send(http.MethodPost, "/api/v1/auth/login", login, origin, "", http.StatusOK)
	var session struct {
		SchemaVersion int    `json:"schemaVersion"`
		SessionID     string `json:"sessionId"`
		CSRFToken     string `json:"csrfToken"`
	}
	if err := json.Unmarshal(content, &session); err != nil {
		t.Fatal(err)
	}
	if session.SchemaVersion != 1 || session.CSRFToken == "" || len(cookies) != 1 {
		t.Fatalf("login did not issue a browser Session: %s", content)
	}
	cookie := cookies[0]
	if !cookie.Secure || !cookie.HttpOnly {
		t.Fatalf("Session cookie is not Secure and HttpOnly behind the TLS proxy: %+v", cookie)
	}
	if strings.Contains(string(content), cookie.Value) || strings.Contains(string(content), "initial-password") {
		t.Fatal("login JSON leaks the password or the cookie secret")
	}

	content, _ = send(http.MethodGet, "/api/v1/auth/session", "", "", "", http.StatusOK)
	var current struct {
		SessionID string `json:"sessionId"`
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(content, &current); err != nil || current.SessionID != session.SessionID ||
		current.CSRFToken != session.CSRFToken {
		t.Fatalf("browser reload cannot recover its Session and CSRF state: %s (%v)", content, err)
	}

	collection := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","name":"Stations"}`
	send(http.MethodPost, "/api/v1/unit-collections", collection, origin, "", http.StatusForbidden)
	send(http.MethodPost, "/api/v1/unit-collections", collection, "https://foreign.example", session.CSRFToken, http.StatusForbidden)
	send(http.MethodPost, "/api/v1/unit-collections", collection, origin, session.CSRFToken, http.StatusCreated)

	send(http.MethodPost, "/api/v1/auth/logout", "", origin, "", http.StatusForbidden)
	send(http.MethodPost, "/api/v1/auth/logout", "", "https://foreign.example", session.CSRFToken, http.StatusForbidden)
	_, cleared := send(http.MethodPost, "/api/v1/auth/logout", "", origin, session.CSRFToken, http.StatusNoContent)
	if len(cleared) != 1 || cleared[0].Name != cookie.Name || cleared[0].MaxAge != -1 || !cleared[0].Secure {
		t.Fatalf("logout did not clear the Session cookie: %+v", cleared)
	}
	send(http.MethodGet, "/api/v1/auth/session", "", "", "", http.StatusUnauthorized)
	send(http.MethodPost, "/api/v1/auth/logout", "", origin, session.CSRFToken, http.StatusUnauthorized)

	// The stored Session is revoked, not only forgotten by the browser: replay the old secret.
	replay, err := http.NewRequestWithContext(t.Context(), http.MethodGet, origin+"/api/v1/auth/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	replay.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	response, err := (&http.Client{Transport: client.Transport}).Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logout did not revoke the stored Session: replay returned %d", response.StatusCode)
	}
	send(http.MethodPost, "/api/v1/auth/login", login, origin, "", http.StatusOK)
}
