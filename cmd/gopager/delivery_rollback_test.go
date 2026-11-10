package main

import (
	"context"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
)

// TestDeliveryFanOutBatchRollsBackAsAWhole proves that fan-out commits its Deliveries together with
// the consumer offset: when inserting the Delivery of a second Subscription fails, the first
// Subscription's Delivery is rolled back too and the fact stays unconsumed; once the failure is
// gone the fact is handled again and each Subscription gets exactly one Delivery.
func TestDeliveryFanOutBatchRollsBackAsAWhole(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	first, second := f.instance(true), f.instance(true)
	f.subscribe(bearer, map[string]any{"eventTypes": []string{"message.created"}, "targets": []any{target(first)}})
	failing := f.subscribe(bearer, map[string]any{"eventTypes": []string{"message.created"}, "targets": []any{target(second)}})
	f.exec(`CREATE FUNCTION test_reject_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'injected Delivery insert failure'; END $$`)
	f.exec(`CREATE TRIGGER test_reject_delivery BEFORE INSERT ON deliveries FOR EACH ROW
		WHEN (NEW.subscription_id = '` + failing.String() + `'::uuid) EXECUTE FUNCTION test_reject_delivery()`)
	unit := f.unit("Engine 7", nil)
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "ROLLBACK", unit), nil)

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
	if listed := f.deliveries(bearer, ""); len(listed) != 0 {
		t.Fatalf("a failed batch left %d Deliveries", len(listed))
	}
	if f.fanOutCaughtUp() {
		t.Fatal("the offset advanced past a fact whose batch failed")
	}

	f.exec(`DROP TRIGGER test_reject_delivery ON deliveries`)
	f.fanOut()
	listed := f.deliveries(bearer, "")
	if len(listed) != 2 || listed[0].TargetInstanceID == listed[1].TargetInstanceID {
		t.Fatalf("after recovery: %+v", listed)
	}
}
