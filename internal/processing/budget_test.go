package processing

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWorkflowComputeBudgetAccumulatesWithoutGuestWaitReset(t *testing.T) {
	budget := &executionBudget{}
	budget.startCompute(context.Background(), time.Second)
	defer budget.closeCompute()
	budget.started = time.Now().Add(-600 * time.Millisecond)
	called := false
	if err := budget.guestWait(func(ctx context.Context) error {
		called = true
		return ctx.Err()
	}); err != nil || !called {
		t.Fatalf("first guest was rejected: %v", err)
	}
	if budget.remaining <= 0 || budget.remaining > 400*time.Millisecond {
		t.Fatalf("host compute did not reduce shared allowance: %s", budget.remaining)
	}
	budget.started = time.Now().Add(-450 * time.Millisecond)
	called = false
	if err := budget.guestWait(func(context.Context) error {
		called = true
		return nil
	}); !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("second step obtained fresh allowance: called=%v, %v", called, err)
	}
}

func TestWorkflowComputeBudgetPreservesCallerCancellationDuringGuest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	budget := &executionBudget{}
	budget.startCompute(ctx, time.Second)
	defer budget.closeCompute()
	err := budget.guestWait(func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(budget.computeContext().Err(), context.Canceled) {
		t.Fatalf("guest wait detached caller cancellation: %v", err)
	}
}

func TestWorkflowComputeBudgetExcludesActualElapsedGuestWait(t *testing.T) {
	budget := &executionBudget{}
	budget.startCompute(context.Background(), 200*time.Millisecond)
	defer budget.closeCompute()
	if err := budget.guestWait(func(ctx context.Context) error {
		time.Sleep(1100 * time.Millisecond)
		return ctx.Err()
	}); err != nil || budget.computeContext().Err() != nil {
		t.Fatalf("elapsed guest wait canceled resumed host compute: %v", err)
	}
	if budget.remaining <= 0 || budget.remaining > 200*time.Millisecond {
		t.Fatalf("guest wait renewed/exhausted host allowance: %s", budget.remaining)
	}
	<-budget.computeContext().Done()
	if !errors.Is(budget.computeContext().Err(), context.DeadlineExceeded) {
		t.Fatalf("resumed host lost its finite allowance: %v", budget.computeContext().Err())
	}
}
