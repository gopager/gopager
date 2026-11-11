package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// transmissionOf returns the single Transmission of a fixture Message.
func (f *deliveryFixture) transmissionOf(message uuid.UUID) uuid.UUID {
	f.t.Helper()
	var transmission uuid.UUID
	f.scanOne(&transmission, `SELECT id FROM transmissions WHERE message_id = $1`, message)
	return transmission
}

// absorb moves every Transmission of the absorbed Message into the survivor and removes the
// absorbed Message, as a merge does.
func (f *deliveryFixture) absorb(survivor, absorbed uuid.UUID) {
	f.t.Helper()
	f.exec(`UPDATE transmissions SET message_id = $1 WHERE message_id = $2`, survivor, absorbed)
	f.exec(`DELETE FROM message_units WHERE message_id = $1`, absorbed)
	f.exec(`DELETE FROM messages WHERE id = $1`, absorbed)
}

// scanOne reads one value of a single-row statement.
func (f *deliveryFixture) scanOne(dest any, statement string, args ...any) {
	f.t.Helper()
	(&attemptScene{deliveryFixture: f}).scan([]any{dest}, statement, args...)
}

// TestDeliveryFollowsAMergeBeforeFanOut proves that a fact naming a Message that a merge absorbed
// before fan-out read it is delivered from the surviving Message, found through the fact's
// Transmission, instead of being dropped.
func TestDeliveryFollowsAMergeBeforeFanOut(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	f.subscribe(bearer, map[string]any{"eventTypes": []string{"message.created"}, "targets": []any{target(f.instance(true))}})
	unit := f.unit("Engine 9", nil)
	survivor := f.message(f.now().Add(-2*time.Minute), "SURVIVOR", unit)
	absorbed := f.message(f.now().Add(-time.Minute), "ABSORBED", unit)
	f.messageFact("message.created", absorbed, map[string]any{"transmissionId": f.transmissionOf(absorbed)})
	f.absorb(survivor, absorbed)

	f.fanOut()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].MessageID == nil || *listed[0].MessageID != survivor || listed[0].State != delivery.Prepared {
		t.Fatalf("after fan-out: %+v", listed)
	}
	f.attempt()
	if calls := f.targets.calls(); len(calls) != 1 || !strings.Contains(calls[0].Input.Body, "Engine 9") {
		t.Fatalf("invocations: %+v", calls)
	}
}

// TestDeliveryFollowsAMergeBeforeItStarts proves that a prepared Delivery whose Message is merged
// into another before it starts is still sent after an access change forces a fresh check,
// instead of being cancelled as if the subscriber had lost access.
func TestDeliveryFollowsAMergeBeforeItStarts(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	f.subscribe(bearer, map[string]any{"eventTypes": []string{"message.created"}, "targets": []any{target(f.instance(true))}})
	unit := f.unit("Engine 10", nil)
	survivor := f.message(f.now().Add(-2*time.Minute), "SURVIVOR", unit)
	absorbed := f.message(f.now().Add(-time.Minute), "ABSORBED", unit)
	f.messageFact("message.created", absorbed, map[string]any{"transmissionId": f.transmissionOf(absorbed)})
	f.fanOut()
	f.absorb(survivor, absorbed)
	f.exec(`UPDATE identity_control SET access_revision = access_revision + 1`)

	f.attempt()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].State != delivery.Accepted || len(f.targets.calls()) != 1 {
		t.Fatalf("after attempt: %+v, %d invocations", listed, len(f.targets.calls()))
	}
}
