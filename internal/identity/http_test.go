package identity

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/authz"
)

func TestHTTPProofSelectionNeverFallsBack(t *testing.T) {
	transport, err := NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name          string
		authorization []string
		cookies       []string
		kind          ProofKind
		secret        string
		reject        bool
	}{
		{name: "explicit anonymous", kind: AnonymousProof},
		{name: "session", cookies: []string{SessionCookieName + "=" + session}, kind: SessionProof, secret: session},
		{name: "bearer", authorization: []string{"Bearer " + key}, kind: AccessKeyProof, secret: key},
		{name: "mixed explicit key", authorization: []string{"bEaReR " + key},
			cookies: []string{SessionCookieName + "=" + session}, kind: AccessKeyProof, secret: key},
		{name: "mixed ignores malformed cookie", authorization: []string{"Bearer " + key},
			cookies: []string{SessionCookieName + "=bad"}, kind: AccessKeyProof, secret: key},
		{name: "invalid key never becomes session", authorization: []string{"Bearer bad"},
			cookies: []string{SessionCookieName + "=" + session}, reject: true},
		{name: "empty authorization never becomes anonymous", authorization: []string{""}, reject: true},
		{name: "basic never becomes anonymous", authorization: []string{"Basic " + key}, reject: true},
		{name: "duplicate authorization", authorization: []string{"Bearer " + key, "Bearer " + key}, reject: true},
		{name: "comma-joined authorization", authorization: []string{"Bearer " + key + ", Bearer " + key}, reject: true},
		{name: "empty session never becomes anonymous", cookies: []string{SessionCookieName + "="}, reject: true},
		{name: "malformed session never becomes anonymous", cookies: []string{SessionCookieName}, reject: true},
		{name: "skipped malformed cookie never becomes anonymous", cookies: []string{SessionCookieName + "=\"bad\\value\""}, reject: true},
		{name: "duplicate session line", cookies: []string{SessionCookieName + "=" + session + "; " + SessionCookieName + "=" + session}, reject: true},
		{name: "duplicate session headers", cookies: []string{SessionCookieName + "=" + session, SessionCookieName + "=" + session}, reject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", nil)
			request.Header["Authorization"] = test.authorization
			request.Header["Cookie"] = test.cookies
			actual, err := transport.SelectProof(request)
			if test.reject {
				if !errors.Is(err, ErrLoginRequired) || actual.Kind != "" || actual.Secret != "" {
					t.Fatalf("bad proof fell back: kind=%q error=%v", actual.Kind, err)
				}
			} else if err != nil || actual.Kind != test.kind || actual.Secret != test.secret {
				t.Fatalf("wrong proof selection: kind=%q error=%v", actual.Kind, err)
			}
		})
	}
}

func TestHTTPOriginValidation(t *testing.T) {
	for _, origin := range []string{
		"", "null", "//example.test", "ftp://example.test", "http://user@example.test",
		"http://example.test/path", "http://example.test?", "http://example.test#",
		"http://example.test?x=y", "http://example.test/#fragment", "http://:80",
		"http://example.test:", "http://example.test:65536", "http://[fe80::1%25en0]",
	} {
		if _, err := NewHTTPTransport(origin); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("invalid public origin accepted: %q, %v", origin, err)
		}
	}
	for _, pair := range [][2]string{
		{"HTTPS://EXAMPLE.test:443/", "https://example.test"},
		{"http://example.test:80", "http://example.test"},
		{"http://[::1]:8080", "http://[0:0:0:0:0:0:0:1]:8080"},
		{"https://b\u00fccher.example", "https://xn--bcher-kva.example"},
	} {
		transport, err := NewHTTPTransport(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("Origin", pair[1])
		if err := transport.CheckBrowserOrigin(request); err != nil {
			t.Fatalf("equivalent origin rejected: %q, %v", pair[0], err)
		}
		for _, other := range []string{"null", pair[1] + "/", pair[1] + "/path", "https://foreign.example"} {
			request.Header.Set("Origin", other)
			if err := transport.CheckBrowserOrigin(request); !errors.Is(err, authz.ErrForbidden) {
				t.Fatalf("foreign/malformed browser origin accepted: %q, %v", other, err)
			}
		}
	}
}

func TestHTTPAndTLSProxySessionProtection(t *testing.T) {
	for _, behindTLSProxy := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct HTTP", true: "HTTPS edge HTTP upstream"}[behindTLSProxy], func(t *testing.T) {
			session, _, err := NewSecret()
			if err != nil {
				t.Fatal(err)
			}
			csrf, _, err := NewSecret()
			if err != nil {
				t.Fatal(err)
			}
			var transport *HTTPTransport
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.TLS != nil {
					t.Error("test must reach application over HTTP")
				}
				if r.URL.Path == "/issue" {
					cookie, err := transport.SessionCookie(session, time.Now().Add(time.Hour))
					if err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					http.SetCookie(w, cookie)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				proof, err := transport.SelectProof(r)
				if err != nil {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				// This fixture isolates transport policy; authority resolution has separate primary-store tests.
				if proof.Kind != SessionProof || proof.Secret != session {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if err := transport.CheckStateChange(r, Authentication{Kind: proof.Kind, CSRFToken: csrf}); err != nil {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer backend.Close()
			edge := backend
			if behindTLSProxy {
				upstream, err := url.Parse(backend.URL)
				if err != nil {
					t.Fatal(err)
				}
				edge = httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
				defer edge.Close()
			}
			transport, err = NewHTTPTransport(edge.URL)
			if err != nil {
				t.Fatal(err)
			}
			client := edge.Client()
			client.Jar, err = cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Get(edge.URL + "/issue")
			if err != nil {
				t.Fatal(err)
			}
			cookies := response.Cookies()
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusNoContent || len(cookies) != 1 {
				t.Fatalf("Session cookie not issued over HTTP upstream: %d", response.StatusCode)
			}
			cookie := cookies[0]
			if cookie.Secure != behindTLSProxy || !cookie.HttpOnly || cookie.Domain != "" ||
				cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatal("wrong public-origin Session cookie attributes")
			}
			cleared := transport.ClearSessionCookie()
			if cleared.Name != cookie.Name || cleared.Path != cookie.Path || cleared.Domain != "" ||
				cleared.MaxAge != -1 || cleared.Secure != behindTLSProxy {
				t.Fatal("logout cookie does not clear the same Session")
			}
			for _, test := range []struct {
				name    string
				origins []string
				tokens  []string
				status  int
			}{
				{"valid despite forged forwarding", []string{edge.URL}, []string{csrf}, http.StatusNoContent},
				{"foreign origin", []string{"https://foreign.example"}, []string{csrf}, http.StatusForbidden},
				{"absent origin", nil, []string{csrf}, http.StatusForbidden},
				{"duplicate origin", []string{edge.URL, edge.URL}, []string{csrf}, http.StatusForbidden},
				{"missing CSRF", []string{edge.URL}, nil, http.StatusForbidden},
				{"bad CSRF", []string{edge.URL}, []string{"bad"}, http.StatusForbidden},
				{"duplicate CSRF", []string{edge.URL}, []string{csrf, csrf}, http.StatusForbidden},
			} {
				t.Run(test.name, func(t *testing.T) {
					request, err := http.NewRequest(http.MethodPost, edge.URL+"/write", strings.NewReader("{}"))
					if err != nil {
						t.Fatal(err)
					}
					request.Header["Origin"] = test.origins
					request.Header[CSRFHeaderName] = test.tokens
					request.Header.Set("Forwarded", "proto=http;host=forged.example")
					request.Header.Set("X-Forwarded-Proto", "http")
					request.Header.Set("X-Forwarded-Host", "forged.example")
					response, err := client.Do(request)
					if err != nil {
						t.Fatal(err)
					}
					if err := response.Body.Close(); err != nil {
						t.Fatal(err)
					}
					if response.StatusCode != test.status {
						t.Fatalf("HTTP Session protection returned %d, want %d", response.StatusCode, test.status)
					}
				})
			}
		})
	}
}
