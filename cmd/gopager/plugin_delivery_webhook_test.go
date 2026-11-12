package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// webhookScene is a scene with a configured webhook instance and one subscriber subscribed to it.
type webhookScene struct {
	*pluginDeliveryScene
	receiver      *recordingReceiver
	artifact      pluginArtifact
	instance      uuid.UUID
	bearer        string
	configuration map[string]any
}

// newWebhookScene provisions the real webhook plugin, configures an instance with a stored Bearer
// token and HMAC secret against a local receiver and subscribes a subscriber to it.
func newWebhookScene(t *testing.T) *webhookScene {
	t.Helper()
	s := &webhookScene{pluginDeliveryScene: newPluginDeliveryScene(t), receiver: newRecordingReceiver(t)}
	s.artifact = provisionFirstParty(t, s.directory, "webhook")
	s.configuration = map[string]any{"url": typedString(s.receiver.URL), "allowPrivateDestinations": typedBoolean(true)}
	s.instance = s.createInstance(s.artifact, s.configuration,
		map[string]any{"bearerToken": plainSecret("bearer-secret-1"), "hmacSecret": plainSecret("hmac-secret-1")})
	s.bearer, _ = s.subscriber("payload", "unit")
	s.subscribeTarget(s.bearer, s.instance, nil)
	return s
}

// TestWebhookDeliveryReachesTheReceiverWithTheStoredSecrets proves the whole path with the shipped
// webhook plugin: a recorded fact becomes a Delivery, the runner starts the plugin, and the receiver
// gets one signed POST carrying the stored Bearer token, while the instance view never shows a Secret.
func TestWebhookDeliveryReachesTheReceiverWithTheStoredSecrets(t *testing.T) {
	t.Parallel()
	s := newWebhookScene(t)
	receiver, instance := s.receiver, s.instance.String()

	view := string(s.adminRequest(http.MethodGet, "/api/v1/instances/"+instance, "", http.StatusOK).Data)
	for _, leaked := range []string{"bearer-secret-1", "hmac-secret-1", "secret:"} {
		if strings.Contains(view, leaked) {
			t.Fatalf("instance view leaked %q: %s", leaked, view)
		}
	}

	ids := s.deliverMessage("alarm")
	if len(ids) != 1 {
		t.Fatalf("Deliveries = %v", ids)
	}
	s.attempt()
	if got := s.status(ids[0]); got != "accepted/accepted/1" {
		t.Fatalf("Delivery = %s", got)
	}

	requests := receiver.requests()
	if len(requests) != 1 {
		t.Fatalf("receiver got %d requests", len(requests))
	}
	request := requests[0]
	if request.header.Get("Content-Type") != "application/json" || request.header.Get("Authorization") != "Bearer bearer-secret-1" {
		t.Fatalf("headers: %v", request.header)
	}
	timestamp := request.header.Get("X-GoPager-Timestamp")
	if timestamp == "" || request.header.Get("X-GoPager-Signature") != webhookSignature("hmac-secret-1", timestamp, request.body) {
		t.Fatalf("signature does not verify: %v", request.header)
	}
	var payload struct {
		DeliveryID    string          `json:"deliveryId"`
		AttemptNumber int             `json:"attemptNumber"`
		InstanceID    string          `json:"instanceId"`
		OutputKind    string          `json:"outputKind"`
		Body          json.RawMessage `json:"body"`
		Inputs        map[string]any  `json:"inputs"`
	}
	if err := json.Unmarshal(request.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.DeliveryID != ids[0].String() || payload.AttemptNumber != 1 || payload.InstanceID != instance ||
		payload.OutputKind == "" || len(payload.Body) == 0 || payload.Inputs == nil {
		t.Fatalf("payload: %s", request.body)
	}
	if request.header.Get("X-GoPager-Delivery") != ids[0].String() {
		t.Fatalf("delivery header: %v", request.header)
	}
}

// TestWebhookReceiverFailureOutcomes proves that an unexpected 500 leaves the Delivery uncertain
// without any automatic retry, and that a 503 schedules a retry for later.
func TestWebhookReceiverFailureOutcomes(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		status  int
		want    string
		attempt string
		retry   bool
	}{
		"500 is uncertain":           {http.StatusInternalServerError, "uncertain/uncertain/1", "1:uncertain:uncertain:1", false},
		"503 is retried later":       {http.StatusServiceUnavailable, "retry-pending/transient/1", "1:transient:transient:1", true},
		"a rejection is not retried": {http.StatusBadRequest, "failed/permanent/1", "1:permanent:permanent:1", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newWebhookScene(t)
			receiver := s.receiver
			receiver.status.Store(int32(test.status))
			ids := s.deliverMessage("alarm")
			s.attempt()
			s.attempt()
			if got := s.status(ids[0]); got != test.want {
				t.Fatalf("Delivery = %s, want %s", got, test.want)
			}
			if got := s.attemptStates(ids[0]); got != test.attempt {
				t.Fatalf("attempts = %s, want %s", got, test.attempt)
			}
			if got := s.text(`SELECT receipt->>'status' FROM delivery_attempts WHERE delivery_id = $1`, ids[0]); got != strconv.Itoa(test.status) {
				t.Fatalf("receipt status = %s, want %d", got, test.status)
			}
			if got := len(receiver.requests()); got != 1 {
				t.Fatalf("receiver got %d requests, want exactly 1", got)
			}
			later := s.text(`SELECT (next_attempt_at > now())::text FROM deliveries WHERE id = $1`, ids[0])
			if test.retry != (later == "true") {
				t.Fatalf("next attempt in the future = %s, want %t", later, test.retry)
			}
		})
	}
}

// TestWebhookRevokedTargetUseCancelsTheNextStart proves that withdrawing the instance from the
// subscriber through an instance update stops a prepared Delivery before the plugin starts.
func TestWebhookRevokedTargetUseCancelsTheNextStart(t *testing.T) {
	t.Parallel()
	s := newWebhookScene(t)
	ids := s.deliverMessage("alarm")

	s.updateInstance(s.instance, s.artifact, "1", true, s.configuration, map[string]any{"everyone": false, "principalIds": []string{}})
	s.attempt()
	if got := s.status(ids[0]); got != "cancelled/"+delivery.CodeAuthorityRevoked+"/0" {
		t.Fatalf("Delivery = %s", got)
	}
	if got := len(s.receiver.requests()); got != 0 {
		t.Fatalf("receiver got %d requests after the revocation", got)
	}
}
