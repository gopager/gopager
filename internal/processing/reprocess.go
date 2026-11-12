package processing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// Reprocess request states. A request is pending until a runner claims it, running while a runner
// holds its lease, completed once no Transmission is left to scan and superseded when the
// Workflow changed after the request was made.
const (
	ReprocessPending    = "pending"
	ReprocessRunning    = "running"
	ReprocessCompleted  = "completed"
	ReprocessSuperseded = "superseded"
)

// ReprocessLeaseDuration is how long a runner may hold a Reprocess request before another runner
// may take it over, measured on the primary clock.
const ReprocessLeaseDuration = 30 * time.Second

// ReprocessRequest is the durable state of one explicit historical Reprocess of a Workflow over a
// Message-time range [MessageTimeFrom, MessageTimeTo). The pins (WorkflowRevision and
// DefinitionVersionID) fence the run: a later change of the Workflow supersedes it. CutoffAt is
// the primary-clock time of the request; only Transmissions accepted at or before it are scanned,
// because later ones are admitted at the active version anyway. Cursor is the ID of the last
// Transmission scanned, so a restarted runner resumes after it.
type ReprocessRequest struct {
	ID                  uuid.UUID
	Revision            int64
	WorkflowID          uuid.UUID
	DefinitionVersionID uuid.UUID
	WorkflowRevision    int64
	MessageTimeFrom     time.Time
	MessageTimeTo       time.Time
	CutoffAt            time.Time
	Cursor              *uuid.UUID
	State               string
	Scanned             int64
	Readmitted          int64
	Retracted           int64
	RequestedBy         uuid.UUID
	DueAt               time.Time
	WorkerID            *uuid.UUID
	LeaseUntil          *time.Time
	FailureCode         *string
	// Attempts counts consecutive failed steps and sets the retry backoff.
	Attempts  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReprocessInput is the caller-supplied part of a Reprocess request.
type ReprocessInput struct {
	// VersionID must be the Workflow's active version.
	VersionID uuid.UUID
	// ExpectedRevision is the Workflow revision the caller saw.
	ExpectedRevision int64
	// MessageTimeFrom (inclusive) and MessageTimeTo (exclusive) bound the Message time.
	MessageTimeFrom time.Time
	MessageTimeTo   time.Time
}

// Validate rejects a missing version or revision and a range that is empty or reversed.
func (i ReprocessInput) Validate() error {
	if i.VersionID == uuid.Nil || i.ExpectedRevision <= 0 || i.MessageTimeFrom.IsZero() ||
		i.MessageTimeTo.IsZero() || !i.MessageTimeFrom.Before(i.MessageTimeTo) {
		return invalid(errors.New("invalid Reprocess request"))
	}
	return nil
}

// ReprocessRepository stores and reads Reprocess requests in the caller's transaction. Services
// authorize the Workflow before they call it.
type ReprocessRepository interface {
	// CreateReprocess inserts a pending request.
	CreateReprocess(context.Context, platform.TransactionContext, ReprocessRequest) error
	// ReadReprocess returns the request of a Workflow, or authz.ErrNotFound.
	ReadReprocess(ctx context.Context, tx platform.TransactionContext, workflow, id uuid.UUID) (ReprocessRequest, error)
	// ListReprocesses returns the requests of a Workflow, newest first.
	ListReprocesses(ctx context.Context, tx platform.TransactionContext, workflow uuid.UUID, page PageInput) (Page[ReprocessRequest], error)
}

// ReprocessNotificationChannel is the PostgreSQL notification channel on which a new Reprocess
// request wakes idle runners.
const ReprocessNotificationChannel = "gopager_processing_reprocess"

// WorkflowPins are the current Workflow facts a Reprocess run is fenced by.
type WorkflowPins struct {
	Revision        int64
	ActiveVersionID *uuid.UUID
	// Usable is false for a missing, deleted or disabled Workflow.
	Usable bool
}

// ReprocessWorkRepository is the runner's view of the request table and of the data it scans.
// Every write is fenced by the claim: it fails with a revision conflict once the lease was lost.
type ReprocessWorkRepository interface {
	// ClaimReprocess takes the oldest due pending request, or a running one whose lease expired,
	// without waiting for rows another transaction holds. The bool reports whether one was found.
	ClaimReprocess(ctx context.Context, tx platform.TransactionContext, worker uuid.UUID) (ReprocessRequest, bool, error)
	// ReadWorkflowPins returns the current pins of the request's Workflow.
	ReadWorkflowPins(ctx context.Context, tx platform.TransactionContext, workflow uuid.UUID) (WorkflowPins, error)
	// NextReprocessTransmission returns the first Transmission after the cursor, in ID order, that
	// the request covers; the bool is false when none is left.
	NextReprocessTransmission(ctx context.Context, tx platform.TransactionContext, request ReprocessRequest) (uuid.UUID, bool, error)
	// IsFreshIntake reports whether intake admitted the Transmission's candidate of the Workflow
	// at the request's pinned revision after the request was made. Such work is new, not history.
	IsFreshIntake(ctx context.Context, tx platform.TransactionContext, request ReprocessRequest, transmission uuid.UUID) (bool, error)
	// ApplyReprocessStep marks the Transmission's current candidate of the Workflow with the
	// request, or, without one, retracts the Workflow's published results of the Transmission,
	// and advances the cursor and counts, all in the caller's transaction. With fresh set it only
	// advances the cursor and counts the Transmission as scanned.
	ApplyReprocessStep(ctx context.Context, tx platform.TransactionContext, request ReprocessRequest, transmission uuid.UUID, fresh bool) error
	// FinishReprocess ends the claimed request as completed or superseded.
	FinishReprocess(ctx context.Context, tx platform.TransactionContext, request ReprocessRequest, state string) error
	// RescheduleReprocess releases the claim and makes the request due again at the given time,
	// recording the failure code.
	RescheduleReprocess(ctx context.Context, tx platform.TransactionContext, request ReprocessRequest, due time.Time, failureCode string) error
}
