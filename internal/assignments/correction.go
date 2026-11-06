package assignments

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// TargetContext is the stored state of one target position of a transmission:
// its message, Network, current Resolution and observation/acceptance times.
type TargetContext struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	Position       int
	Resolution     Resolution
	ObservedAt     time.Time
	AcceptedAt     time.Time
}

// Suppression excludes one target position of a transmission from assignment.
// Active is false once lifted, and LiftedAt is then set.
type Suppression struct {
	ID             uuid.UUID  `json:"id"`
	TransmissionID uuid.UUID  `json:"transmissionId"`
	TargetPosition int        `json:"targetPosition"`
	Revision       int64      `json:"revision,string"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"createdAt"`
	LiftedAt       *time.Time `json:"liftedAt"`
}

// CorrectionStatus is a pending correction scope queued for one assignment
// revision. State is queued, running, completed, failed or superseded;
// FailureCode is set for failures. Fields tagged json:"-" are internal scope
// and cursor data that are not exposed to API clients.
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

// CorrectionRepository is the persistence port for suppressions and
// correction status used by Service. Reads return authz.ErrNotFound for
// unknown IDs. RetryCorrection returns authz.ErrRevisionConflict unless the
// scope is failed, and otherwise supersedes it with a new queued scope at the
// current assignment revision.
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

// CorrectionLease is a claimed correction scope. FencingToken guards every
// write made under the lease, which expires at LeaseUntil; AssignmentRev and
// AccessRevision are the revisions the correction must still match.
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

// CorrectionBatchItem is one stored transmission within a correction scope,
// with the original targets and observation time needed to re-match it.
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

// CorrectionUpdate pairs a batch item with its fresh match result.
type CorrectionUpdate struct {
	Item  CorrectionBatchItem
	Match MatchResult
}

// CorrectionChange is one target whose stored resolution was changed by
// ApplyCorrectionBatch; MessageRev is the message revision after the change.
type CorrectionChange struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	TargetPosition int
	MessageRev     int64
	Before         events.IntakeResolution
	After          events.IntakeResolution
}

// CorrectionWorkRepository is the persistence port used by CorrectionRunner.
// ClaimCorrection leases the oldest queued (or expired running) scope recorded
// for the current assignment revision; it reports false when there is none.
// ApplyCorrectionBatch and CompleteCorrection must be called with a still-valid
// lease and fail with a revision conflict when the assignment or access
// revision moved. CompleteCorrection returns false while transmissions in the
// scope still need correction. FailCorrection reschedules the scope with a
// delay, marks it failed after repeated attempts, or supersedes it when the
// assignment revision moved; it is a no-op for a lost lease.
type CorrectionWorkRepository interface {
	ClaimCorrection(context.Context, platform.TransactionContext, time.Time, time.Duration) (CorrectionLease, bool, error)
	ReadCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, int) ([]CorrectionBatchItem, error)
	ApplyCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, []CorrectionUpdate) ([]CorrectionChange, error)
	CompleteCorrection(context.Context, platform.TransactionContext, CorrectionLease) (bool, error)
	FailCorrection(context.Context, platform.TransactionContext, CorrectionLease, string, time.Duration) error
}

// ProcessingReadmission re-admits Processing for a Transmission whose target
// resolution a correction changed: it supersedes candidates bound to the old
// Assignment context, admits current ones and refreshes the Transmission's
// processing status in the correction's batch transaction.
type ProcessingReadmission interface {
	ReadmitProcessing(context.Context, platform.TransactionContext, uuid.UUID) error
}

// SuppressionResult is a suppression together with the correction scope its
// change queued. Correction is zero for ReadSuppression.
type SuppressionResult struct {
	Suppression Suppression      `json:"suppression"`
	Correction  CorrectionStatus `json:"correction"`
}
