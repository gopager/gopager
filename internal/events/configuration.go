package events

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// ConfigurationFact is the producer input for a configuration.changed fact about
// a unit, unit collection (master-data core) or address rule (assignment
// matcher). It is a separate vocabulary from IdentityFact and MasterDataFact.
type ConfigurationFact struct {
	ID            uuid.UUID
	Aggregate     EntityRef
	Revision      int64
	OccurredAt    time.Time
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
	ChangeKind    string
}

// ConfigurationData is the data payload of a configuration envelope.
type ConfigurationData struct {
	ChangeKind string `json:"changeKind"`
}

// ConfigurationEnvelope is the persisted master-data-facts 1.0 serialization of
// a ConfigurationFact, classified "configuration".
type ConfigurationEnvelope struct {
	SchemaVersion     int               `json:"schemaVersion"`
	DomainEventID     uuid.UUID         `json:"domainEventId"`
	Type              Type              `json:"type"`
	Producer          Producer          `json:"producer"`
	AggregateRef      EntityRef         `json:"aggregateRef"`
	AggregateRevision int64             `json:"aggregateRevision,string"`
	OccurredAt        time.Time         `json:"occurredAt"`
	RecordedAt        time.Time         `json:"recordedAt"`
	CausationID       *uuid.UUID        `json:"causationId,omitempty"`
	CorrelationID     *uuid.UUID        `json:"correlationId,omitempty"`
	Data              ConfigurationData `json:"data"`
	Classification    string            `json:"classification"`
	ReplayMode        string            `json:"replayMode"`
}

// ConfigurationAppender records configuration facts through the caller's active
// platform transaction. AppendConfigurationFact returns the fact ID, or
// ErrInvalidFact when the fact fails Envelope validation.
type ConfigurationAppender interface {
	AppendConfigurationFact(context.Context, platform.TransactionContext, ConfigurationFact) (uuid.UUID, error)
}

// Envelope validates the fact and returns its configuration.changed envelope
// stamped with recordedAt. Aggregate.Type must be unit, unitCollection or
// addressRule and ChangeKind created, updated, deleted or membership-changed;
// address-rule facts are attributed to the matcher producer, all others to
// master-data core. It returns ErrInvalidFact when validation fails.
func (f ConfigurationFact) Envelope(recordedAt time.Time) (ConfigurationEnvelope, error) {
	if f.ID == uuid.Nil || f.Aggregate.ID == uuid.Nil || f.Revision <= 0 ||
		!slices.Contains([]string{"unit", "unitCollection", "addressRule"}, f.Aggregate.Type) ||
		!slices.Contains([]string{"created", "updated", "deleted", "membership-changed"}, f.ChangeKind) ||
		f.OccurredAt.IsZero() || recordedAt.IsZero() ||
		f.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(f.CausationID != nil && *f.CausationID == uuid.Nil) ||
		(f.CorrelationID != nil && *f.CorrelationID == uuid.Nil) {
		return ConfigurationEnvelope{}, ErrInvalidFact
	}
	producer := Producer{Domain: "masterdata", Version: "master-data-core1.0"}
	if f.Aggregate.Type == "addressRule" {
		producer = Producer{Domain: "assignment", Version: "current-matcher1.0"}
	}
	return ConfigurationEnvelope{SchemaVersion: 1, DomainEventID: f.ID, Type: ConfigurationChanged,
		Producer: producer, AggregateRef: f.Aggregate, AggregateRevision: f.Revision,
		OccurredAt: f.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: f.CausationID, CorrelationID: f.CorrelationID,
		Data: ConfigurationData{ChangeKind: f.ChangeKind}, Classification: "configuration", ReplayMode: "normal"}, nil
}
