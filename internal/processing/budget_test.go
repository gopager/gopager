package processing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

func TestWorkflowComputeBudgetAccumulatesWithoutGuestWaitReset(t *testing.T) {
	t.Parallel()
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
	}); platform.FailureCode(err) != platform.CodeResourceLimit || called {
		t.Fatalf("second step obtained fresh allowance: called=%v, %v", called, err)
	}
}

func TestWorkflowComputeBudgetPreservesCallerCancellationDuringGuest(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	budget := &executionBudget{}
	budget.startCompute(ctx, time.Second)
	defer budget.closeCompute()
	err := budget.guestWait(func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(budget.computeContext().Err(), context.Canceled) ||
		platform.FailureCode(budget.bound(err)) == platform.CodeResourceLimit {
		t.Fatalf("guest wait detached caller cancellation: %v", err)
	}
}

func TestWorkflowComputeBudgetExcludesActualElapsedGuestWait(t *testing.T) {
	t.Parallel()
	// The allowance is larger than the margin a loaded machine can steal between resuming and checking.
	const allowance = time.Second
	budget := &executionBudget{}
	budget.startCompute(context.Background(), allowance)
	defer budget.closeCompute()
	if err := budget.guestWait(func(ctx context.Context) error {
		time.Sleep(allowance + 500*time.Millisecond)
		return ctx.Err()
	}); err != nil || budget.computeContext().Err() != nil {
		t.Fatalf("elapsed guest wait canceled resumed host compute: %v", err)
	}
	if budget.remaining <= 0 || budget.remaining > allowance {
		t.Fatalf("guest wait renewed/exhausted host allowance: %s", budget.remaining)
	}
	<-budget.computeContext().Done()
	if !errors.Is(budget.computeContext().Err(), context.DeadlineExceeded) {
		t.Fatalf("resumed host lost its finite allowance: %v", budget.computeContext().Err())
	}
}

func TestWorkflowComputeBudgetExpiryIsExplicitResourceLimit(t *testing.T) {
	t.Parallel()
	budget := &executionBudget{}
	budget.startCompute(context.Background(), time.Nanosecond)
	defer budget.closeCompute()
	<-budget.computeContext().Done()
	err := budget.bound(budget.computeContext().Err())
	if platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("compute budget expiry was not RESOURCE_LIMIT: %v", err)
	}
	if err := budget.guestWait(func(context.Context) error { return nil }); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("guest wait after compute expiry was not RESOURCE_LIMIT: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	parentBudget := &executionBudget{}
	parentBudget.startCompute(ctx, time.Second)
	defer parentBudget.closeCompute()
	cancel()
	err = parentBudget.bound(parentBudget.computeContext().Err())
	if !errors.Is(err, context.Canceled) || platform.FailureCode(err) == platform.CodeResourceLimit {
		t.Fatalf("caller cancellation became a resource bound: %v", err)
	}
}

func TestWorkflowEvaluationComputeExhaustionAndCallerCancellation(t *testing.T) { //nolint:paralleltest // swaps the package variable computeAllowance
	_, source := sourceFixture(t)
	program, err := CompileWorkflow(context.Background(), selectedDefinition(), source, testOutputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	original := computeAllowance
	computeAllowance = 0
	result, err := program.Evaluate(context.Background(), source)
	computeAllowance = original
	if platform.FailureCode(err) != platform.CodeResourceLimit || result.Selected {
		t.Fatalf("compute budget exhaustion was not RESOURCE_LIMIT: %+v, %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = program.Evaluate(ctx, source)
	if !errors.Is(err, context.Canceled) || platform.FailureCode(err) == platform.CodeResourceLimit || result.Selected {
		t.Fatalf("caller cancellation was not reported as cancellation: %+v, %v", result, err)
	}
	if result, err := program.Evaluate(context.Background(), source); err != nil || !result.Selected {
		t.Fatalf("evaluation after restored allowance failed: %+v, %v", result, err)
	}
}

// withoutWallClockBounds lifts the compute and predicate time bounds for a serial test that
// asserts size limits, so CPU contention from other test packages cannot decide its outcome.
func withoutWallClockBounds(t *testing.T) {
	t.Helper()
	allowance, timeout := computeAllowance, predicateTimeout
	computeAllowance, predicateTimeout = time.Minute, time.Minute
	t.Cleanup(func() { computeAllowance, predicateTimeout = allowance, timeout })
}
