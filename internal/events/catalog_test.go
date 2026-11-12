package events

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCatalogCoversDeliverableTypes(t *testing.T) {
	t.Parallel()
	deliverable := map[string]Projection{
		"message.created": ProjectionMessage, "message.transmission-joined": ProjectionMessage,
		"message.merged": ProjectionMessage, "assignment.changed": ProjectionMessage,
		"suppression.changed": ProjectionMessage, "processing.completed": ProjectionMessage,
		"gateway.state-changed": ProjectionGateway,
	}
	seen := map[string]bool{}
	for _, entry := range Catalog() {
		if seen[entry.Type] {
			t.Fatalf("duplicate catalog entry %q", entry.Type)
		}
		seen[entry.Type] = true
		if entry.SchemaVersion != 1 {
			t.Fatalf("%s: schema version %d", entry.Type, entry.SchemaVersion)
		}
		projection, want := deliverable[entry.Type]
		if entry.Deliverable != want || entry.Projection != projection {
			t.Fatalf("%s: deliverable=%v projection=%q", entry.Type, entry.Deliverable, entry.Projection)
		}
	}
	for _, eventType := range []string{"configuration.changed", "principal.deleted", "correction.failed",
		"operation.network-ambiguous", "subscription.health-changed"} {
		if entry, ok := LookupType(eventType); !ok || entry.Deliverable {
			t.Fatalf("%s must be catalogued and not deliverable", eventType)
		}
	}
	if _, ok := LookupType("message.unknown"); ok {
		t.Fatal("unknown type found")
	}
}

func TestSubscriptionFactEnvelope(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	fact := SubscriptionFact{ID: uuid.New(), Subscription: uuid.New(), Revision: 2, OccurredAt: now, ChangeKind: "paused"}
	envelope, err := fact.Envelope(now)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Type != ConfigurationChanged || envelope.Producer.Domain != "delivery" ||
		envelope.AggregateRef.Type != SubscriptionAggregate || envelope.AggregateRevision != 2 ||
		envelope.Data.ChangeKind != "paused" || envelope.ReplayMode != ReplayNormal ||
		envelope.Classification != "configuration" {
		t.Fatalf("envelope = %+v", envelope)
	}
	for _, broken := range []SubscriptionFact{
		{ID: uuid.New(), Subscription: uuid.New(), Revision: 1, OccurredAt: now, ChangeKind: "resumed"},
		{ID: uuid.New(), Subscription: uuid.Nil, Revision: 1, OccurredAt: now, ChangeKind: "created"},
		{ID: uuid.New(), Subscription: uuid.New(), Revision: 0, OccurredAt: now, ChangeKind: "created"},
	} {
		if _, err := broken.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%+v: err = %v", broken, err)
		}
	}
}

func TestSubscriptionHealthFactEnvelope(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	fact := SubscriptionHealthFact{ID: uuid.New(), Subscription: uuid.New(), Revision: 3, OccurredAt: now,
		Health: "invalid", Reason: "field-unavailable"}
	envelope, err := fact.Envelope(now)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Type != SubscriptionHealthChanged || envelope.Producer.Domain != "delivery" ||
		envelope.AggregateRef.Type != SubscriptionAggregate || envelope.AggregateRevision != 3 ||
		envelope.Data != (SubscriptionHealthData{Health: "invalid", Reason: "field-unavailable", Revision: 3}) {
		t.Fatalf("envelope = %+v", envelope)
	}
	entry, ok := LookupType(string(SubscriptionHealthChanged))
	if !ok || entry.Producer != "delivery" || entry.SchemaVersion != 1 || entry.Deliverable || entry.Projection != ProjectionNone {
		t.Fatalf("catalog entry = %+v", entry)
	}
	valid := SubscriptionHealthFact{ID: uuid.New(), Subscription: uuid.New(), Revision: 1, OccurredAt: now, Health: "valid"}
	if _, err := valid.Envelope(now); err != nil {
		t.Fatal(err)
	}
	for _, broken := range []SubscriptionHealthFact{
		{ID: uuid.New(), Subscription: uuid.New(), Revision: 1, OccurredAt: now, Health: "valid", Reason: "filter-invalid"},
		{ID: uuid.New(), Subscription: uuid.New(), Revision: 1, OccurredAt: now, Health: "invalid"},
		{ID: uuid.New(), Subscription: uuid.New(), Revision: 1, OccurredAt: now, Health: "invalid", Reason: "other"},
		{ID: uuid.New(), Subscription: uuid.New(), Revision: 0, OccurredAt: now, Health: "valid"},
	} {
		if _, err := broken.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%+v: err = %v", broken, err)
		}
	}
}
