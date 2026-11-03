package processing

import (
	"context"
	"errors"
	"time"
)

// startCompute gives a run one accumulated built-in allowance, separate from installed guest waits.
func (b *executionBudget) startCompute(parent context.Context, allowance time.Duration) {
	b.parent, b.remaining = parent, allowance
	b.resumeCompute()
}

func (b *executionBudget) resumeCompute() {
	b.active, b.cancel = context.WithTimeout(b.parent, b.remaining)
	b.started = time.Now()
}

func (b *executionBudget) computeContext() context.Context { return b.active }

func (b *executionBudget) closeCompute() {
	if b.cancel != nil {
		b.cancel()
	}
}

// guestWait pauses only guest instantiation/execution; preparation and result checks remain charged.
func (b *executionBudget) guestWait(run func(context.Context) error) error {
	if b.active == nil || b.parent == nil || b.started.IsZero() {
		return invalid(errors.New("invalid accumulated compute budget"))
	}
	if err := b.active.Err(); err != nil {
		return err
	}
	b.remaining -= time.Since(b.started)
	if b.remaining <= 0 {
		return context.DeadlineExceeded
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
