package events

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

type IntakeType string

const (
	SubmissionRejected         IntakeType = "operation.submission-rejected"
	ObservationTimeImplausible IntakeType = "operation.observation-time-implausible"
	NetworkAmbiguous           IntakeType = "operation.network-ambiguous"
	GatewayStateChanged        IntakeType = "gateway.state-changed"
	MessageCreated             IntakeType = "message.created"
	MessageTransmissionJoined  IntakeType = "message.transmission-joined"
	MessageMerged              IntakeType = "message.merged"
	AssignmentChanged          IntakeType = "assignment.changed"
)

type IntakeResolution struct {
	Status        string     `json:"status"`
	UnitID        *uuid.UUID `json:"unitId,omitempty"`
	AddressRuleID *uuid.UUID `json:"matchedRuleId,omitempty"`
}

type IntakeData struct {
	GatewayID      uuid.UUID         `json:"gatewayId,omitempty"`
	TransmissionID uuid.UUID         `json:"transmissionId,omitempty"`
	MessageID      uuid.UUID         `json:"messageId,omitempty"`
	NetworkID      uuid.UUID         `json:"networkId,omitempty"`
	SubmissionID   string            `json:"submissionId,omitempty"`
	Resolution     string            `json:"resolution,omitempty"`
	ContactTime    *time.Time        `json:"contactTime,omitempty"`
	OldState       string            `json:"oldState,omitempty"`
	NewState       string            `json:"newState,omitempty"`
	ReasonCode     string            `json:"reasonCode,omitempty"`
	Detail         string            `json:"detail,omitempty"`
	Reason         string            `json:"reason,omitempty"`
	LateJoin       bool              `json:"lateJoin,omitempty"`
	Absorbed       []uuid.UUID       `json:"absorbedMessageIds,omitempty"`
	TargetPosition *int              `json:"targetPosition,omitempty"`
	Before         *IntakeResolution `json:"before,omitempty"`
	After          *IntakeResolution `json:"after,omitempty"`
}

type IntakeFact struct {
	ID             uuid.UUID
	Type           IntakeType
	GatewayID      uuid.UUID
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	SubmissionID   string
	Resolution     string
	Revision       int64
	OccurredAt     time.Time
	ContactTime    *time.Time
	OldState       string
	NewState       string
	ReasonCode     string
	Detail         string
	Reason         string
	LateJoin       bool
	Absorbed       []uuid.UUID
	TargetPosition *int
	Before         *IntakeResolution
	After          *IntakeResolution
}

type IntakeAppender interface {
	AppendIntakeFact(context.Context, platform.TransactionContext, IntakeFact) (uuid.UUID, error)
}

type IntakeEnvelope struct {
	SchemaVersion     int        `json:"schemaVersion"`
	DomainEventID     uuid.UUID  `json:"domainEventId"`
	Type              IntakeType `json:"type"`
	Producer          Producer   `json:"producer"`
	AggregateRef      EntityRef  `json:"aggregateRef"`
	AggregateRevision int64      `json:"aggregateRevision,string"`
	OccurredAt        time.Time  `json:"occurredAt"`
	RecordedAt        time.Time  `json:"recordedAt"`
	Data              IntakeData `json:"data"`
	Classification    string     `json:"classification"`
	ReplayMode        string     `json:"replayMode"`
}

// Envelope validates the AP-07 fact vocabulary and excludes original Payload and credential data.
func (f IntakeFact) Envelope(recorded time.Time) (IntakeEnvelope, error) {
	if f.ID == uuid.Nil || f.Revision <= 0 || f.OccurredAt.IsZero() || recorded.IsZero() ||
		f.OccurredAt.Nanosecond()%1000 != 0 || recorded.Nanosecond()%1000 != 0 {
		return IntakeEnvelope{}, ErrInvalidFact
	}
	envelope := IntakeEnvelope{SchemaVersion: 1, DomainEventID: f.ID, Type: f.Type,
		OccurredAt: f.OccurredAt.UTC(), RecordedAt: recorded.UTC(), Classification: "operational", ReplayMode: "normal"}
	switch f.Type {
	case SubmissionRejected:
		if f.GatewayID == uuid.Nil || f.SubmissionID == "" || f.ReasonCode == "" {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "DOM-01", Version: "transmission-intake1.0"}
		envelope.AggregateRef = EntityRef{Type: "gateway", ID: f.GatewayID}
		envelope.Data = IntakeData{GatewayID: f.GatewayID, SubmissionID: f.SubmissionID, ReasonCode: f.ReasonCode}
	case ObservationTimeImplausible, NetworkAmbiguous:
		if f.TransmissionID == uuid.Nil || f.GatewayID == uuid.Nil ||
			f.Type == NetworkAmbiguous && (f.NetworkID == uuid.Nil || f.Resolution == "") {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "DOM-01", Version: "transmission-intake1.0"}
		envelope.AggregateRef = EntityRef{Type: "transmission", ID: f.TransmissionID}
		envelope.Data = IntakeData{GatewayID: f.GatewayID, TransmissionID: f.TransmissionID,
			NetworkID: f.NetworkID, Resolution: f.Resolution}
	case GatewayStateChanged:
		if f.GatewayID == uuid.Nil || f.OldState == "" || f.NewState == "" ||
			f.OldState == f.NewState || f.ContactTime == nil || f.ContactTime.IsZero() {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "DOM-01", Version: "transmission-intake1.0"}
		envelope.AggregateRef = EntityRef{Type: "gateway", ID: f.GatewayID}
		envelope.Data = IntakeData{ContactTime: f.ContactTime, OldState: f.OldState, NewState: f.NewState,
			ReasonCode: f.ReasonCode, Detail: f.Detail}
	case MessageCreated, MessageTransmissionJoined:
		if f.MessageID == uuid.Nil || f.TransmissionID == uuid.Nil {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "DOM-02", Version: "message-grouping1.0"}
		envelope.AggregateRef = EntityRef{Type: "message", ID: f.MessageID}
		envelope.Data = IntakeData{MessageID: f.MessageID, TransmissionID: f.TransmissionID,
			LateJoin: f.Type == MessageTransmissionJoined && f.LateJoin}
	case MessageMerged:
		if f.MessageID == uuid.Nil || f.TransmissionID == uuid.Nil || len(f.Absorbed) == 0 {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		absorbed := slices.Clone(f.Absorbed)
		slices.SortFunc(absorbed, func(left, right uuid.UUID) int {
			return stringsCompare(left.String(), right.String())
		})
		if len(slices.Compact(absorbed)) != len(absorbed) || slices.Contains(absorbed, f.MessageID) {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "DOM-02", Version: "message-grouping1.0"}
		envelope.AggregateRef = EntityRef{Type: "message", ID: f.MessageID}
		envelope.Data = IntakeData{MessageID: f.MessageID, TransmissionID: f.TransmissionID, Absorbed: absorbed}
	case AssignmentChanged:
		if f.MessageID == uuid.Nil || f.TransmissionID == uuid.Nil || f.TargetPosition == nil ||
			*f.TargetPosition < 0 || f.After == nil || !validIntakeResolution(*f.After) ||
			(f.Before != nil && !validIntakeResolution(*f.Before)) ||
			!slices.Contains([]string{"created", "corrected", "removed", "first-time-changed"}, f.Reason) {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "DOM-04", Version: "target-admission1.0"}
		envelope.AggregateRef = EntityRef{Type: "message", ID: f.MessageID}
		envelope.Data = IntakeData{MessageID: f.MessageID, TransmissionID: f.TransmissionID,
			Reason: f.Reason, TargetPosition: f.TargetPosition, Before: f.Before, After: f.After}
	default:
		return IntakeEnvelope{}, errors.Join(ErrInvalidFact, errors.New("unknown intake fact type"))
	}
	envelope.AggregateRevision = f.Revision
	return envelope, nil
}

func validIntakeResolution(value IntakeResolution) bool {
	if !slices.Contains([]string{"unresolved", "unassigned", "assigned", "pending", "failed"}, value.Status) {
		return false
	}
	return (value.Status == "assigned") == (value.UnitID != nil) &&
		(value.Status == "assigned") == (value.AddressRuleID != nil)
}

func stringsCompare(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
