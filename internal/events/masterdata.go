package events

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

type MasterDataFact struct {
	ID            uuid.UUID
	Aggregate     EntityRef
	Revision      int64
	OccurredAt    time.Time
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
	ChangeKind    string
}

type MasterDataData struct {
	ChangeKind string `json:"changeKind"`
}

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

type MasterDataAppender interface {
	AppendMasterDataFact(context.Context, platform.TransactionContext, MasterDataFact) (uuid.UUID, error)
}

func (fact MasterDataFact) Envelope(recordedAt time.Time) (MasterDataEnvelope, error) {
	if fact.ID == uuid.Nil || fact.Aggregate.ID == uuid.Nil || fact.Revision <= 0 ||
		!slices.Contains([]string{"unit", "unitCollection", "organization", "unitTemplate"}, fact.Aggregate.Type) ||
		!slices.Contains([]string{"created", "updated", "deleted", "membership-changed", "defaults-changed", "bulk-updated"}, fact.ChangeKind) ||
		fact.OccurredAt.IsZero() || recordedAt.IsZero() ||
		fact.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(fact.CausationID != nil && *fact.CausationID == uuid.Nil) ||
		(fact.CorrelationID != nil && *fact.CorrelationID == uuid.Nil) {
		return MasterDataEnvelope{}, ErrInvalidFact
	}
	return MasterDataEnvelope{SchemaVersion: 1, DomainEventID: fact.ID, Type: ConfigurationChanged,
		Producer:     Producer{Domain: "DOM-03", Version: "master-data-management1.0"},
		AggregateRef: fact.Aggregate, AggregateRevision: fact.Revision,
		OccurredAt: fact.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: fact.CausationID, CorrelationID: fact.CorrelationID,
		Data: MasterDataData{ChangeKind: fact.ChangeKind}, Classification: "configuration", ReplayMode: "normal"}, nil
}
