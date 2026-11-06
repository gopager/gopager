package events

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// CorrectionFact is the producer input for a historical-correction fact. Type
// selects the vocabulary: "assignment.changed" records that a correction run
// changed stored target resolutions of one Transmission (Aggregate is the
// message; Targets lists the changed positions in ascending order, each with
// differing Before and After; Reason must be "corrected"),
// "correction.failed" records a correction scope that used up its attempts, and
// "suppression.changed" records that an operator created or lifted a
// suppression (Aggregate is the suppression; ChangeKind is created or lifted).
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
	Targets        []CorrectionTarget
	ChangeKind     string
	Reason         string
	FailureCode    string
}

// CorrectionTarget is one target position whose resolution a correction changed.
type CorrectionTarget struct {
	Position int              `json:"position"`
	Before   IntakeResolution `json:"before"`
	After    IntakeResolution `json:"after"`
}

// CorrectionData is the data payload of a correction envelope; only the fields
// relevant to the envelope type are set.
type CorrectionData struct {
	TargetPosition *int               `json:"targetPosition,omitempty"`
	TransmissionID uuid.UUID          `json:"transmissionId,omitempty"`
	MessageID      uuid.UUID          `json:"messageId,omitempty"`
	NetworkID      uuid.UUID          `json:"networkId,omitempty"`
	UnitID         *uuid.UUID         `json:"unitId,omitempty"`
	Targets        []CorrectionTarget `json:"targets,omitempty"`
	ChangeKind     string             `json:"changeKind,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	FailureCode    string             `json:"failureCode,omitempty"`
}

// CorrectionEnvelope is the persisted serialization of a CorrectionFact,
// classified "operational".
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

// CorrectionAppender records correction facts through the caller's active
// platform transaction. AppendCorrectionFact returns the fact ID, or
// ErrInvalidFact when the fact fails Envelope validation.
type CorrectionAppender interface {
	AppendCorrectionFact(context.Context, platform.TransactionContext, CorrectionFact) (uuid.UUID, error)
}

// Envelope validates the fact for its Type and returns the envelope stamped with
// recordedAt, attributed to the historical-correction producer. It returns
// ErrInvalidFact for missing fields, invalid resolutions, an unchanged
// assignment, or an unknown Type.
func (fact CorrectionFact) Envelope(recordedAt time.Time) (CorrectionEnvelope, error) {
	if fact.ID == uuid.Nil || fact.Aggregate.ID == uuid.Nil || fact.Revision <= 0 ||
		fact.OccurredAt.IsZero() || recordedAt.IsZero() ||
		fact.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(fact.CausationID != nil && *fact.CausationID == uuid.Nil) {
		return CorrectionEnvelope{}, ErrInvalidFact
	}
	envelope := CorrectionEnvelope{SchemaVersion: 1, DomainEventID: fact.ID, Type: fact.Type,
		Producer:     Producer{Domain: "assignment", Version: "historical-correction1.0"},
		AggregateRef: fact.Aggregate, AggregateRevision: fact.Revision,
		OccurredAt: fact.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: fact.CausationID, Classification: "operational", ReplayMode: "normal"}
	switch fact.Type {
	case "assignment.changed":
		if fact.Aggregate.Type != "message" || fact.MessageID != fact.Aggregate.ID ||
			fact.TransmissionID == uuid.Nil || fact.NetworkID == uuid.Nil || fact.Reason != "corrected" ||
			!validCorrectionTargets(fact.Targets) {
			return CorrectionEnvelope{}, ErrInvalidFact
		}
		envelope.Data = CorrectionData{TransmissionID: fact.TransmissionID, MessageID: fact.MessageID,
			NetworkID: fact.NetworkID, Targets: slices.Clone(fact.Targets), Reason: fact.Reason}
	case "suppression.changed":
		if fact.Aggregate.Type != "suppression" || fact.TransmissionID == uuid.Nil || fact.NetworkID == uuid.Nil ||
			fact.TargetPosition == nil || *fact.TargetPosition < 0 ||
			!slices.Contains([]string{"created", "lifted"}, fact.ChangeKind) {
			return CorrectionEnvelope{}, ErrInvalidFact
		}
		envelope.Data = CorrectionData{TargetPosition: fact.TargetPosition, TransmissionID: fact.TransmissionID,
			NetworkID: fact.NetworkID, UnitID: fact.UnitID, ChangeKind: fact.ChangeKind}
	case "correction.failed":
		if fact.Aggregate.Type != "correction" || fact.Aggregate.ID == uuid.Nil || fact.FailureCode == "" ||
			!slices.Contains([]string{"rule-change", "unit-deleted", "suppression-change"}, fact.Reason) {
			return CorrectionEnvelope{}, ErrInvalidFact
		}
		envelope.Data = CorrectionData{Reason: fact.Reason, FailureCode: fact.FailureCode}
	default:
		return CorrectionEnvelope{}, errors.Join(ErrInvalidFact, errors.New("unknown correction fact type"))
	}
	return envelope, nil
}

// validCorrectionTargets reports whether targets is non-empty, strictly ascending from a
// non-negative position and every entry is a valid, actual change.
func validCorrectionTargets(targets []CorrectionTarget) bool {
	if len(targets) == 0 {
		return false
	}
	for i, target := range targets {
		if target.Position < 0 || i > 0 && target.Position <= targets[i-1].Position ||
			!validIntakeResolution(target.Before) || !validIntakeResolution(target.After) ||
			equalIntakeResolution(&target.Before, &target.After) {
			return false
		}
	}
	return true
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
