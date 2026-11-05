package assignments

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

type TargetContext struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	Position       int
	Resolution     Resolution
	ObservedAt     time.Time
	AcceptedAt     time.Time
}

type Suppression struct {
	ID             uuid.UUID  `json:"id"`
	TransmissionID uuid.UUID  `json:"transmissionId"`
	TargetPosition int        `json:"targetPosition"`
	Revision       int64      `json:"revision,string"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"createdAt"`
	LiftedAt       *time.Time `json:"liftedAt"`
}

type CorrectionStatus struct {
	ID            uuid.UUID   `json:"id"`
	AssignmentRev int64       `json:"assignmentRevision,string"`
	State         string      `json:"state"`
	NetworkCount  int         `json:"networkCount"`
	UnitCount     int         `json:"unitCount"`
	Processed     int         `json:"processed"`
	Remaining     int         `json:"remaining"`
	FailureCode   string      `json:"failureCode,omitempty"`
	Networks      []uuid.UUID `json:"-"`
	UnitID        *uuid.UUID  `json:"-"`
	Reason        string      `json:"-"`
	HighWaterAt   *time.Time  `json:"-"`
	HighWaterID   *uuid.UUID  `json:"-"`
	CursorAt      *time.Time  `json:"-"`
	CursorID      *uuid.UUID  `json:"-"`
	Attempts      int         `json:"-"`
}

type CorrectionRepository interface {
	ReadTargetContext(context.Context, platform.TransactionContext, uuid.UUID, int) (TargetContext, error)
	ReadSuppression(context.Context, platform.TransactionContext, uuid.UUID) (Suppression, error)
	FindActiveSuppression(context.Context, platform.TransactionContext, uuid.UUID, int) (Suppression, bool, error)
	ListSuppressions(context.Context, platform.TransactionContext) ([]Suppression, error)
	CreateSuppression(context.Context, platform.TransactionContext, Suppression, uuid.UUID) error
	LiftSuppression(context.Context, platform.TransactionContext, uuid.UUID, int64, uuid.UUID, time.Time) (Suppression, error)
	ReadCorrection(context.Context, platform.TransactionContext, uuid.UUID) (CorrectionStatus, error)
	CorrectionForRevision(context.Context, platform.TransactionContext, int64) (CorrectionStatus, error)
	RetryCorrection(context.Context, platform.TransactionContext, uuid.UUID) (CorrectionStatus, error)
}

type CorrectionLease struct {
	ID             uuid.UUID
	AssignmentRev  int64
	Networks       []uuid.UUID
	UnitID         *uuid.UUID
	Reason         string
	HighWaterAt    *time.Time
	HighWaterID    *uuid.UUID
	CursorAt       *time.Time
	CursorID       *uuid.UUID
	AccessRevision int64
	FencingToken   int64
	LeaseUntil     time.Time
	Attempts       int
}

type CorrectionBatchItem struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	Protocol       protocols.Protocol
	Targets        []protocols.Target
	ObservedAt     time.Time
	AcceptedAt     time.Time
	SourceRevision int64
	Dirty          bool
}

type CorrectionUpdate struct {
	Item  CorrectionBatchItem
	Match MatchResult
}

type CorrectionChange struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	TargetPosition int
	MessageRev     int64
	Before         events.IntakeResolution
	After          events.IntakeResolution
}

type CorrectionWorkRepository interface {
	ClaimCorrection(context.Context, platform.TransactionContext, time.Time, time.Duration) (CorrectionLease, bool, error)
	ReadCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, int) ([]CorrectionBatchItem, error)
	ApplyCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, []CorrectionUpdate) ([]CorrectionChange, error)
	CompleteCorrection(context.Context, platform.TransactionContext, CorrectionLease) (bool, error)
	FailCorrection(context.Context, platform.TransactionContext, CorrectionLease, string, time.Duration) error
}

// CorrectionPublication re-enters unpublished Transmissions into the C-11 publication barrier
// after correction (C-05 post-correction publication re-entry). Eligibility is derived from durable rows.
type CorrectionPublication interface {
	PublicationReentries(context.Context, platform.TransactionContext, int) ([]uuid.UUID, error)
	// ReenterPublication reports whether it admitted current Processing work or published the Transmission.
	ReenterPublication(context.Context, platform.TransactionContext, uuid.UUID) (bool, error)
}

type SuppressionResult struct {
	Suppression Suppression      `json:"suppression"`
	Correction  CorrectionStatus `json:"correction"`
}
