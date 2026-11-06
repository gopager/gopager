package events

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// IntakeType is the envelope event type of an IntakeFact.
type IntakeType string

// Intake event types accepted by IntakeFact.Envelope.
const (
	// SubmissionRejected records that a gateway submission was rejected
	// (requires GatewayID, SubmissionID, ReasonCode).
	SubmissionRejected IntakeType = "operation.submission-rejected"
	// ObservationTimeImplausible records that a transmission's reported
	// observation time was implausible (requires TransmissionID, GatewayID).
	ObservationTimeImplausible IntakeType = "operation.observation-time-implausible"
	// NetworkAmbiguous records that several of a Gateway's Networks matched a
	// transmission's targets and the highest-priority matching Network was
	// used (also requires NetworkID and Resolution).
	NetworkAmbiguous IntakeType = "operation.network-ambiguous"
	// GatewayStateChanged records a gateway connectivity state transition
	// (requires distinct OldState/NewState and ContactTime).
	GatewayStateChanged IntakeType = "gateway.state-changed"
	// MessageCreated records that a transmission started a new message.
	MessageCreated IntakeType = "message.created"
	// MessageTransmissionJoined records that a transmission joined an existing
	// message; LateJoin marks a transmission observed earlier than the
	// message's latest observation (it arrived out of order).
	MessageTransmissionJoined IntakeType = "message.transmission-joined"
	// MessageMerged records that a message absorbed the messages in Absorbed
	// (non-empty, distinct, not containing MessageID).
	MessageMerged IntakeType = "message.merged"
	// AssignmentChanged records the unit resolution of all targets of one
	// Transmission at admission time (requires Targets in ascending position
	// order and Reason "created"); targetless Transmissions emit none.
	AssignmentChanged IntakeType = "assignment.changed"
)

// IntakeResolution is the assignment state of one message target. Status is
// unresolved, unassigned, assigned, pending or failed; UnitID and
// AddressRuleID must be set exactly when Status is assigned.
type IntakeResolution struct {
	Status        string     `json:"status"`
	UnitID        *uuid.UUID `json:"unitId,omitempty"`
	AddressRuleID *uuid.UUID `json:"matchedRuleId,omitempty"`
}

// IntakeTarget is the resolution of one target position in an assignment.changed
// fact; the resolution fields are inlined in the JSON form.
type IntakeTarget struct {
	Position int `json:"position"`
	IntakeResolution
}

// IntakeData is the data payload of an intake envelope; only the fields
// relevant to the envelope type are set.
type IntakeData struct {
	GatewayID      uuid.UUID      `json:"gatewayId,omitempty"`
	TransmissionID uuid.UUID      `json:"transmissionId,omitempty"`
	MessageID      uuid.UUID      `json:"messageId,omitempty"`
	NetworkID      uuid.UUID      `json:"networkId,omitempty"`
	SubmissionID   string         `json:"submissionId,omitempty"`
	Resolution     string         `json:"resolution,omitempty"`
	ContactTime    *time.Time     `json:"contactTime,omitempty"`
	OldState       string         `json:"oldState,omitempty"`
	NewState       string         `json:"newState,omitempty"`
	ReasonCode     string         `json:"reasonCode,omitempty"`
	Detail         string         `json:"detail,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	LateJoin       bool           `json:"lateJoin,omitempty"`
	Absorbed       []uuid.UUID    `json:"absorbedMessageIds,omitempty"`
	Targets        []IntakeTarget `json:"targets,omitempty"`
}

// IntakeFact is the producer input for an operational fact from transmission
// intake, message grouping or target admission. Type selects which fields are
// required; see the IntakeType constants.
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
	Targets        []IntakeTarget
}

// IntakeAppender records intake facts through the caller's active platform
// transaction. AppendIntakeFact returns the fact ID, or ErrInvalidFact when
// the fact fails Envelope validation.
type IntakeAppender interface {
	AppendIntakeFact(context.Context, platform.TransactionContext, IntakeFact) (uuid.UUID, error)
}

// IntakeEnvelope is the persisted serialization of an IntakeFact, classified
// "operational".
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

// Envelope validates the fact for its Type and returns the envelope stamped
// with recorded, deriving the aggregate and producer from the Type. The
// envelope never carries the original transmission payload or credential
// data. Absorbed IDs are sorted. It returns ErrInvalidFact for missing or
// inconsistent fields or an unknown Type.
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
		envelope.Producer = Producer{Domain: "intake", Version: "transmission-intake1.0"}
		envelope.AggregateRef = EntityRef{Type: "gateway", ID: f.GatewayID}
		envelope.Data = IntakeData{GatewayID: f.GatewayID, SubmissionID: f.SubmissionID, ReasonCode: f.ReasonCode}
	case ObservationTimeImplausible, NetworkAmbiguous:
		if f.TransmissionID == uuid.Nil || f.GatewayID == uuid.Nil ||
			f.Type == NetworkAmbiguous && (f.NetworkID == uuid.Nil || f.Resolution == "") {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "intake", Version: "transmission-intake1.0"}
		envelope.AggregateRef = EntityRef{Type: "transmission", ID: f.TransmissionID}
		envelope.Data = IntakeData{GatewayID: f.GatewayID, TransmissionID: f.TransmissionID,
			NetworkID: f.NetworkID, Resolution: f.Resolution}
	case GatewayStateChanged:
		if f.GatewayID == uuid.Nil || f.OldState == "" || f.NewState == "" ||
			f.OldState == f.NewState || f.ContactTime == nil || f.ContactTime.IsZero() {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "intake", Version: "transmission-intake1.0"}
		envelope.AggregateRef = EntityRef{Type: "gateway", ID: f.GatewayID}
		envelope.Data = IntakeData{ContactTime: f.ContactTime, OldState: f.OldState, NewState: f.NewState,
			ReasonCode: f.ReasonCode, Detail: f.Detail}
	case MessageCreated, MessageTransmissionJoined:
		if f.MessageID == uuid.Nil || f.TransmissionID == uuid.Nil {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "grouping", Version: "message-grouping1.0"}
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
		envelope.Producer = Producer{Domain: "grouping", Version: "message-grouping1.0"}
		envelope.AggregateRef = EntityRef{Type: "message", ID: f.MessageID}
		envelope.Data = IntakeData{MessageID: f.MessageID, TransmissionID: f.TransmissionID, Absorbed: absorbed}
	case AssignmentChanged:
		if f.MessageID == uuid.Nil || f.TransmissionID == uuid.Nil || f.Reason != "created" ||
			!validIntakeTargets(f.Targets) {
			return IntakeEnvelope{}, ErrInvalidFact
		}
		envelope.Producer = Producer{Domain: "assignment", Version: "target-admission1.0"}
		envelope.AggregateRef = EntityRef{Type: "message", ID: f.MessageID}
		envelope.Data = IntakeData{MessageID: f.MessageID, TransmissionID: f.TransmissionID,
			Reason: f.Reason, Targets: slices.Clone(f.Targets)}
	default:
		return IntakeEnvelope{}, errors.Join(ErrInvalidFact, errors.New("unknown intake fact type"))
	}
	envelope.AggregateRevision = f.Revision
	return envelope, nil
}

// validIntakeTargets reports whether targets is non-empty, strictly ascending from a non-negative
// position and every resolution is valid.
func validIntakeTargets(targets []IntakeTarget) bool {
	if len(targets) == 0 {
		return false
	}
	for i, target := range targets {
		if target.Position < 0 || i > 0 && target.Position <= targets[i-1].Position ||
			!validIntakeResolution(target.IntakeResolution) {
			return false
		}
	}
	return true
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
