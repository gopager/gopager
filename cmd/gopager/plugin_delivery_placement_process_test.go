package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// recordMessage records a message.created fact for a new Message without running any consumer and
// returns the Message.
func (s *pluginDeliveryScene) recordMessage(text string) uuid.UUID {
	s.t.Helper()
	message := s.message(s.now().Add(-time.Minute), text, s.unit("Engine "+text, nil))
	s.messageFact("message.created", message, nil)
	return message
}

// awaitDelivery waits until a worker process has created the Delivery of a Message and returns it.
func (s *pluginDeliveryScene) awaitDelivery(message uuid.UUID) uuid.UUID {
	s.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if ids := s.deliveryIDs(message); len(ids) == 1 {
			return ids[0]
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.t.Fatal("no worker created the Delivery")
	return uuid.Nil
}

// awaitStatus waits until a Delivery reaches the status rendered by status.
func (s *pluginDeliveryScene) awaitStatus(id uuid.UUID, want string) {
	s.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if s.status(id) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.t.Fatalf("Delivery = %s, want %s", s.status(id), want)
}

// TestDeliveryPlacementAcrossTwoWorkerProcesses starts two real worker processes on one database,
// one with the webhook plugin in its plugin directory and one with an empty directory. The node
// that has the plugin attempts a prepared Delivery and the other leaves it alone. Once only the node
// without the plugin is left, it prepares the next Delivery as dependency-unavailable but still due,
// leaves it to nodes that have the plugin, and once it is overdue claims it, defers it and records
// no attempt.
func TestDeliveryPlacementAcrossTwoWorkerProcesses(t *testing.T) {
	t.Parallel()
	s := newPluginDeliveryScene(t)
	art := provisionFirstParty(t, s.directory, "webhook")
	receiver := newRecordingReceiver(t)
	instance := s.createInstance(art, map[string]any{"url": typedString(receiver.URL),
		"allowPrivateDestinations": typedBoolean(true)}, nil)
	bearer, _ := s.subscriber("payload", "unit")
	s.subscribeTarget(bearer, instance, nil)

	// The scene prepares the first Delivery itself, because which worker fans a fact out is not
	// deterministic.
	served := s.deliverMessage("first")[0]

	withPlugin := startCommand(t, "worker", s.dsn, "", "--event-poll-interval=100ms --processing-artifact-root="+s.directory)
	withoutPlugin := startCommand(t, "worker", s.dsn, "", "--event-poll-interval=100ms --processing-artifact-root="+t.TempDir())
	stoppedWithPlugin := false
	defer func() {
		if !stoppedWithPlugin {
			withPlugin()
		}
		withoutPlugin()
	}()

	s.awaitStatus(served, "accepted/accepted/1")
	if got := len(receiver.requests()); got != 1 {
		t.Fatalf("receiver got %d requests, want exactly 1", got)
	}
	if got := s.attemptStates(served); got != "1:accepted:accepted:1" {
		t.Fatalf("attempts = %s", got)
	}

	withPlugin()
	stoppedWithPlugin = true
	orphaned := s.awaitDelivery(s.recordMessage("second"))
	if got := s.text(`SELECT (next_attempt_at <= now())::text FROM deliveries WHERE id = $1`, orphaned); got != "true" {
		t.Fatalf("a Delivery prepared without the plugin must stay due for nodes that have it: %s", s.status(orphaned))
	}
	prepared := s.text(`SELECT updated_at::text FROM deliveries WHERE id = $1`, orphaned)
	time.Sleep(time.Second)
	if got := s.text(`SELECT updated_at::text FROM deliveries WHERE id = $1`, orphaned); got != prepared {
		t.Fatalf("the node without the plugin claimed a Delivery that is not overdue: %s", s.status(orphaned))
	}
	before := s.now()
	s.overdue(orphaned, delivery.PlacementGrace)
	deadline := time.Now().Add(30 * time.Second)
	for s.text(`SELECT (updated_at > $2 AND next_attempt_at > now())::text FROM deliveries WHERE id = $1`, orphaned, before) != "true" {
		if time.Now().After(deadline) {
			t.Fatalf("the node without the plugin did not defer the overdue Delivery: %s", s.status(orphaned))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := s.status(orphaned); got != "prepared/"+delivery.CodeDependency+"/0" {
		t.Fatalf("overdue Delivery = %s", got)
	}
	if got := s.attemptStates(orphaned); got != "" {
		t.Fatalf("an attempt was recorded: %s", got)
	}
	if got := len(receiver.requests()); got != 1 {
		t.Fatalf("receiver got %d requests, want still 1", got)
	}
}
