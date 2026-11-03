package events

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// ConfigurationFact is the narrow DOM-03/DOM-04 producer input, not an IdentityFact.
type ConfigurationFact struct {
	ID            uuid.UUID
	Aggregate     EntityRef
	Revision      int64
	OccurredAt    time.Time
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
	ChangeKind    string
}

type ConfigurationData struct {
	ChangeKind string `json:"changeKind"`
}

// ConfigurationEnvelope is the classified master-data-facts 1.0 serialization.
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

// ConfigurationAppender records configuration facts in the existing platform transaction.
type ConfigurationAppender interface {
	AppendConfigurationFact(context.Context, platform.TransactionContext, ConfigurationFact) (uuid.UUID, error)
}

// Envelope validates the frozen configuration vocabulary and stamps primary recorded time.
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
	producer := Producer{Domain: "DOM-03", Version: "master-data-core1.0"}
	if f.Aggregate.Type == "addressRule" {
		producer = Producer{Domain: "DOM-04", Version: "current-matcher1.0"}
	}
	return ConfigurationEnvelope{SchemaVersion: 1, DomainEventID: f.ID, Type: ConfigurationChanged,
		Producer: producer, AggregateRef: f.Aggregate, AggregateRevision: f.Revision,
		OccurredAt: f.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: f.CausationID, CorrelationID: f.CorrelationID,
		Data: ConfigurationData{ChangeKind: f.ChangeKind}, Classification: "configuration", ReplayMode: "normal"}, nil
}
