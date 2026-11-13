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
	// DefaultCorrectionBatchSize is the number of Transmissions one correction transaction moves.
	DefaultCorrectionBatchSize = 100
	correctionLease            = 30 * time.Second
	correctionPoll             = 500 * time.Millisecond
	correctionMaxRetry         = 30 * time.Second
	// correctionScopeRetryBase is the first delay before a failed scope is claimed again.
	correctionScopeRetryBase = time.Second
	retryLimitCode           = "RETRY_LIMIT"
)

// CorrectionMatcher re-matches stored targets against current rules inside the
// runner's transaction; Service.MatchTargets implements it.
type CorrectionMatcher interface {
	MatchTargets(context.Context, platform.TransactionContext, uuid.UUID, protocols.Protocol,
		time.Time, []protocols.Target) (MatchResult, error)
}

// CorrectionRunner is the background worker that drains pending correction
// scopes and re-admits Processing for changed transmissions. Create it
// with NewCorrectionRunner. Concurrent runners are safe: scopes are claimed
// under a lease with a fencing token.
type CorrectionRunner struct {
	transactions identity.Transactions
	clock        platform.Clock
	matcher      CorrectionMatcher
	networks     NetworkResolver
	repository   CorrectionWorkRepository
	facts        events.CorrectionAppender
	readmission  ProcessingReadmission
	batchSize    int
}

// CorrectionRunnerOption adjusts a CorrectionRunner at construction.
type CorrectionRunnerOption func(*CorrectionRunner)

// WithCorrectionBatchSize sets how many Transmissions one correction transaction moves
// (DefaultCorrectionBatchSize when not set). A smaller batch exercises multi-batch scopes with
// little data; a value below 1 is invalid input at construction.
func WithCorrectionBatchSize(size int) CorrectionRunnerOption {
	return func(r *CorrectionRunner) { r.batchSize = size }
}

// NewCorrectionRunner returns a runner using the given ports; networks re-resolves the Network
// of the Transmissions of a gateway scope. It returns authz.ErrInvalidInput if any dependency
// is nil or an option is invalid.
func NewCorrectionRunner(transactions identity.Transactions, clock platform.Clock, matcher CorrectionMatcher,
	networks NetworkResolver, repository CorrectionWorkRepository, facts events.CorrectionAppender,
	readmission ProcessingReadmission, options ...CorrectionRunnerOption,
) (*CorrectionRunner, error) {
	if transactions == nil || clock == nil || matcher == nil || networks == nil || repository == nil ||
		facts == nil || readmission == nil {
		return nil, authz.ErrInvalidInput
	}
	runner := &CorrectionRunner{transactions: transactions, clock: clock, matcher: matcher, networks: networks,
		repository: repository, facts: facts, readmission: readmission, batchSize: DefaultCorrectionBatchSize}
	for _, option := range options {
		option(runner)
	}
	if runner.batchSize < 1 {
		return nil, authz.ErrInvalidInput
	}
	return runner, nil
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
			wait = platform.Backoff(failures, correctionPoll, correctionMaxRetry)
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

// RunOnce claims one due correction scope and processes it in batches of up to
// the runner's batch size (DefaultCorrectionBatchSize unless set), each batch in its own transaction that, for a gateway scope,
// re-resolves each transmission's Network first, re-matches the targets, applies
// changed resolutions and Network moves, appends one assignment.changed correction
// fact per changed or moved transmission and re-admits Processing once for every
// transmission whose targets were restamped, changed resolution or not, or that moved. A batch or completion failure is recorded on the scope
// with exponential backoff and is not returned; RunOnce returns an error only
// when it cannot claim or reschedule. A reschedule failure is joined ahead of
// the batch failure, so FailureCode reports why the scope could not be
// rescheduled. A claim that finds a scope with no attempt left (its fifth lease expired) fails it
// and appends its correction.failed fact in the claim transaction; that counts as work. The bool
// reports whether any work was done.
func (r *CorrectionRunner) RunOnce(ctx context.Context) (bool, error) {
	var lease CorrectionLease
	var claimed bool
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := r.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		lease, claimed, err = r.repository.ClaimCorrection(tx.Context(), tx, now, correctionLease)
		if err != nil || !lease.Exhausted {
			return err
		}
		return r.appendFailed(tx, lease, retryLimitCode)
	})
	if err != nil {
		return false, err
	}
	if lease.Exhausted {
		return true, nil
	}
	if !claimed {
		return false, nil
	}
	for {
		var hasItems bool
		err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			items, err := r.repository.ReadCorrectionBatch(tx.Context(), tx, lease, r.batchSize)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				return nil
			}
			hasItems = true
			updates := make([]CorrectionUpdate, len(items))
			for i, item := range items {
				update, err := r.reresolve(tx, lease, item)
				if err != nil {
					return err
				}
				updates[i] = update
			}
			applied, err := r.repository.ApplyCorrectionBatch(tx.Context(), tx, lease, updates)
			if err != nil {
				return err
			}
			if applied.Unresolved > 0 {
				slog.Warn("Current Gateway mapping and rules no longer resolve accepted Transmissions; they keep their Network",
					"correctionId", lease.ID, "unresolved", applied.Unresolved)
			}
			now, err := r.clock.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			for _, fact := range correctionFacts(applied.Changes, applied.Moves, now) {
				if _, err := r.facts.AppendCorrectionFact(tx.Context(), tx, fact); err != nil {
					return err
				}
			}
			for _, transmission := range applied.Restamped {
				if err := r.readmission.ReadmitProcessing(tx.Context(), tx, transmission); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			code := string(platform.FailureCode(err))
			if code == "" || code == string(platform.CodeInternalError) {
				code = "CORRECTION_FAILED"
			}
			if rescheduleErr := r.fail(ctx, lease, code); rescheduleErr != nil {
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
			if rescheduleErr := r.fail(ctx, lease, "CORRECTION_FAILED"); rescheduleErr != nil {
				return true, errors.Join(rescheduleErr, err)
			}
			return true, nil
		}
		if completed {
			return true, nil
		}
	}
}

// reresolve builds the update of one batch item: for a gateway scope it re-resolves the Network
// first and keeps the stored one when the mapping no longer resolves it, then it matches the
// targets against the rules of that Network.
func (r *CorrectionRunner) reresolve(tx platform.TransactionContext, lease CorrectionLease,
	item CorrectionBatchItem) (CorrectionUpdate, error) {
	update := CorrectionUpdate{Item: item}
	network := item.NetworkID
	if lease.Scope.Kind() == "gateway" {
		resolved, err := r.networks.ResolveNetwork(tx.Context(), tx, item)
		if err != nil {
			return CorrectionUpdate{}, err
		}
		if resolved.Resolved {
			network = resolved.NetworkID
		}
		update.Network = &resolved
	}
	matched, err := r.matcher.MatchTargets(tx.Context(), tx, network, item.Protocol, item.ObservedAt, item.Targets)
	if err != nil {
		return CorrectionUpdate{}, err
	}
	if matched.Revision < lease.AssignmentRev || len(matched.Resolutions) != len(item.Targets) {
		return CorrectionUpdate{}, platform.NewFailure(platform.CodeRevisionConflict,
			errors.New("correction source revision changed"))
	}
	update.Match = matched
	return update, nil
}

// fail reschedules the leased scope with backoff and, once the scope has used up its attempts,
// records the final failure as an operational fact; nothing waits for a failed scope, its
// Messages simply keep their correcting status until a later correction refreshes them.
func (r *CorrectionRunner) fail(ctx context.Context, lease CorrectionLease, code string) error {
	return r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		final, err := r.repository.FailCorrection(tx.Context(), tx, lease, code, platform.Backoff(lease.Attempts, correctionScopeRetryBase, correctionMaxRetry))
		if err != nil || !final {
			return err
		}
		return r.appendFailed(tx, lease, code)
	})
}

// appendFailed appends the final-failure fact in the caller's transaction, so the fact commits with
// the scope's failed state.
func (r *CorrectionRunner) appendFailed(tx platform.TransactionContext, lease CorrectionLease, code string) error {
	now, err := r.clock.Now(tx.Context(), tx)
	if err != nil {
		return err
	}
	_, err = r.facts.AppendCorrectionFact(tx.Context(), tx, events.CorrectionFact{
		ID: uuid.New(), Type: "correction.failed",
		Aggregate: events.EntityRef{Type: "correction", ID: lease.ID},
		Revision:  int64(lease.Attempts), OccurredAt: now, Reason: lease.Reason, FailureCode: code,
	})
	return err
}

var _ platform.Runner = (*CorrectionRunner)(nil)
