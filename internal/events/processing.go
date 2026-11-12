package events

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ProcessingConfigurationFact is the processing domain's input for a
// configuration.changed fact about a workflow (created, version-saved,
// activated, updated, deleted). It is separate from identity and master-data
// facts.
type ProcessingConfigurationFact struct {
	ID            uuid.UUID
	WorkflowID    uuid.UUID
	Revision      int64
	OccurredAt    time.Time
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
	ChangeKind    string
}

// ProcessingInstanceFact records only nonsecret administrative instance identity and revision.
type ProcessingInstanceFact struct {
	ID         uuid.UUID
	InstanceID uuid.UUID
	Revision   int64
	OccurredAt time.Time
	ChangeKind string
}

// ProcessingInstanceAppender atomically records instance changes without Workflow impersonation.
type ProcessingInstanceAppender interface {
	AppendProcessingInstance(context.Context, platform.TransactionContext, ProcessingInstanceFact) (uuid.UUID, error)
}

// Envelope projects no artifact paths, configuration values or execution-Principal information.
func (f ProcessingInstanceFact) Envelope(recorded time.Time) (ProcessingEnvelope, error) {
	envelope, err := processingFactEnvelope(f.ID, f.InstanceID, f.Revision, f.OccurredAt, recorded, nil, nil)
	if err != nil {
		return ProcessingEnvelope{}, err
	}
	if !slices.Contains([]string{"created", "updated"}, f.ChangeKind) {
		return ProcessingEnvelope{}, processingInvalid(errors.New("invalid instance configuration change"))
	}
	envelope.AggregateRef.Type = "processing-instance"
	envelope.Type = "configuration.changed"
	envelope.Data, err = json.Marshal(struct {
		ChangeKind string `json:"changeKind"`
	}{f.ChangeKind})
	return envelope, err
}

// ProcessingCompletionFact is emitted only by the atomic source-publication participant.
// Persisting a private completed evaluation alone does not authorize appending this fact.
type ProcessingCompletionFact struct {
	ID             uuid.UUID
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	// MessageTime is the Message time (first observation) of MessageID; required.
	MessageTime         time.Time
	WorkflowID          uuid.UUID
	DefinitionVersionID uuid.UUID
	ResultID            uuid.UUID
	ResultRevision      int64
	SourceRevision      int64
	ContextRevision     int64
	OccurredAt          time.Time
	CausationID         *uuid.UUID
	CorrelationID       *uuid.UUID
	Historical          bool
	// Outputs declares the Attribute fields the result may hold, as current at publication.
	Outputs processing.Declarations
	Result  processing.Evaluation
}

// ProcessingRetractionFact announces that a historical Reprocess withdrew published results of one
// Workflow from a Message without publishing a replacement, so readers of the Message must look
// again. It is not a Delivery trigger.
type ProcessingRetractionFact struct {
	ID             uuid.UUID
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	// MessageTime is the Message time (first observation) of MessageID; required.
	MessageTime time.Time
	WorkflowID  uuid.UUID
	// ResultIDs are the withdrawn results; at least one. The first one is the aggregate.
	ResultIDs []uuid.UUID
	// ResultRevision is the revision the withdrawal gave the first result, higher than the
	// revision of its completion fact.
	ResultRevision int64
	OccurredAt     time.Time
}

// NewProcessingRetraction returns the fact of a Reprocess that withdrew the results of one
// Transmission and Workflow without publishing a replacement, in the order retracted lists them.
// Its aggregate continues the first withdrawn result at the revision the retraction gave it.
func NewProcessingRetraction(transmission, message uuid.UUID, messageTime time.Time, workflow uuid.UUID,
	retracted []processing.RetractedResult, now time.Time) ProcessingRetractionFact {
	fact := ProcessingRetractionFact{ID: uuid.New(), TransmissionID: transmission, MessageID: message,
		MessageTime: messageTime, WorkflowID: workflow, ResultIDs: processing.RetractedIDs(retracted), OccurredAt: now}
	if len(retracted) > 0 {
		fact.ResultRevision = retracted[0].Revision
	}
	return fact
}

// Envelope validates the retraction and builds its envelope. The aggregate is the first withdrawn
// result at the revision the withdrawal gave it, like processing.completed; the replay mode is always reprocess, because only a Reprocess retracts.
func (f ProcessingRetractionFact) Envelope(recorded time.Time) (ProcessingEnvelope, error) {
	if f.TransmissionID == uuid.Nil || f.MessageID == uuid.Nil || !validMessageTime(f.MessageTime) ||
		f.WorkflowID == uuid.Nil || len(f.ResultIDs) == 0 || slices.Contains(f.ResultIDs, uuid.Nil) || f.ResultRevision <= 0 {
		return ProcessingEnvelope{}, processingInvalid(errors.New("incomplete Processing retraction fact"))
	}
	envelope, err := processingFactEnvelope(f.ID, f.ResultIDs[0], f.ResultRevision, f.OccurredAt, recorded, nil, nil)
	if err != nil {
		return ProcessingEnvelope{}, err
	}
	envelope.Type, envelope.Classification, envelope.ReplayMode = "processing.retracted", "source-classified", ReplayReprocess
	envelope.AggregateRef.Type = "processingResult"
	envelope.Data, err = json.Marshal(struct {
		TransmissionID uuid.UUID   `json:"transmissionId"`
		MessageID      uuid.UUID   `json:"messageId"`
		MessageTime    time.Time   `json:"messageTime"`
		WorkflowID     uuid.UUID   `json:"workflowId"`
		ResultIDs      []uuid.UUID `json:"resultIds"`
	}{f.TransmissionID, f.MessageID, f.MessageTime.UTC(), f.WorkflowID, f.ResultIDs})
	if err != nil {
		return ProcessingEnvelope{}, processingInvalid(err)
	}
	return envelope, nil
}

// ProcessingEnvelope is the common versioned event envelope used for processing
// facts; it is built only by the typed processing fact Envelope methods.
// Data is generated only by the fact factories, not accepted as arbitrary caller JSON.
type ProcessingEnvelope struct {
	SchemaVersion     int             `json:"schemaVersion"`
	DomainEventID     uuid.UUID       `json:"domainEventId"`
	Type              string          `json:"type"`
	Producer          Producer        `json:"producer"`
	AggregateRef      EntityRef       `json:"aggregateRef"`
	AggregateRevision int64           `json:"aggregateRevision,string"`
	OccurredAt        time.Time       `json:"occurredAt"`
	RecordedAt        time.Time       `json:"recordedAt"`
	CausationID       *uuid.UUID      `json:"causationId,omitempty"`
	CorrelationID     *uuid.UUID      `json:"correlationId,omitempty"`
	Data              json.RawMessage `json:"data"`
	Classification    string          `json:"classification"`
	ReplayMode        string          `json:"replayMode"`
}

// ProcessingAppender writes through the caller's live platform transaction, never its own commit.
type ProcessingAppender interface {
	AppendProcessingConfiguration(context.Context, platform.TransactionContext, ProcessingConfigurationFact) (uuid.UUID, error)
	AppendProcessingCompletion(context.Context, platform.TransactionContext, ProcessingCompletionFact) (uuid.UUID, error)
	AppendProcessingRetraction(context.Context, platform.TransactionContext, ProcessingRetractionFact) (uuid.UUID, error)
}

func processingInvalid(err error) error {
	return platform.NewFailure(platform.CodeInvalidInput, errors.Join(ErrInvalidFact, err))
}

func processingFactEnvelope(id, aggregate uuid.UUID, revision int64, occurred, recorded time.Time, causation, correlation *uuid.UUID) (ProcessingEnvelope, error) {
	if id == uuid.Nil || aggregate == uuid.Nil || revision <= 0 || occurred.IsZero() || recorded.IsZero() ||
		occurred.Nanosecond()%1000 != 0 || recorded.Nanosecond()%1000 != 0 ||
		(causation != nil && *causation == uuid.Nil) || (correlation != nil && *correlation == uuid.Nil) {
		return ProcessingEnvelope{}, processingInvalid(errors.New("invalid Processing fact envelope"))
	}
	if causation != nil {
		id := *causation
		causation = &id
	}
	if correlation != nil {
		id := *correlation
		correlation = &id
	}
	return ProcessingEnvelope{SchemaVersion: 1, DomainEventID: id,
		Producer:     Producer{Domain: "processing", Version: "processing-kernel1.0"},
		AggregateRef: EntityRef{Type: "workflow", ID: aggregate}, AggregateRevision: revision,
		OccurredAt: occurred.UTC(), RecordedAt: recorded.UTC(), CausationID: causation, CorrelationID: correlation,
		Classification: "configuration", ReplayMode: "normal"}, nil
}

// Envelope validates the frozen configuration-change vocabulary and supplies primary recorded time.
func (f ProcessingConfigurationFact) Envelope(recorded time.Time) (ProcessingEnvelope, error) {
	envelope, err := processingFactEnvelope(f.ID, f.WorkflowID, f.Revision, f.OccurredAt, recorded, f.CausationID, f.CorrelationID)
	if err != nil {
		return ProcessingEnvelope{}, err
	}
	if !slices.Contains([]string{"created", "version-saved", "activated", "updated", "deleted"}, f.ChangeKind) {
		return ProcessingEnvelope{}, processingInvalid(errors.New("unknown Processing configuration change"))
	}
	envelope.Type = "configuration.changed"
	envelope.Data, err = json.Marshal(struct {
		ChangeKind string `json:"changeKind"`
	}{f.ChangeKind})
	if err != nil {
		return ProcessingEnvelope{}, processingInvalid(err)
	}
	return envelope, nil
}

// Envelope validates one completed whole-result publication and preserves historical replay mode.
func (f ProcessingCompletionFact) Envelope(recorded time.Time) (ProcessingEnvelope, error) {
	envelope, err := processingFactEnvelope(f.ID, f.ResultID, f.ResultRevision, f.OccurredAt, recorded, f.CausationID, f.CorrelationID)
	if err != nil {
		return ProcessingEnvelope{}, err
	}
	if f.TransmissionID == uuid.Nil || f.MessageID == uuid.Nil || !validMessageTime(f.MessageTime) || f.WorkflowID == uuid.Nil || f.DefinitionVersionID == uuid.Nil ||
		f.ResultID == uuid.Nil || f.SourceRevision <= 0 || f.ContextRevision <= 0 || !f.Result.Selected ||
		(f.Result.Fields == nil) == (len(f.Result.TargetResults) == 0) || f.Outputs == nil {
		return ProcessingEnvelope{}, processingInvalid(errors.New("incomplete Processing publication fact"))
	}
	if err := processing.ValidateEvaluation(f.Result, f.Outputs, processing.EvaluationBinding{
		TransmissionID: f.TransmissionID, SourceRevision: f.SourceRevision,
		ContextRevision: f.ContextRevision, TargetLimit: processing.MaxArguments,
	}); err != nil {
		if platform.FailureCode(err) == platform.CodeInvalidInput {
			return ProcessingEnvelope{}, processingInvalid(err)
		}
		return ProcessingEnvelope{}, err
	}
	envelope.Type, envelope.Classification = "processing.completed", "source-classified"
	envelope.AggregateRef.Type = "processingResult"
	if f.Historical {
		envelope.ReplayMode = ReplayReprocess
	}
	envelope.Data, err = json.Marshal(struct {
		TransmissionID      uuid.UUID             `json:"transmissionId"`
		MessageID           uuid.UUID             `json:"messageId"`
		MessageTime         time.Time             `json:"messageTime"`
		WorkflowID          uuid.UUID             `json:"workflowId"`
		DefinitionVersionID uuid.UUID             `json:"definitionVersionId"`
		ResultID            uuid.UUID             `json:"resultId"`
		ResultRevision      int64                 `json:"resultRevision,string"`
		SourceRevision      int64                 `json:"sourceRevision,string"`
		ContextRevision     int64                 `json:"contextRevision,string"`
		Status              string                `json:"status"`
		Result              processing.Evaluation `json:"result"`
	}{f.TransmissionID, f.MessageID, f.MessageTime.UTC(), f.WorkflowID, f.DefinitionVersionID, f.ResultID, f.ResultRevision,
		f.SourceRevision, f.ContextRevision, "completed", f.Result})
	if err != nil {
		return ProcessingEnvelope{}, processingInvalid(err)
	}
	return envelope, nil
}
