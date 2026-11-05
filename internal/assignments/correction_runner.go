package assignments

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

const (
	correctionBatchSize = 100
	correctionLease     = 30 * time.Second
	correctionPoll      = 500 * time.Millisecond
	correctionMaxRetry  = 30 * time.Second
)

// CorrectionMatcher re-matches stored targets against current rules inside the
// runner's transaction; Service.MatchTargets implements it.
type CorrectionMatcher interface {
	MatchTargets(context.Context, platform.TransactionContext, uuid.UUID, protocols.Protocol,
		time.Time, []protocols.Target) (MatchResult, error)
}

// CorrectionRunner is the background worker that drains pending correction
// scopes and re-enters held-back transmissions into publication. Create it
// with NewCorrectionRunner. Concurrent runners are safe: scopes are claimed
// under a lease with a fencing token.
type CorrectionRunner struct {
	transactions identity.Transactions
	clock        platform.Clock
	matcher      CorrectionMatcher
	repository   CorrectionWorkRepository
	facts        events.CorrectionAppender
	publication  CorrectionPublication
}

// NewCorrectionRunner returns a runner using the given ports. It returns
// authz.ErrInvalidInput if any dependency is nil.
func NewCorrectionRunner(transactions identity.Transactions, clock platform.Clock, matcher CorrectionMatcher,
	repository CorrectionWorkRepository, facts events.CorrectionAppender, publication CorrectionPublication,
) (*CorrectionRunner, error) {
	if transactions == nil || clock == nil || matcher == nil || repository == nil || facts == nil ||
		publication == nil {
		return nil, authz.ErrInvalidInput
	}
	return &CorrectionRunner{transactions: transactions, clock: clock, matcher: matcher,
		repository: repository, facts: facts, publication: publication}, nil
}

// Run calls RunOnce every 500ms until ctx is cancelled, returning ctx.Err().
// A transient storage failure from RunOnce (STORAGE_UNAVAILABLE,
// AUTHORITY_UNAVAILABLE, COMMIT_UNCERTAIN or REVISION_CONFLICT) is logged and
// retried after a backoff that starts at 500ms and doubles up to 30s; any
// other RunOnce error is returned.
func (r *CorrectionRunner) Run(ctx context.Context) error {
	failures := 0
	for {
		wait := correctionPoll
		if _, err := r.RunOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !transientFailure(err) {
				return err
			}
			failures++
			wait = correctionRetryDelay(failures)
			slog.Warn("correction runner storage failure; retrying", "code", platform.FailureCode(err),
				"failures", failures, "retryIn", wait)
		} else if failures > 0 {
			slog.Info("correction runner recovered", "failures", failures)
			failures = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// transientFailure reports storage failures that a later attempt can overcome.
func transientFailure(err error) bool {
	switch platform.FailureCode(err) {
	case platform.CodeStorageUnavailable, platform.CodeAuthorityUnavailable,
		platform.CodeCommitUncertain, platform.CodeRevisionConflict:
		return true
	}
	return false
}

// correctionRetryDelay doubles the poll interval per consecutive failure, bounded by correctionMaxRetry.
func correctionRetryDelay(failures int) time.Duration {
	delay := correctionPoll
	for i := 1; i < failures && delay < correctionMaxRetry; i++ {
		delay *= 2
	}
	return min(delay, correctionMaxRetry)
}

// RunOnce claims one due correction scope and processes it in batches of up to
// 100 transmissions, each batch in its own transaction that re-matches the
// targets, applies changed resolutions and appends an assignment.changed
// correction fact per changed target. A batch or completion failure is
// recorded on the scope with exponential backoff and is not returned; RunOnce
// returns an error only when it cannot claim or reschedule. A reschedule
// failure is joined ahead of the batch failure, so FailureCode reports why
// the scope could not be rescheduled. When no scope is
// due it instead re-enters one batch of held-back transmissions into
// publication. The bool reports whether any work was done.
func (r *CorrectionRunner) RunOnce(ctx context.Context) (bool, error) {
	var lease CorrectionLease
	var claimed bool
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := r.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		lease, claimed, err = r.repository.ClaimCorrection(tx.Context(), tx, now, correctionLease)
		return err
	})
	if err != nil {
		return false, err
	}
	if !claimed {
		return r.reenterPublication(ctx), nil
	}
	for {
		var changes []CorrectionChange
		var hasItems bool
		err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			items, err := r.repository.ReadCorrectionBatch(tx.Context(), tx, lease, correctionBatchSize)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				return nil
			}
			hasItems = true
			updates := make([]CorrectionUpdate, len(items))
			for i, item := range items {
				matched, err := r.matcher.MatchTargets(tx.Context(), tx, item.NetworkID, item.Protocol,
					item.ObservedAt, item.Targets)
				if err != nil {
					return err
				}
				if matched.Revision != lease.AssignmentRev || len(matched.Resolutions) != len(item.Targets) {
					return platform.NewFailure(platform.CodeRevisionConflict, errors.New("correction source revision changed"))
				}
				updates[i] = CorrectionUpdate{Item: item, Match: matched}
			}
			changes, err = r.repository.ApplyCorrectionBatch(tx.Context(), tx, lease, updates)
			if err != nil {
				return err
			}
			now, err := r.clock.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			for _, change := range changes {
				position := change.TargetPosition
				if _, err := r.facts.AppendCorrectionFact(tx.Context(), tx, events.CorrectionFact{
					ID: uuid.New(), Type: "assignment.changed",
					Aggregate: events.EntityRef{Type: "message", ID: change.MessageID},
					Revision:  change.MessageRev, OccurredAt: now,
					TargetPosition: &position, TransmissionID: change.TransmissionID,
					MessageID: change.MessageID, NetworkID: change.NetworkID,
					Before: &change.Before, After: &change.After, Reason: "corrected",
				}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			delay := correctionBackoff(lease.Attempts)
			code := string(platform.FailureCode(err))
			if code == "" || code == string(platform.CodeInternalError) {
				code = "CORRECTION_FAILED"
			}
			rescheduleErr := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
				return r.repository.FailCorrection(tx.Context(), tx, lease, code, delay)
			})
			if rescheduleErr != nil {
				return true, errors.Join(rescheduleErr, err)
			}
			return true, nil
		}
		if hasItems {
			continue
		}
		var completed bool
		err = r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			var err error
			completed, err = r.repository.CompleteCorrection(tx.Context(), tx, lease)
			return err
		})
		if err != nil {
			delay := correctionBackoff(lease.Attempts)
			rescheduleErr := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
				return r.repository.FailCorrection(tx.Context(), tx, lease, "CORRECTION_FAILED", delay)
			})
			if rescheduleErr != nil {
				return true, errors.Join(rescheduleErr, err)
			}
			return true, nil
		}
		if completed {
			return true, nil
		}
	}
}

// reenterPublication hands at most one batch of unpublished Transmissions back to publication.
// Each Transmission uses its own short transaction; failures stay eligible for a later pass.
func (r *CorrectionRunner) reenterPublication(ctx context.Context) bool {
	var transmissions []uuid.UUID
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		transmissions, err = r.publication.PublicationReentries(tx.Context(), tx, correctionBatchSize)
		return err
	})
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("correction publication re-entry selection failed", "code", platform.FailureCode(err))
		}
		return false
	}
	progressed := false
	for _, transmission := range transmissions {
		if ctx.Err() != nil {
			return progressed
		}
		var changed bool
		err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			var err error
			changed, err = r.publication.ReenterPublication(tx.Context(), tx, transmission)
			return err
		})
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("correction publication re-entry failed", "transmission", transmission,
					"code", platform.FailureCode(err))
			}
			continue
		}
		progressed = progressed || changed
	}
	return progressed
}

func correctionBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second
	for i := 1; i < attempt && delay < 30*time.Second; i++ {
		delay *= 2
	}
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}

var _ platform.Runner = (*CorrectionRunner)(nil)
