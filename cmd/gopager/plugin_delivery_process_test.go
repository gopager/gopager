package main

import (
	"net"
	"net/http"
	"testing"
)

// TestApiAndWorkerProcessDeliversThroughThePluginHost starts the real command with both roles and
// a plugin directory: the Subscription is saved against the real webhook target over HTTP, the fact
// becomes a Delivery, and the worker's runner has the receiver accept one request.
func TestApiAndWorkerProcessDeliversThroughThePluginHost(t *testing.T) {
	t.Parallel()
	s := newPluginDeliveryScene(t)
	art := provisionFirstParty(t, s.directory, "webhook")
	receiver := newRecordingReceiver(t)
	instance := s.createInstance(art, map[string]any{"url": typedString(receiver.URL),
		"allowPrivateDestinations": typedBoolean(true)}, nil)
	bearer, _ := s.subscriber("payload", "unit")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	stop := startCommand(t, "api,worker", s.dsn, address, "--event-poll-interval=100ms --processing-artifact-root="+s.directory)
	defer stop()

	body := mustJSON(t, subInput(instance, map[string]any{
		"targets": []any{map[string]any{"instanceId": instance, "inputs": map[string]any{}}}}))
	managementRequest(t, http.DefaultClient, "http://"+address, http.MethodPost, "/api/v1/subscriptions", body, "", bearer,
		http.StatusCreated)

	delivery := s.awaitDelivery(s.recordMessage("alarm"))
	s.awaitStatus(delivery, "accepted/accepted/1")
	if got := len(receiver.requests()); got != 1 {
		t.Fatalf("receiver got %d requests, want exactly 1", got)
	}
}
