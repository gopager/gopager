package processing

import (
	"context"
	"errors"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

// computeAllowance is the accumulated built-in compute allowance of one Workflow evaluation.
// It is a variable only so tests can exercise exhaustion without waiting for the production bound.
var computeAllowance = time.Second

// errComputeBudgetExhausted is the cancellation cause recorded when a run's own compute allowance
// expires, which distinguishes it from cancellation of the caller's context.
var errComputeBudgetExhausted = errors.New("accumulated compute budget exhausted")

// startCompute gives a run one accumulated built-in allowance, separate from installed guest waits.
func (b *executionBudget) startCompute(parent context.Context, allowance time.Duration) {
	b.parent, b.remaining = parent, allowance
	b.resumeCompute()
}

func (b *executionBudget) resumeCompute() {
	b.active, b.cancel = context.WithTimeoutCause(b.parent, b.remaining, errComputeBudgetExhausted)
	b.started = time.Now()
}

func (b *executionBudget) computeContext() context.Context { return b.active }

func (b *executionBudget) closeCompute() {
	if b.cancel != nil {
		b.cancel()
	}
}

// exhausted is the explicit resource failure reported when the compute allowance runs out.
func exhausted() error {
	return resource(errComputeBudgetExhausted)
}

// bound turns a context error caused by this run's own compute allowance into the explicit
// resource failure. Cancellation or expiry of the caller's context stays a cancellation, and
// every already classified failure is returned unchanged.
func (b *executionBudget) bound(err error) error {
	if err == nil || b == nil || b.parent == nil || b.active == nil || b.parent.Err() != nil ||
		platform.FailureCode(err) != platform.CodeInternalError ||
		(!errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)) {
		return err
	}
	if errors.Is(context.Cause(b.active), errComputeBudgetExhausted) {
		return exhausted()
	}
	return err
}

// guestWait pauses the compute allowance while run waits on something that is not built-in
// compute: guest instantiation/execution, or the database transaction that authorizes an installed
// call. run receives the caller's context. Preparation and result checks remain charged.
func (b *executionBudget) guestWait(run func(context.Context) error) error {
	if b.active == nil || b.parent == nil || b.started.IsZero() {
		return invalid(errors.New("invalid accumulated compute budget"))
	}
	if err := b.active.Err(); err != nil {
		return b.bound(err)
	}
	b.remaining -= time.Since(b.started)
	if b.remaining <= 0 {
		return exhausted()
	}
	b.cancel()
	b.started = time.Time{}
	err := run(b.parent)
	b.resumeCompute()
	if parentErr := b.parent.Err(); parentErr != nil {
		return parentErr
	}
	return err
}
