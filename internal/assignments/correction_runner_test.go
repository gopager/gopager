package assignments

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

type runnerTransaction struct{ ctx context.Context }

// Context returns the fixture context, not a resolvable primary capability.
func (tx runnerTransaction) Context() context.Context { return tx.ctx }

// runnerFixture fails the first failures transactions with err and otherwise
// offers one correction scope that completes without items.
type runnerFixture struct {
	mu        sync.Mutex
	failures  int
	err       error
	calls     int
	pending   bool
	completed int
}

func (f *runnerFixture) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	f.mu.Lock()
	f.calls++
	failing := f.failures > 0
	if failing {
		f.failures--
	}
	f.mu.Unlock()
	if failing {
		return f.err
	}
	return callback(runnerTransaction{ctx: ctx})
}

func (f *runnerFixture) snapshot() (calls, completed int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.completed
}

func (f *runnerFixture) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return time.Now(), nil
}

func (f *runnerFixture) MatchTargets(context.Context, platform.TransactionContext, uuid.UUID, protocols.Protocol,
	time.Time, []protocols.Target) (MatchResult, error) {
	return MatchResult{}, errors.New("unexpected match")
}

func (f *runnerFixture) ClaimCorrection(context.Context, platform.TransactionContext, time.Time, time.Duration) (CorrectionLease, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return CorrectionLease{}, f.pending, nil
}

func (f *runnerFixture) ReadCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, int) ([]CorrectionBatchItem, error) {
	return nil, nil
}

func (f *runnerFixture) ApplyCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, []CorrectionUpdate) (CorrectionApplied, error) {
	return CorrectionApplied{}, errors.New("unexpected apply")
}

func (f *runnerFixture) CompleteCorrection(context.Context, platform.TransactionContext, CorrectionLease) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = false
	f.completed++
	return true, nil
}

func (f *runnerFixture) FailCorrection(context.Context, platform.TransactionContext, CorrectionLease, string, time.Duration) (bool, error) {
	return false, nil
}

func (f *runnerFixture) AppendCorrectionFact(context.Context, platform.TransactionContext, events.CorrectionFact) (uuid.UUID, error) {
	return uuid.Nil, errors.New("unexpected fact")
}

func (f *runnerFixture) ReadmitProcessing(context.Context, platform.TransactionContext, uuid.UUID) error {
	return errors.New("unexpected readmission")
}

func newFixtureRunner(t *testing.T, f *runnerFixture) *CorrectionRunner {
	t.Helper()
	runner, err := NewCorrectionRunner(f, f, f, f, f, f)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestCorrectionRunnerSurvivesTransientFailures(t *testing.T) {
	t.Parallel()
	for _, code := range []platform.ErrorCode{platform.CodeStorageUnavailable,
		platform.CodeAuthorityUnavailable, platform.CodeCommitUncertain} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				f := &runnerFixture{failures: 3, pending: true,
					err: platform.NewFailure(code, errors.New("database restarting"))}
				runner := newFixtureRunner(t, f)
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan error, 1)
				go func() { done <- runner.Run(ctx) }()
				synctest.Wait()
				if calls, _ := f.snapshot(); calls != 1 {
					t.Fatalf("attempts=%d want 1", calls)
				}
				// Backoff doubles from the poll interval: 500ms, 1s, 2s.
				time.Sleep(correctionPoll)
				synctest.Wait()
				if calls, _ := f.snapshot(); calls != 2 {
					t.Fatalf("attempts=%d want 2", calls)
				}
				time.Sleep(2*correctionPoll - time.Nanosecond)
				synctest.Wait()
				if calls, _ := f.snapshot(); calls != 2 {
					t.Fatalf("second backoff did not double: attempts=%d", calls)
				}
				time.Sleep(time.Nanosecond + 4*correctionPoll)
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("runner stopped on transient failure: %v", err)
				default:
				}
				if _, completed := f.snapshot(); completed != 1 {
					t.Fatalf("correction not processed after recovery: completed=%d", completed)
				}
				cancel()
				synctest.Wait()
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation result=%v", err)
				}
			})
		})
	}
}

func TestCorrectionRunnerBackoffBoundedAndCancellable(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := &runnerFixture{failures: 1 << 20,
			err: platform.NewFailure(platform.CodeStorageUnavailable, errors.New("down"))}
		runner := newFixtureRunner(t, f)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- runner.Run(ctx) }()
		time.Sleep(time.Hour)
		synctest.Wait()
		// Backoff caps at 30 s: roughly 120 attempts per hour.
		if calls, _ := f.snapshot(); calls < 110 || calls > 130 {
			t.Fatalf("backoff not bounded: %d attempts in an hour", calls)
		}
		start := time.Now()
		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation result=%v", err)
			}
		default:
			t.Fatal("runner did not stop on cancellation during backoff")
		}
		if time.Since(start) != 0 {
			t.Fatal("cancellation waited for backoff")
		}
	})
}

func TestCorrectionRunnerReturnsNonTransientFailure(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		failure := platform.NewFailure(platform.CodeInternalError, errors.New("invariant broken"))
		f := &runnerFixture{failures: 1, err: failure}
		runner := newFixtureRunner(t, f)
		if err := runner.Run(t.Context()); !errors.Is(err, failure) {
			t.Fatalf("non-transient failure hidden: %v", err)
		}
	})
}
