package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gopager/gopager/pluginsdk"
	"github.com/gopager/gopager/pluginsdk/safehttp"
)

// fakeAPI is a local Pushover API answering with a fixed status and body.
type fakeAPI struct {
	*httptest.Server
	mu    sync.Mutex
	forms []url.Values
	query []string
	types []string
}

func newFakeAPI(t *testing.T, code int, body string, header map[string]string) *fakeAPI {
	t.Helper()
	api := &fakeAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		api.mu.Lock()
		api.forms = append(api.forms, r.PostForm)
		api.query = append(api.query, r.URL.RawQuery)
		api.types = append(api.types, r.Header.Get("Content-Type"))
		api.mu.Unlock()
		for name, value := range header {
			w.Header().Set(name, value)
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(api.Close)
	return api
}

func typed(kind string, value any) map[string]any {
	return map[string]any{"kind": kind, "value": value}
}

// request builds a delivery request; config values may be string, bool or int (integer kind).
func request(t *testing.T, config map[string]any, body string, inputs map[string]any) pluginsdk.DeliveryRequest {
	t.Helper()
	members := map[string]any{}
	for name, value := range config {
		switch v := value.(type) {
		case bool:
			members[name] = typed("boolean", v)
		case int:
			members[name] = typed("integer", strconv.Itoa(v))
		default:
			members[name] = typed("string", v)
		}
	}
	raw, err := json.Marshal(map[string]any{
		"schemaVersion": 2, "kind": "delivery", "instanceId": "inst-1",
		"configuration": typed("object", members),
		"attempt": map[string]any{
			"deliveryId": "del-1", "attemptId": "att-1", "attemptNumber": 1, "subscriberId": "sub-1",
			"causationId": "cause-1", "outputKind": "text", "body": body, "inputs": inputs,
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

func deliver(t *testing.T, r pluginsdk.DeliveryRequest) pluginsdk.DeliveryResult {
	t.Helper()
	return (&pushover{newClient: safehttp.NewClient}).Deliver(context.Background(), r)
}

func base(api *fakeAPI) map[string]any {
	return map[string]any{"apiToken": "app-token", "userKey": "user-key", "endpoint": api.URL, "allowPrivateDestinations": true}
}

const okBody = `{"status":1,"request":"req-1"}`

func TestAdminTokenAndPersonalUserKeyAreSent(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 200, okBody, nil)
	config := base(api)
	config["device"] = "phone"
	config["title"] = "Alarm"
	config["sound"] = "siren"
	config["priority"] = 1
	result := deliver(t, request(t, config, "fire at 5", nil))
	if result.Status != pluginsdk.StatusAccepted || result.Code != "accepted" || string(result.Receipt) != `{"status":200,"request":"req-1"}` {
		t.Fatalf("%+v %s", result, result.Receipt)
	}
	form := api.forms[0]
	want := map[string]string{"token": "app-token", "user": "user-key", "message": "fire at 5", "title": "Alarm",
		"priority": "1", "sound": "siren", "device": "phone"}
	for name, value := range want {
		if form.Get(name) != value {
			t.Errorf("%s = %q, want %q", name, form.Get(name), value)
		}
	}
	if api.query[0] != "" || api.types[0] != "application/x-www-form-urlencoded" {
		t.Errorf("credentials must travel in the body: query %q type %q", api.query[0], api.types[0])
	}
	if form.Has("url") || form.Has("url_title") {
		t.Errorf("unset fields sent: %v", form)
	}
}

func TestInputsOverrideDefaults(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 200, okBody, nil)
	config := base(api)
	config["title"] = "Default"
	config["priority"] = 0
	config["sound"] = "pushover"
	inputs := map[string]any{
		"title": typed("string", "Override"), "priority": typed("integer", "-2"), "url": typed("string", "https://x.example/a"),
		"urlTitle": typed("string", "Open"), "sound": typed("string", "bike"),
	}
	deliver(t, request(t, config, "m", inputs))
	form := api.forms[0]
	for name, want := range map[string]string{"title": "Override", "priority": "-2", "url": "https://x.example/a", "url_title": "Open", "sound": "bike"} {
		if form.Get(name) != want {
			t.Errorf("%s = %q, want %q", name, form.Get(name), want)
		}
	}
}

func TestInvalidInputsArePermanent(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 200, okBody, nil)
	for name, inputs := range map[string]map[string]any{
		"priority too high": {"priority": typed("integer", "2")},
		"priority too low":  {"priority": typed("integer", "-3")},
		"priority kind":     {"priority": typed("string", "1")},
		"title kind":        {"title": typed("integer", "1")},
	} {
		result := deliver(t, request(t, base(api), "m", inputs))
		if result.Status != pluginsdk.StatusPermanent || result.Code != "input-invalid" {
			t.Errorf("%s: %+v", name, result)
		}
	}
	if len(api.forms) != 0 {
		t.Errorf("an invalid input reached the provider")
	}
}

func TestMissingUserKeyIsRecipientMissing(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 200, okBody, nil)
	config := base(api)
	delete(config, "userKey")
	result := deliver(t, request(t, config, "m", nil))
	if result.Status != pluginsdk.StatusPermanent || result.Code != "recipient-missing" || len(api.forms) != 0 {
		t.Fatalf("%+v", result)
	}
}

func TestLongMessageIsTruncated(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 200, okBody, nil)
	body := strings.Repeat("ä", 1500)
	result := deliver(t, request(t, base(api), body, nil))
	if got := []rune(api.forms[0].Get("message")); len(got) != 1024 {
		t.Fatalf("message has %d characters", len(got))
	}
	if string(result.Receipt) != `{"status":200,"request":"req-1","truncated":true}` {
		t.Fatalf("%s", result.Receipt)
	}
	deliver(t, request(t, base(api), strings.Repeat("a", 1024), nil))
	if len(api.forms[1].Get("message")) != 1024 {
		t.Fatal("a message of exactly the limit was cut")
	}
}

func TestClassification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		code   int
		body   string
		header map[string]string
		status string
		result string
		retry  int
	}{
		{"accepted", 200, okBody, nil, pluginsdk.StatusAccepted, "accepted", 0},
		{"status zero with 200", 200, `{"status":0}`, nil, pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"not json with 200", 200, `<html>`, nil, pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"rejected", 400, `{"status":0,"errors":["message cannot be blank"],"request":"r"}`, nil, pluginsdk.StatusPermanent, "rejected", 0},
		{"user invalid", 400, `{"user":"invalid","errors":["user identifier is invalid"],"status":0,"request":"r"}`, nil, pluginsdk.StatusPermanent, "recipient-invalid", 0},
		{"400 without status", 400, `nope`, nil, pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"500", 500, ``, nil, pluginsdk.StatusTransient, "provider-unavailable", 0},
		{"503", 503, ``, nil, pluginsdk.StatusTransient, "provider-unavailable", 0},
		{"429 default", 429, ``, nil, pluginsdk.StatusTransient, "rate-limited", 60},
		{"429 retry-after", 429, ``, map[string]string{"Retry-After": "90"}, pluginsdk.StatusTransient, "rate-limited", 90},
		{"429 capped", 429, ``, map[string]string{"Retry-After": "999999"}, pluginsdk.StatusTransient, "rate-limited", 3600},
		{"404", 404, ``, nil, pluginsdk.StatusUncertain, "unexpected-response", 0},
		{"302", 302, ``, map[string]string{"Location": "/x"}, pluginsdk.StatusUncertain, "unexpected-response", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			api := newFakeAPI(t, c.code, c.body, c.header)
			result := deliver(t, request(t, base(api), "m", nil))
			if result.Status != c.status || result.Code != c.result || result.RetryAfterSeconds != c.retry {
				t.Fatalf("%+v", result)
			}
			if len(api.forms) != 1 {
				t.Fatalf("%d requests: a redirect was followed or the request repeated", len(api.forms))
			}
		})
	}
}

func TestSecretsNeverAppearInResults(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 400, `{"status":0,"user":"invalid","request":"echo-user-key"}`, nil)
	result := deliver(t, request(t, base(api), "m", nil))
	text := result.Code + string(result.Receipt)
	if strings.Contains(text, "user-key") || strings.Contains(text, "app-token") {
		t.Fatalf("secret leaked: %+v %s", result, result.Receipt)
	}
}

func TestPrivateDestinationIsDeniedByDefault(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 200, okBody, nil)
	config := base(api)
	config["allowPrivateDestinations"] = false
	result := deliver(t, request(t, config, "m", nil))
	if result.Status != pluginsdk.StatusPermanent || result.Code != "destination-denied" || len(api.forms) != 0 {
		t.Fatalf("%+v", result)
	}
}

func TestConnectionFailureIsTransient(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t, 200, okBody, nil)
	config := base(api)
	api.Close()
	if result := deliver(t, request(t, config, "m", nil)); result.Status != pluginsdk.StatusTransient || result.Code != "connection-failed" {
		t.Fatalf("%+v", result)
	}
}

func TestLostResponseIsUncertain(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	}))
	defer server.Close()
	config := map[string]any{"apiToken": "t", "userKey": "u", "endpoint": server.URL, "allowPrivateDestinations": true}
	if result := deliver(t, request(t, config, "m", nil)); result.Status != pluginsdk.StatusUncertain || result.Code != "response-lost" {
		t.Fatalf("%+v", result)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for name, config := range map[string]map[string]any{
		"missing token": {"userKey": "u"},
		"bad endpoint":  {"apiToken": "t", "userKey": "u", "endpoint": "nonsense"},
	} {
		result := deliver(t, request(t, config, "m", nil))
		if result.Status != pluginsdk.StatusPermanent || result.Code != "configuration-invalid" {
			t.Errorf("%s: %+v", name, result)
		}
	}
	// The default endpoint is https, so it passes the scheme check and fails on the network
	// only with an injected client; here an http endpoint without allowPrivate is refused.
	result := deliver(t, request(t, map[string]any{"apiToken": "t", "userKey": "u", "endpoint": "http://a.example/"}, "m", nil))
	if result.Status != pluginsdk.StatusPermanent || result.Code != "destination-denied" {
		t.Errorf("http endpoint: %+v", result)
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
			Properties map[string]struct {
				Scope  string `json:"configurationScope"`
				Secret bool   `json:"secret"`
			} `json:"properties"`
		} `json:"configurationPolicy"`
		Delivery struct {
			OutputKinds    []string `json:"outputKinds"`
			Inputs         []struct{ Name, Kind string }
			SendsPerMinute int  `json:"sendsPerMinute"`
			Idempotent     bool `json:"idempotent"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion != 2 || m.SDKMajor != 2 || m.SDKMinor != pluginsdk.SDKMinor || m.PluginID != "pushover" || m.Version != "1.0.0" ||
		m.CapabilityID != "pushover" || m.Capability != pluginsdk.KindDelivery ||
		m.Artifact.Path != "pushover/1.0.0/pushover" || m.Artifact.Digest != "@DIGEST@" {
		t.Errorf("identity %+v", m)
	}
	if len(m.Schema.Properties) != 8 || len(m.Policy.Properties) != 8 {
		t.Fatalf("schema %+v policy %+v", m.Schema, m.Policy)
	}
	for name, property := range m.Policy.Properties {
		schema, ok := m.Schema.Properties[name]
		if !ok {
			t.Errorf("policy %s has no schema", name)
		}
		wantUser := name == "userKey" || name == "device"
		if (property.Scope == "user") != wantUser {
			t.Errorf("%s scope %s", name, property.Scope)
		}
		if property.Scope == "user" && !schema.Optional {
			t.Errorf("user property %s must be optional", name)
		}
		if wantSecret := name == "apiToken" || name == "userKey"; property.Secret != wantSecret {
			t.Errorf("%s secret %v", name, property.Secret)
		}
		if property.Secret && schema.Kind != "string" {
			t.Errorf("secret %s is not a string", name)
		}
	}
	if len(m.Delivery.Inputs) != 5 || strings.Join(m.Delivery.OutputKinds, ",") != "text" ||
		m.Delivery.SendsPerMinute != 60 || m.Delivery.Idempotent {
		t.Errorf("delivery %+v", m.Delivery)
	}
}
