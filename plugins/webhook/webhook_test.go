package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopager/gopager/pluginsdk"
	"github.com/gopager/gopager/pluginsdk/safehttp"
)

// captured is one request seen by the fake receiver.
type captured struct {
	header http.Header
	body   []byte
}

// receiver is a local webhook receiver answering with a fixed status.
type receiver struct {
	*httptest.Server
	mu       sync.Mutex
	requests []captured
}

func newReceiver(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) *receiver {
	t.Helper()
	rec := &receiver{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.requests = append(rec.requests, captured{r.Header.Clone(), body})
		rec.mu.Unlock()
		respond(w, r)
	}))
	t.Cleanup(rec.Close)
	return rec
}

func status(code int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func typed(kind string, value any) map[string]any {
	return map[string]any{"kind": kind, "value": value}
}

// request builds a delivery request around the typed configuration members.
func request(t *testing.T, config map[string]any, outputKind, body string, inputs map[string]any) pluginsdk.DeliveryRequest {
	t.Helper()
	members := map[string]any{}
	for name, value := range config {
		switch v := value.(type) {
		case bool:
			members[name] = typed("boolean", v)
		default:
			members[name] = typed("string", v)
		}
	}
	correlation := "corr-1"
	raw, err := json.Marshal(map[string]any{
		"schemaVersion": 2, "kind": "delivery", "instanceId": "inst-1",
		"configuration": typed("object", members),
		"attempt": map[string]any{
			"deliveryId": "del-1", "attemptId": "att-1", "attemptNumber": 3, "subscriberId": "sub-1",
			"correlationId": correlation, "causationId": "cause-1", "outputKind": outputKind, "body": body, "inputs": inputs,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var parsed pluginsdk.DeliveryRequest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

func plugin(now time.Time) *webhook {
	return &webhook{newClient: safehttp.NewClient, now: func() time.Time { return now }}
}

func deliver(t *testing.T, w *webhook, r pluginsdk.DeliveryRequest) pluginsdk.DeliveryResult {
	t.Helper()
	return w.Deliver(context.Background(), r)
}

func TestPostsJSONBody(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, status(http.StatusNoContent))
	req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true}, "json", `{"level":2}`,
		map[string]any{"topic": typed("string", "alarm")})
	result := deliver(t, plugin(time.Unix(1700000000, 0)), req)
	if result.Status != pluginsdk.StatusAccepted || result.Code != "accepted" || string(result.Receipt) != `{"status":204}` {
		t.Fatalf("%+v %s", result, result.Receipt)
	}
	if len(rec.requests) != 1 {
		t.Fatalf("%d requests", len(rec.requests))
	}
	got := rec.requests[0]
	if got.header.Get("Content-Type") != "application/json" || got.header.Get("User-Agent") != "GoPager-Webhook/1" ||
		got.header.Get("X-GoPager-Delivery") != "del-1" || got.header.Get("X-GoPager-Attempt") != "att-1" {
		t.Errorf("headers %v", got.header)
	}
	if got.header.Get("Authorization") != "" || got.header.Get("X-GoPager-Signature") != "" || got.header.Get("X-GoPager-Timestamp") != "" {
		t.Errorf("unconfigured credentials sent: %v", got.header)
	}
	var payload struct {
		DeliveryID    string         `json:"deliveryId"`
		AttemptID     string         `json:"attemptId"`
		AttemptNumber int            `json:"attemptNumber"`
		InstanceID    string         `json:"instanceId"`
		CorrelationID string         `json:"correlationId"`
		CausationID   string         `json:"causationId"`
		OutputKind    string         `json:"outputKind"`
		Body          map[string]any `json:"body"`
		Inputs        map[string]any `json:"inputs"`
	}
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.DeliveryID != "del-1" || payload.AttemptID != "att-1" || payload.AttemptNumber != 3 || payload.InstanceID != "inst-1" ||
		payload.CorrelationID != "corr-1" || payload.CausationID != "cause-1" || payload.OutputKind != "json" ||
		payload.Body["level"] != float64(2) || payload.Inputs["topic"] != "alarm" {
		t.Errorf("payload %s", got.body)
	}
}

func TestTextBodyIsAString(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, status(http.StatusOK))
	req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true}, "markdown", "# hi \"x\"", nil)
	if result := deliver(t, plugin(time.Now()), req); result.Status != pluginsdk.StatusAccepted {
		t.Fatalf("%+v", result)
	}
	var payload struct {
		Body   string         `json:"body"`
		Inputs map[string]any `json:"inputs"`
	}
	if err := json.Unmarshal(rec.requests[0].body, &payload); err != nil || payload.Body != "# hi \"x\"" || payload.Inputs == nil {
		t.Fatalf("%s %v", rec.requests[0].body, err)
	}
}

func TestInvalidJSONBodyIsPermanent(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, status(http.StatusOK))
	req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true}, "json", `{broken`, nil)
	result := deliver(t, plugin(time.Now()), req)
	if result.Status != pluginsdk.StatusPermanent || result.Code != "input-invalid" || len(rec.requests) != 0 {
		t.Fatalf("%+v", result)
	}
}

func TestClassification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		respond func(http.ResponseWriter, *http.Request)
		status  string
		code    string
		retry   int
	}{
		{"200", status(200), pluginsdk.StatusAccepted, "accepted", 0},
		{"204", status(204), pluginsdk.StatusAccepted, "accepted", 0},
		{"408", status(408), pluginsdk.StatusTransient, "provider-unavailable", 0},
		{"503", status(503), pluginsdk.StatusTransient, "provider-unavailable", 0},
		{"429 default", status(429), pluginsdk.StatusTransient, "rate-limited", 60},
		{"429 retry-after", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(429)
		}, pluginsdk.StatusTransient, "rate-limited", 120},
		{"429 capped", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "100000")
			w.WriteHeader(429)
		}, pluginsdk.StatusTransient, "rate-limited", 3600},
		{"500", status(500), pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"502", status(502), pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"504", status(504), pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"599", status(599), pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"302", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/other", 302) }, pluginsdk.StatusPermanent, "redirect-refused", 0},
		{"400", status(400), pluginsdk.StatusPermanent, "rejected", 0},
		{"401", status(401), pluginsdk.StatusPermanent, "rejected", 0},
		{"404", status(404), pluginsdk.StatusPermanent, "rejected", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rec := newReceiver(t, c.respond)
			req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true}, "text", "b", nil)
			result := deliver(t, plugin(time.Now()), req)
			if result.Status != c.status || result.Code != c.code || result.RetryAfterSeconds != c.retry {
				t.Fatalf("%+v", result)
			}
			if len(rec.requests) != 1 {
				t.Fatalf("a redirect or retry was followed: %d requests", len(rec.requests))
			}
		})
	}
}

func TestRedirectFollowedWhenEnabled(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(200)
	})
	req := request(t, map[string]any{"url": rec.URL + "/start", "allowPrivateDestinations": true, "followRedirects": true}, "text", "b", nil)
	if result := deliver(t, plugin(time.Now()), req); result.Status != pluginsdk.StatusAccepted {
		t.Fatalf("%+v", result)
	}
	if len(rec.requests) != 2 {
		t.Fatalf("%d requests", len(rec.requests))
	}
}

func TestBearerToken(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, status(http.StatusUnauthorized))
	req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true, "bearerToken": "s3cret"}, "text", "b", nil)
	result := deliver(t, plugin(time.Now()), req)
	if got := rec.requests[0].header.Get("Authorization"); got != "Bearer s3cret" {
		t.Errorf("Authorization %q", got)
	}
	if result.Status != pluginsdk.StatusPermanent || result.Code != "rejected" {
		t.Errorf("%+v", result)
	}
	if strings.Contains(result.Code, "s3cret") || strings.Contains(string(result.Receipt), "s3cret") {
		t.Errorf("secret leaked: %+v %s", result, result.Receipt)
	}
}

func TestSecretEchoedAsRequestIDIsNotReturned(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "id-s3cret")
		w.WriteHeader(200)
	})
	req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true, "bearerToken": "s3cret"}, "text", "b", nil)
	if result := deliver(t, plugin(time.Now()), req); strings.Contains(string(result.Receipt), "s3cret") || string(result.Receipt) != `{"status":200}` {
		t.Fatalf("%s", result.Receipt)
	}
}

func TestReceiptHoldsRequestID(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "req-42")
		w.WriteHeader(200)
	})
	req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true}, "text", "b", nil)
	if result := deliver(t, plugin(time.Now()), req); string(result.Receipt) != `{"status":200,"requestId":"req-42"}` {
		t.Fatalf("%s", result.Receipt)
	}
}

func TestHMACSignature(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, status(200))
	clock := time.Unix(1700000000, 0)
	for _, secret := range []string{"k", "k2"} {
		req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true, "hmacSecret": secret}, "text", "B", nil)
		deliver(t, plugin(clock), req)
	}
	for i, secret := range []string{"k", "k2"} {
		got := rec.requests[i]
		if got.header.Get("X-GoPager-Timestamp") != "1700000000" {
			t.Errorf("timestamp %q", got.header.Get("X-GoPager-Timestamp"))
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte("1700000000."))
		mac.Write(got.body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if got.header.Get("X-GoPager-Signature") != want {
			t.Errorf("signature with %q: got %q want %q", secret, got.header.Get("X-GoPager-Signature"), want)
		}
	}
	if rec.requests[0].header.Get("X-GoPager-Signature") == rec.requests[1].header.Get("X-GoPager-Signature") {
		t.Error("changing the secret did not change the signature")
	}
}

func TestPrivateDestinationsAreDeniedByDefault(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, status(200))
	req := request(t, map[string]any{"url": rec.URL}, "text", "b", nil)
	result := deliver(t, plugin(time.Now()), req)
	if result.Status != pluginsdk.StatusPermanent || result.Code != "destination-denied" || len(rec.requests) != 0 {
		t.Fatalf("%+v", result)
	}
	// An https URL passes the scheme rule and is stopped by the address rule.
	req = request(t, map[string]any{"url": "https://169.254.169.254/latest/meta-data"}, "text", "b", nil)
	if result := deliver(t, plugin(time.Now()), req); result.Status != pluginsdk.StatusPermanent || result.Code != "destination-denied" {
		t.Fatalf("%+v", result)
	}
}

func TestConnectionFailureIsTransient(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, status(200))
	url := rec.URL
	rec.Close()
	req := request(t, map[string]any{"url": url, "allowPrivateDestinations": true}, "text", "b", nil)
	if result := deliver(t, plugin(time.Now()), req); result.Status != pluginsdk.StatusTransient || result.Code != "connection-failed" {
		t.Fatalf("%+v", result)
	}
}

func TestLostResponseIsUncertain(t *testing.T) {
	t.Parallel()
	rec := newReceiver(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	})
	req := request(t, map[string]any{"url": rec.URL, "allowPrivateDestinations": true}, "text", "b", nil)
	if result := deliver(t, plugin(time.Now()), req); result.Status != pluginsdk.StatusUncertain || result.Code != "response-lost" {
		t.Fatalf("%+v", result)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for name, config := range map[string]map[string]any{
		"missing url": {},
		"bad url":     {"url": "not a url"},
	} {
		result := deliver(t, plugin(time.Now()), request(t, config, "text", "b", nil))
		if result.Status != pluginsdk.StatusPermanent || result.Code != "configuration-invalid" {
			t.Errorf("%s: %+v", name, result)
		}
	}
}

func TestManifestTemplate(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("manifest.template.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		SchemaVersion int    `json:"schemaVersion"`
		SDKMajor      int    `json:"sdkMajor"`
		SDKMinor      int    `json:"sdkMinor"`
		PluginID      string `json:"pluginId"`
		Version       string `json:"version"`
		CapabilityID  string `json:"capabilityId"`
		Capability    string `json:"capability"`
		Artifact      struct{ Path, Digest string }
		Schema        struct {
			Properties map[string]struct {
				Kind     string `json:"kind"`
				Optional bool   `json:"optional"`
			} `json:"properties"`
		} `json:"configurationSchema"`
		Policy struct {
			Scope      string `json:"configurationScope"`
			Properties map[string]struct {
				Scope  string `json:"configurationScope"`
				Secret bool   `json:"secret"`
			} `json:"properties"`
		} `json:"configurationPolicy"`
		Delivery struct {
			OutputKinds []string `json:"outputKinds"`
			Inputs      []struct {
				Name     string `json:"name"`
				Kind     string `json:"kind"`
				Required bool   `json:"required"`
				MaxBytes int    `json:"maxBytes"`
			} `json:"inputs"`
			SendsPerMinute int  `json:"sendsPerMinute"`
			Idempotent     bool `json:"idempotent"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion != 2 || m.SDKMajor != 2 || m.SDKMinor != pluginsdk.SDKMinor || m.PluginID != "webhook" || m.Version != "1.0.0" ||
		m.CapabilityID != "webhook" || m.Capability != pluginsdk.KindDelivery ||
		m.Artifact.Path != "webhook/1.0.0/webhook" || m.Artifact.Digest != "@DIGEST@" {
		t.Errorf("identity %+v", m)
	}
	secrets := map[string]bool{"bearerToken": true, "hmacSecret": true}
	if len(m.Schema.Properties) != 5 || len(m.Policy.Properties) != 5 || m.Policy.Scope != "admin" {
		t.Errorf("schema %+v policy %+v", m.Schema, m.Policy)
	}
	for name, property := range m.Policy.Properties {
		if _, ok := m.Schema.Properties[name]; !ok || property.Secret != secrets[name] || property.Scope != "admin" {
			t.Errorf("policy %s: %+v", name, property)
		}
		if property.Secret && m.Schema.Properties[name].Kind != "string" {
			t.Errorf("secret %s is not a string", name)
		}
	}
	if m.Schema.Properties["url"].Optional || !m.Schema.Properties["bearerToken"].Optional {
		t.Errorf("optional flags %+v", m.Schema.Properties)
	}
	if len(m.Delivery.Inputs) != 1 || m.Delivery.Inputs[0].Name != "topic" || m.Delivery.Inputs[0].MaxBytes != 256 ||
		strings.Join(m.Delivery.OutputKinds, ",") != "json,text,markdown" || m.Delivery.SendsPerMinute != 0 || m.Delivery.Idempotent {
		t.Errorf("delivery %+v", m.Delivery)
	}
}
