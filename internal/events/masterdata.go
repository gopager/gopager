package events

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// MasterDataFact is the producer input for a configuration.changed fact emitted
// by master-data management when a unit, unit collection, organization or unit
// template is created, updated, deleted, has its membership or defaults
// changed, or is bulk-updated. The Attribute schema service also records it with
// aggregate type attributeSchema when the Agencies of a unit schema change.
type MasterDataFact struct {
	ID            uuid.UUID
	Aggregate     EntityRef
	Revision      int64
	OccurredAt    time.Time
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
	ChangeKind    string
}

// MasterDataData is the data payload of a master-data envelope.
type MasterDataData struct {
	ChangeKind string `json:"changeKind"`
}

// MasterDataEnvelope is the persisted serialization of a MasterDataFact,
// classified "configuration".
type MasterDataEnvelope struct {
	SchemaVersion     int            `json:"schemaVersion"`
	DomainEventID     uuid.UUID      `json:"domainEventId"`
	Type              Type           `json:"type"`
	Producer          Producer       `json:"producer"`
	AggregateRef      EntityRef      `json:"aggregateRef"`
	AggregateRevision int64          `json:"aggregateRevision,string"`
	OccurredAt        time.Time      `json:"occurredAt"`
	RecordedAt        time.Time      `json:"recordedAt"`
	CausationID       *uuid.UUID     `json:"causationId,omitempty"`
	CorrelationID     *uuid.UUID     `json:"correlationId,omitempty"`
	Data              MasterDataData `json:"data"`
	Classification    string         `json:"classification"`
	ReplayMode        string         `json:"replayMode"`
}

// MasterDataAppender records master-data facts through the caller's active
// platform transaction. AppendMasterDataFact returns the fact ID, or
// ErrInvalidFact when the fact fails Envelope validation.
type MasterDataAppender interface {
	AppendMasterDataFact(context.Context, platform.TransactionContext, MasterDataFact) (uuid.UUID, error)
}

// Envelope validates the aggregate type and change kind and returns the
// configuration.changed envelope stamped with recordedAt. It returns
// ErrInvalidFact when validation fails.
func (fact MasterDataFact) Envelope(recordedAt time.Time) (MasterDataEnvelope, error) {
	if fact.ID == uuid.Nil || fact.Aggregate.ID == uuid.Nil || fact.Revision <= 0 ||
		!slices.Contains([]string{"unit", "unitCollection", "organization", "unitTemplate", "agency", "attributeSchema"}, fact.Aggregate.Type) ||
		!slices.Contains([]string{"created", "updated", "deleted", "membership-changed", "defaults-changed", "bulk-updated"}, fact.ChangeKind) ||
		fact.OccurredAt.IsZero() || recordedAt.IsZero() ||
		fact.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(fact.CausationID != nil && *fact.CausationID == uuid.Nil) ||
		(fact.CorrelationID != nil && *fact.CorrelationID == uuid.Nil) {
		return MasterDataEnvelope{}, ErrInvalidFact
	}
	return MasterDataEnvelope{SchemaVersion: 1, DomainEventID: fact.ID, Type: ConfigurationChanged,
		Producer:     Producer{Domain: "masterdata", Version: "master-data-management1.0"},
		AggregateRef: fact.Aggregate, AggregateRevision: fact.Revision,
		OccurredAt: fact.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: fact.CausationID, CorrelationID: fact.CorrelationID,
		Data: MasterDataData{ChangeKind: fact.ChangeKind}, Classification: "configuration", ReplayMode: "normal"}, nil
}
