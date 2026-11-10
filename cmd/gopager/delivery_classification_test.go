package main

import (
	"context"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
)

// TestDeliveryFanOutWaitsForAnUnclassifiedResultVersion proves that a processing.completed fact
// whose published result belongs to a Workflow version that cannot be classified yet is not
// skipped: fan-out keeps the fact unconsumed and creates the Delivery once the classification
// succeeds, instead of treating the result as hidden and losing the notification.
func TestDeliveryFanOutWaitsForAnUnclassifiedResultVersion(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	workflow := f.levelWorkflow()
	f.subCreated(bearer, subInput(f.instance(true), map[string]any{"eventTypes": []string{"processing.completed"}}))
	message := f.message(f.now().Add(-time.Minute), "CLASSIFY", f.unit("Engine 11", nil))
	resultID := f.publishLevel(workflow, message, "3")
	var definition []byte
	(&attemptScene{deliveryFixture: f}).scan([]any{&definition}, `SELECT v.definition FROM processing_workflow_versions v
		JOIN processing_workflows w ON w.active_version_id = v.id WHERE w.id = $1`, workflow)
	f.exec(`UPDATE processing_workflow_versions SET definition = '{"schemaVersion":2}'::jsonb
		WHERE id = (SELECT active_version_id FROM processing_workflows WHERE id = $1)`, workflow)
	f.messageFact("processing.completed", message, map[string]any{"resultId": resultID})

	runtime, err := composeDelivery(f.core)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := events.NewRunner(runtime.fanOut, f.core.facts, f.store, &manualWakes{signal: make(chan struct{}, 1)},
		slogDiscard(), 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if listed := f.deliveries(bearer, ""); len(listed) != 0 || f.fanOutCaughtUp() {
		t.Fatalf("an unclassified result was skipped or delivered: %d Deliveries, caught up %v", len(listed), f.fanOutCaughtUp())
	}

	f.exec(`UPDATE processing_workflow_versions SET definition = $2::jsonb
		WHERE id = (SELECT active_version_id FROM processing_workflows WHERE id = $1)`, workflow, string(definition))
	f.fanOut()
	if listed := f.deliveries(bearer, ""); len(listed) != 1 {
		t.Fatalf("after classification: %d Deliveries", len(listed))
	}
}
