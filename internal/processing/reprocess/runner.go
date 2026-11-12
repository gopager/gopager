// Package reprocess runs explicit historical Reprocess requests: it walks the Transmissions a
// request covers and has each one re-judged against the current Workflow, access and targets.
package reprocess

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

const (
	pollInterval = time.Second
	baseBackoff  = time.Second
	maxBackoff   = 30 * time.Second
	// stepFailedCode is recorded on a request whose last step failed without a platform code.
	stepFailedCode = "REPROCESS_FAILED"
)

// Readmission re-admits Processing for one Transmission in the caller's transaction, judging it
// against the current access, targets and Workflow selection without touching its Payload,
// membership or time. The Processing source owner implements it.
type Readmission interface {
	ReadmitProcessing(context.Context, platform.TransactionContext, uuid.UUID) error
}

// Runner is the worker-role runner that drains Reprocess requests. Concurrent runners are safe:
// a request is claimed under a lease and every write is fenced by the claim. Create it with New.
type Runner struct {
	transactions identity.Transactions
	repository   processing.ReprocessWorkRepository
	readmission  Readmission
	clock        platform.Clock
	worker       uuid.UUID
	wake         chan struct{}
}

// New returns a runner; it fails with authz.ErrInvalidInput when a dependency is nil.
func New(transactions identity.Transactions, clock platform.Clock, repository processing.ReprocessWorkRepository,
	readmission Readmission) (*Runner, error) {
	if transactions == nil || clock == nil || repository == nil || readmission == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Runner{transactions: transactions, clock: clock, repository: repository, readmission: readmission,
		worker: uuid.New(), wake: make(chan struct{}, 1)}, nil
}

// Wake tells an idle runner that a request may be due; hints coalesce.
func (r *Runner) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run drains due requests until ctx is cancelled and returns ctx.Err(). It sleeps only when no
// request is due, until a wake-up or the poll interval. A failure to claim is logged and retried.
func (r *Runner) Run(ctx context.Context) error {
	for {
		worked, err := r.RunOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			slog.Warn("reprocess runner failure; retrying", "code", platform.FailureCode(err))
		}
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-r.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// RunOnce claims one due request and drives it until it completes, is superseded or a step fails
// and the request is rescheduled. A step is one Transmission, handled in one transaction together
// with the advance of the request's cursor, so a crash or rollback leaves the request exactly at
// the last committed step. The bool reports whether a request was claimed. A step failure is
// recorded on the request and not returned; RunOnce returns an error only when it cannot claim or
// reschedule.
func (r *Runner) RunOnce(ctx context.Context) (bool, error) {
	var request processing.ReprocessRequest
	var claimed bool
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		request, claimed, err = r.repository.ClaimReprocess(tx.Context(), tx, r.worker)
		return err
	})
	if err != nil || !claimed {
		return false, err
	}
	for {
		var finished bool
		var next uuid.UUID
		err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			var err error
			next, finished, err = r.step(tx, request)
			return err
		})
		if err != nil {
			return true, r.fail(ctx, request, err)
		}
		if finished {
			return true, nil
		}
		request.Cursor, request.Attempts = &next, 0
	}
}

// step handles the next Transmission of the request, or ends the request. It reports finished
// when the request ended, otherwise the Transmission it handled.
func (r *Runner) step(tx platform.TransactionContext, request processing.ReprocessRequest) (uuid.UUID, bool, error) {
	ctx := tx.Context()
	pins, err := r.repository.ReadWorkflowPins(ctx, tx, request.WorkflowID)
	if err != nil {
		return uuid.Nil, false, err
	}
	if !pins.Usable || pins.Revision != request.WorkflowRevision || pins.ActiveVersionID == nil ||
		*pins.ActiveVersionID != request.DefinitionVersionID {
		return uuid.Nil, true, r.repository.FinishReprocess(ctx, tx, request, processing.ReprocessSuperseded)
	}
	transmission, found, err := r.repository.NextReprocessTransmission(ctx, tx, request)
	if err != nil {
		return uuid.Nil, false, err
	}
	if !found {
		return uuid.Nil, true, r.repository.FinishReprocess(ctx, tx, request, processing.ReprocessCompleted)
	}
	// Asked before readmission, which may create candidates of its own: a candidate that intake
	// admitted at the pinned revision after the request is new work, not history to replace.
	snapshot, err := r.repository.SnapshotTransmission(ctx, tx, request, transmission)
	if err != nil {
		return uuid.Nil, false, err
	}
	if err := r.readmission.ReadmitProcessing(ctx, tx, transmission); err != nil {
		return uuid.Nil, false, err
	}
	return transmission, false, r.repository.ApplyReprocessStep(ctx, tx, request, transmission, snapshot)
}

// fail releases the request for a later attempt after a failed step. An open correction on the
// Transmission is a revision conflict and is retried like any other failure, with backoff. A lost
// claim or a cancelled context leaves the request to its new owner or to lease expiry.
func (r *Runner) fail(ctx context.Context, request processing.ReprocessRequest, cause error) error {
	if ctx.Err() != nil {
		return nil
	}
	code := string(platform.FailureCode(cause))
	if code == "" || code == string(platform.CodeInternalError) {
		code = stepFailedCode
	}
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := r.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return r.repository.RescheduleReprocess(tx.Context(), tx, request, now.Add(backoff(request.Attempts+1)), code)
	})
	if errors.Is(err, authz.ErrRevisionConflict) {
		return nil
	}
	return err
}

// backoff doubles from one second per consecutive failed attempt, bounded by maxBackoff.
func backoff(attempt int) time.Duration {
	delay := baseBackoff
	for i := 1; i < attempt && delay < maxBackoff; i++ {
		delay *= 2
	}
	return min(delay, maxBackoff)
}

var _ platform.Runner = (*Runner)(nil)
