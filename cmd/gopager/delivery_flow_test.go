package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/delivery"
)

// TestDeliveryEndToEndAcceptsOnceAndDeduplicatesTheFact covers the Core flow: a personal
// Subscription selects message.created, fan-out materializes one Delivery from the authorized
// projection, the runner sends it once to the controlled provider, and handling the fact again
// creates no second Delivery.
func TestDeliveryEndToEndAcceptsOnceAndDeduplicatesTheFact(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	f.subscribe(bearer, map[string]any{"eventTypes": []string{"message.created"},
		"targets": []any{target(instance)}})
	unit := f.unit("Engine 1", nil)
	message := f.message(f.now().Add(-time.Minute), "FIRE ALARM", unit)
	f.messageFact("message.created", message, nil)

	f.fanOut()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].State != delivery.Prepared || listed[0].MessageID == nil || *listed[0].MessageID != message {
		t.Fatalf("after fan-out: %+v", listed)
	}
	f.attempt()
	calls := f.targets.calls()
	if len(calls) != 1 || !strings.Contains(calls[0].Input.Body, "FIRE ALARM") ||
		!strings.Contains(calls[0].Input.Body, "Engine 1") || calls[0].Input.OutputKind != "text" {
		t.Fatalf("invocations: %+v", calls)
	}
	if string(calls[0].Input.Inputs["recipient"].Value) != `"user-1"` || calls[0].DeliveryID != listed[0].ID {
		t.Fatalf("invocation inputs: %+v", calls[0])
	}
	listed = f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].State != delivery.Accepted || listed[0].AttemptCount != 1 {
		t.Fatalf("after attempt: %+v", listed)
	}

	// The same fact again: the unique Delivery identity makes it a no-op.
	f.exec(`DELETE FROM consumer_offsets WHERE consumer = $1`, delivery.FanOutConsumerName)
	f.fanOut()
	f.attempt()
	if listed := f.deliveries(bearer, ""); len(listed) != 1 || len(f.targets.calls()) != 1 {
		t.Fatalf("replayed fact duplicated: %d deliveries, %d calls", len(listed), len(f.targets.calls()))
	}
}
