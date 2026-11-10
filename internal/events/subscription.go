package events

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// SubscriptionAggregate is the aggregate type of Subscription configuration facts.
const SubscriptionAggregate = "subscription"

// SubscriptionFact is the producer input for the configuration.changed fact of a Subscription.
// It carries only the Subscription ID, its revision and the kind of change: no owner, event
// selection, filter, target or layout.
type SubscriptionFact struct {
	ID           uuid.UUID
	Subscription uuid.UUID
	Revision     int64
	OccurredAt   time.Time
	// ChangeKind is created, updated, paused or deleted; resuming a paused Subscription is updated.
	ChangeKind    string
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
}

// SubscriptionAppender records Subscription facts through the caller's active transaction.
type SubscriptionAppender interface {
	// AppendSubscriptionFact returns the fact ID, or ErrInvalidFact when the fact fails
	// Envelope validation.
	AppendSubscriptionFact(context.Context, platform.TransactionContext, SubscriptionFact) (uuid.UUID, error)
}

// Envelope validates the fact and returns its configuration.changed envelope stamped with
// recordedAt, attributed to the delivery producer. It returns ErrInvalidFact when validation fails.
func (f SubscriptionFact) Envelope(recordedAt time.Time) (ConfigurationEnvelope, error) {
	if f.ID == uuid.Nil || f.Subscription == uuid.Nil || f.Revision <= 0 ||
		!slices.Contains([]string{"created", "updated", "paused", "deleted"}, f.ChangeKind) ||
		f.OccurredAt.IsZero() || recordedAt.IsZero() ||
		f.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(f.CausationID != nil && *f.CausationID == uuid.Nil) ||
		(f.CorrelationID != nil && *f.CorrelationID == uuid.Nil) {
		return ConfigurationEnvelope{}, ErrInvalidFact
	}
	return ConfigurationEnvelope{SchemaVersion: 1, DomainEventID: f.ID, Type: ConfigurationChanged,
		Producer:     Producer{Domain: "delivery", Version: "subscription1.0"},
		AggregateRef: EntityRef{Type: SubscriptionAggregate, ID: f.Subscription}, AggregateRevision: f.Revision,
		OccurredAt: f.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: f.CausationID, CorrelationID: f.CorrelationID,
		Data: ConfigurationData{ChangeKind: f.ChangeKind}, Classification: "configuration", ReplayMode: ReplayNormal}, nil
}
