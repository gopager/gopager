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
	// AppendSubscriptionHealthFact records a Subscription health change like
	// AppendSubscriptionFact records a configuration change.
	AppendSubscriptionHealthFact(context.Context, platform.TransactionContext, SubscriptionHealthFact) (uuid.UUID, error)
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

// SubscriptionHealthChanged is the type of the fact that records a Subscription turning invalid
// or valid again. It is not deliverable.
const SubscriptionHealthChanged Type = "subscription.health-changed"

// SubscriptionHealthFact is the producer input for the subscription.health-changed fact. It
// carries the new health, the reason while invalid, and the Subscription revision the change
// was made at; health changes never change the revision.
type SubscriptionHealthFact struct {
	ID           uuid.UUID
	Subscription uuid.UUID
	Revision     int64
	OccurredAt   time.Time
	// Health is valid or invalid.
	Health string
	// Reason is empty exactly when Health is valid.
	Reason        string
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
}

// SubscriptionHealthData is the data payload of a subscription.health-changed envelope.
type SubscriptionHealthData struct {
	Health   string `json:"health"`
	Reason   string `json:"reason,omitempty"`
	Revision int64  `json:"revision,string"`
}

// SubscriptionHealthEnvelope is the persisted serialization of a SubscriptionHealthFact,
// classified "configuration".
type SubscriptionHealthEnvelope struct {
	SchemaVersion     int                    `json:"schemaVersion"`
	DomainEventID     uuid.UUID              `json:"domainEventId"`
	Type              Type                   `json:"type"`
	Producer          Producer               `json:"producer"`
	AggregateRef      EntityRef              `json:"aggregateRef"`
	AggregateRevision int64                  `json:"aggregateRevision,string"`
	OccurredAt        time.Time              `json:"occurredAt"`
	RecordedAt        time.Time              `json:"recordedAt"`
	CausationID       *uuid.UUID             `json:"causationId,omitempty"`
	CorrelationID     *uuid.UUID             `json:"correlationId,omitempty"`
	Data              SubscriptionHealthData `json:"data"`
	Classification    string                 `json:"classification"`
	ReplayMode        string                 `json:"replayMode"`
}

// subscriptionHealthReasons are the reasons an invalid Subscription names.
var subscriptionHealthReasons = []string{"filter-invalid", "field-unavailable", "layout-invalid", "resource-limit"}

// Envelope validates the fact and returns its envelope stamped with recordedAt, attributed to
// the delivery producer. It returns ErrInvalidFact when validation fails.
func (f SubscriptionHealthFact) Envelope(recordedAt time.Time) (SubscriptionHealthEnvelope, error) {
	validState := (f.Health == "valid" && f.Reason == "") ||
		(f.Health == "invalid" && slices.Contains(subscriptionHealthReasons, f.Reason))
	if f.ID == uuid.Nil || f.Subscription == uuid.Nil || f.Revision <= 0 || !validState ||
		f.OccurredAt.IsZero() || recordedAt.IsZero() ||
		f.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(f.CausationID != nil && *f.CausationID == uuid.Nil) ||
		(f.CorrelationID != nil && *f.CorrelationID == uuid.Nil) {
		return SubscriptionHealthEnvelope{}, ErrInvalidFact
	}
	return SubscriptionHealthEnvelope{SchemaVersion: 1, DomainEventID: f.ID, Type: SubscriptionHealthChanged,
		Producer:     Producer{Domain: "delivery", Version: "subscription1.0"},
		AggregateRef: EntityRef{Type: SubscriptionAggregate, ID: f.Subscription}, AggregateRevision: f.Revision,
		OccurredAt: f.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: f.CausationID, CorrelationID: f.CorrelationID,
		Data:           SubscriptionHealthData{Health: f.Health, Reason: f.Reason, Revision: f.Revision},
		Classification: "configuration", ReplayMode: ReplayNormal}, nil
}
