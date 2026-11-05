package events

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

type CorrectionFact struct {
	ID             uuid.UUID
	Type           string
	Aggregate      EntityRef
	Revision       int64
	OccurredAt     time.Time
	CausationID    *uuid.UUID
	TargetPosition *int
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	UnitID         *uuid.UUID
	Before         *IntakeResolution
	After          *IntakeResolution
	ChangeKind     string
	Reason         string
}

type CorrectionData struct {
	TargetPosition *int              `json:"targetPosition,omitempty"`
	TransmissionID uuid.UUID         `json:"transmissionId,omitempty"`
	MessageID      uuid.UUID         `json:"messageId,omitempty"`
	NetworkID      uuid.UUID         `json:"networkId,omitempty"`
	UnitID         *uuid.UUID        `json:"unitId,omitempty"`
	Before         *IntakeResolution `json:"before,omitempty"`
	After          *IntakeResolution `json:"after,omitempty"`
	ChangeKind     string            `json:"changeKind,omitempty"`
	Reason         string            `json:"reason,omitempty"`
}

type CorrectionEnvelope struct {
	SchemaVersion     int            `json:"schemaVersion"`
	DomainEventID     uuid.UUID      `json:"domainEventId"`
	Type              string         `json:"type"`
	Producer          Producer       `json:"producer"`
	AggregateRef      EntityRef      `json:"aggregateRef"`
	AggregateRevision int64          `json:"aggregateRevision,string"`
	OccurredAt        time.Time      `json:"occurredAt"`
	RecordedAt        time.Time      `json:"recordedAt"`
	CausationID       *uuid.UUID     `json:"causationId,omitempty"`
	Data              CorrectionData `json:"data"`
	Classification    string         `json:"classification"`
	ReplayMode        string         `json:"replayMode"`
}

type CorrectionAppender interface {
	AppendCorrectionFact(context.Context, platform.TransactionContext, CorrectionFact) (uuid.UUID, error)
}

func (fact CorrectionFact) Envelope(recordedAt time.Time) (CorrectionEnvelope, error) {
	if fact.ID == uuid.Nil || fact.Aggregate.ID == uuid.Nil || fact.Revision <= 0 ||
		fact.OccurredAt.IsZero() || recordedAt.IsZero() ||
		fact.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(fact.CausationID != nil && *fact.CausationID == uuid.Nil) {
		return CorrectionEnvelope{}, ErrInvalidFact
	}
	envelope := CorrectionEnvelope{SchemaVersion: 1, DomainEventID: fact.ID, Type: fact.Type,
		Producer:     Producer{Domain: "DOM-04", Version: "historical-correction1.0"},
		AggregateRef: fact.Aggregate, AggregateRevision: fact.Revision,
		OccurredAt: fact.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: fact.CausationID, Classification: "operational", ReplayMode: "normal"}
	switch fact.Type {
	case "assignment.changed":
		if fact.Aggregate.Type != "message" || fact.MessageID != fact.Aggregate.ID ||
			fact.TransmissionID == uuid.Nil || fact.NetworkID == uuid.Nil || fact.TargetPosition == nil ||
			*fact.TargetPosition < 0 || fact.Before == nil || fact.After == nil ||
			!validIntakeResolution(*fact.Before) || !validIntakeResolution(*fact.After) ||
			equalIntakeResolution(fact.Before, fact.After) || fact.Reason != "corrected" {
			return CorrectionEnvelope{}, ErrInvalidFact
		}
		envelope.Data = CorrectionData{TargetPosition: fact.TargetPosition, TransmissionID: fact.TransmissionID,
			MessageID: fact.MessageID, NetworkID: fact.NetworkID, Before: fact.Before, After: fact.After, Reason: fact.Reason}
	case "suppression.changed":
		if fact.Aggregate.Type != "suppression" || fact.TransmissionID == uuid.Nil || fact.NetworkID == uuid.Nil ||
			fact.TargetPosition == nil || *fact.TargetPosition < 0 ||
			!slices.Contains([]string{"created", "lifted"}, fact.ChangeKind) {
			return CorrectionEnvelope{}, ErrInvalidFact
		}
		envelope.Data = CorrectionData{TargetPosition: fact.TargetPosition, TransmissionID: fact.TransmissionID,
			NetworkID: fact.NetworkID, UnitID: fact.UnitID, ChangeKind: fact.ChangeKind}
	default:
		return CorrectionEnvelope{}, errors.Join(ErrInvalidFact, errors.New("unknown correction fact type"))
	}
	return envelope, nil
}

func equalIntakeResolution(left, right *IntakeResolution) bool {
	if left == nil || right == nil || left.Status != right.Status ||
		(left.UnitID == nil) != (right.UnitID == nil) || (left.AddressRuleID == nil) != (right.AddressRuleID == nil) {
		return left == right
	}
	if left.UnitID != nil && *left.UnitID != *right.UnitID {
		return false
	}
	if left.AddressRuleID != nil && *left.AddressRuleID != *right.AddressRuleID {
		return false
	}
	return true
}
