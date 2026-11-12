package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/pluginsdk/plugintest"
)

// seenRequest is one request the fake receiver got.
type seenRequest struct {
	header http.Header
	body   []byte
}

// fakeReceiver is a local receiver that records every request before it answers.
type fakeReceiver struct {
	*httptest.Server
	mu   sync.Mutex
	seen []seenRequest
}

// newFakeReceiver starts a receiver; answer decides the response of each request.
func newFakeReceiver(t *testing.T, answer func(http.ResponseWriter, *http.Request, seenRequest)) *fakeReceiver {
	t.Helper()
	receiver := &fakeReceiver{}
	receiver.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request := seenRequest{header: r.Header.Clone(), body: body}
		receiver.mu.Lock()
		receiver.seen = append(receiver.seen, request)
		receiver.mu.Unlock()
		answer(w, r, request)
	}))
	t.Cleanup(receiver.Close)
	return receiver
}

// requests returns a copy of the requests seen so far.
func (r *fakeReceiver) requests() []seenRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]seenRequest(nil), r.seen...)
}

// newWebhookHarness runs the real webhook executable through the Core's plugin host.
func newWebhookHarness(t *testing.T) *plugintest.Harness {
	t.Helper()
	return plugintest.New(t, plugintest.Options{
		Executable:           plugintest.BuildPlugin(t, ".", "."),
		ManifestTemplatePath: "manifest.template.json",
	})
}

// statusFromBody answers with the status the notification body names, for example "status:503".
func statusFromBody(w http.ResponseWriter, _ *http.Request, request seenRequest) {
	var payload struct {
		Body string `json:"body"`
	}
	_ = json.Unmarshal(request.body, &payload)
	code, err := strconv.Atoi(strings.TrimPrefix(payload.Body, "status:"))
	if err != nil {
		code = http.StatusBadRequest
	}
	if code == http.StatusFound {
		w.Header().Set("Location", "/elsewhere")
	}
	w.WriteHeader(code)
}

func TestWebhookPostsTheNotificationAsJSONMembers(t *testing.T) {
	t.Parallel()
	receiver := newFakeReceiver(t, func(w http.ResponseWriter, _ *http.Request, _ seenRequest) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := newWebhookHarness(t)
	h.Configure(map[string]any{"url": receiver.URL, "allowPrivateDestinations": true}, nil)
	outcome, err := h.Attempt(plugintest.AttemptInput{OutputKind: "json", Body: `{"disk":"full"}`,
		Inputs: map[string]any{"topic": "ops"}, AttemptNumber: 2})
	if err != nil || outcome.Status != processing.AttemptAccepted || outcome.Code != "accepted" {
		t.Fatalf("%+v %v", outcome, err)
	}
	seen := receiver.requests()
	if len(seen) != 1 || seen[0].header.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected requests %+v", seen)
	}
	var payload map[string]any
	if err := json.Unmarshal(seen[0].body, &payload); err != nil {
		t.Fatal(err)
	}
	body, _ := payload["body"].(map[string]any)
	inputs, _ := payload["inputs"].(map[string]any)
	if payload["instanceId"] != h.InstanceID().String() || payload["outputKind"] != "json" ||
		payload["attemptNumber"] != float64(2) || body["disk"] != "full" || inputs["topic"] != "ops" ||
		payload["deliveryId"] == "" || payload["attemptId"] == "" || payload["causationId"] == "" {
		t.Fatalf("unexpected payload %v", payload)
	}
}

func TestWebhookReceiverStatusesAreMappedByTheCore(t *testing.T) {
	t.Parallel()
	receiver := newFakeReceiver(t, statusFromBody)
	h := newWebhookHarness(t)
	h.Configure(map[string]any{"url": receiver.URL, "allowPrivateDestinations": true}, nil)
	cases := []struct {
		status int
		result string
		code   string
	}{
		{http.StatusNoContent, processing.AttemptAccepted, "accepted"},
		{http.StatusServiceUnavailable, processing.AttemptTransient, "provider-unavailable"},
		{http.StatusInternalServerError, processing.AttemptUncertain, "unexpected-response"},
		{http.StatusFound, processing.AttemptPermanent, "redirect-refused"},
		{http.StatusBadRequest, processing.AttemptPermanent, "rejected"},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			t.Parallel()
			outcome, err := h.Attempt(plugintest.AttemptInput{Body: "status:" + strconv.Itoa(tc.status)})
			if err != nil || outcome.Status != tc.result || outcome.Code != tc.code {
				t.Fatalf("%+v %v", outcome, err)
			}
		})
	}
}

func TestWebhookDeniesTheCloudMetadataAddressByDefault(t *testing.T) {
	t.Parallel()
	h := newWebhookHarness(t)
	h.Configure(map[string]any{"url": "http://169.254.169.254/latest/meta-data"}, nil)
	outcome, err := h.Attempt(plugintest.AttemptInput{Body: "hello"})
	if err != nil || outcome.Status != processing.AttemptPermanent || outcome.Code != "destination-denied" {
		t.Fatalf("%+v %v", outcome, err)
	}
}

func TestWebhookSendsTheStoredBearerTokenAndNeverReturnsIt(t *testing.T) {
	t.Parallel()
	const token = "bearer-plaintext-8f3a"
	receiver := newFakeReceiver(t, func(w http.ResponseWriter, _ *http.Request, request seenRequest) {
		if request.header.Get("Authorization") != "Bearer "+token {
			// A hostile receiver that echoes the credential back as its request ID.
			w.Header().Set("X-Request-Id", request.header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-Request-Id", token)
		w.WriteHeader(http.StatusUnauthorized)
	})
	h := newWebhookHarness(t)
	h.Configure(map[string]any{"url": receiver.URL, "allowPrivateDestinations": true},
		map[string]plugintest.Secret{"bearerToken": plugintest.SecretValue(token)})
	outcome, err := h.Attempt(plugintest.AttemptInput{Body: "hello"})
	if err != nil || outcome.Status != processing.AttemptPermanent || outcome.Code != "rejected" {
		t.Fatalf("%+v %v", outcome, err)
	}
	seen := receiver.requests()
	if len(seen) != 1 || seen[0].header.Get("Authorization") != "Bearer "+token {
		t.Fatalf("receiver saw %+v", seen)
	}
	if strings.Contains(outcome.Code, token) || strings.Contains(string(outcome.Receipt), token) {
		t.Fatalf("the Bearer token leaked into %+v", outcome)
	}
}

// signatureValid recomputes the receiver-side HMAC of a request.
func signatureValid(request seenRequest, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(request.header.Get("X-GoPager-Timestamp") + "."))
	mac.Write(request.body)
	return request.header.Get("X-GoPager-Signature") == "sha256="+hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookSignatureFollowsTheReconfiguredSecret(t *testing.T) {
	t.Parallel()
	receiver := newFakeReceiver(t, func(w http.ResponseWriter, _ *http.Request, _ seenRequest) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := newWebhookHarness(t)
	for _, secret := range []string{"first-signing-key", "second-signing-key"} {
		h.Configure(map[string]any{"url": receiver.URL, "allowPrivateDestinations": true},
			map[string]plugintest.Secret{"hmacSecret": plugintest.SecretValue(secret)})
		if outcome, err := h.Attempt(plugintest.AttemptInput{Body: "signed"}); err != nil ||
			outcome.Status != processing.AttemptAccepted {
			t.Fatalf("%+v %v", outcome, err)
		}
	}
	seen := receiver.requests()
	if len(seen) != 2 || !signatureValid(seen[0], "first-signing-key") || signatureValid(seen[0], "second-signing-key") ||
		!signatureValid(seen[1], "second-signing-key") || signatureValid(seen[1], "first-signing-key") {
		t.Fatalf("signatures do not follow the configured key: %+v", seen)
	}
}

func TestWebhookReadsAFileSecretAtEveryAttempt(t *testing.T) {
	t.Parallel()
	receiver := newFakeReceiver(t, func(w http.ResponseWriter, _ *http.Request, _ seenRequest) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := newWebhookHarness(t)
	h.Configure(map[string]any{"url": receiver.URL, "allowPrivateDestinations": true},
		map[string]plugintest.Secret{"bearerToken": plugintest.SecretFile("webhook/token")})
	for _, content := range []string{"old-token\n", "new-token\n"} {
		h.WriteSecretFile("webhook/token", content)
		if outcome, err := h.Attempt(plugintest.AttemptInput{Body: "x"}); err != nil ||
			outcome.Status != processing.AttemptAccepted {
			t.Fatalf("%+v %v", outcome, err)
		}
	}
	seen := receiver.requests()
	if len(seen) != 2 || seen[0].header.Get("Authorization") != "Bearer old-token" ||
		seen[1].header.Get("Authorization") != "Bearer new-token" {
		t.Fatalf("receiver saw %+v", seen)
	}
}

func TestWebhookAnUnansweredRequestIsNeverAcceptedOrRetried(t *testing.T) {
	t.Parallel()
	cases := map[string]func(release <-chan struct{}) func(http.ResponseWriter, *http.Request, seenRequest){
		"the receiver hangs after reading the request": func(release <-chan struct{}) func(http.ResponseWriter, *http.Request, seenRequest) {
			return func(_ http.ResponseWriter, _ *http.Request, _ seenRequest) { <-release }
		},
		"the receiver closes the connection after reading the request": func(<-chan struct{}) func(http.ResponseWriter, *http.Request, seenRequest) {
			return func(w http.ResponseWriter, _ *http.Request, _ seenRequest) {
				conn, _, _ := w.(http.Hijacker).Hijack()
				_ = conn.Close()
			}
		},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			release := make(chan struct{})
			receiver := newFakeReceiver(t, answer(release))
			t.Cleanup(func() { close(release) })
			h := newWebhookHarness(t)
			h.Configure(map[string]any{"url": receiver.URL, "allowPrivateDestinations": true}, nil)
			outcome, err := h.Attempt(plugintest.AttemptInput{Body: "once", Timeout: 2 * time.Second})
			if err == nil && (outcome.Status != processing.AttemptUncertain || outcome.Code != "response-lost") {
				t.Fatalf("an unanswered request must be uncertain, got %+v", outcome)
			}
			if got := len(receiver.requests()); got != 1 {
				t.Fatalf("the notification was sent %d times", got)
			}
		})
	}
}
