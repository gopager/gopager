package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

type dispatchFixture struct {
	mu       sync.Mutex
	items    []processing.Candidate
	active   int
	maximum  int
	starts   []uuid.UUID
	failure  error
	listErr  error
	release  chan struct{}
	exited   int
	requests int
	limits   []int
}

type dispatchTransaction struct{ ctx context.Context }

// Context returns the fixture context, not a resolvable primary capability.
func (tx dispatchTransaction) Context() context.Context { return tx.ctx }

// WithinTransaction supplies fixture context only; native SQL authority has separate primary tests.
func (f *dispatchFixture) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	return callback(dispatchTransaction{ctx: ctx})
}

// SelectDueWork exposes a bounded fixture snapshot without claiming native due/clock validation.
func (f *dispatchFixture) SelectDueWork(_ context.Context, _ platform.TransactionContext, limit int) ([]processing.Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	f.limits = append(f.limits, limit)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]processing.Candidate(nil), f.items[:min(limit, len(f.items))]...), nil
}

// SupersedeCandidate is not part of this fixture's dispatch path and fails explicitly.
func (f *dispatchFixture) SupersedeCandidate(context.Context, platform.TransactionContext, uuid.UUID, int64) (processing.Candidate, error) {
	return processing.Candidate{}, errors.New("unexpected fixture supersession")
}

// ExecuteWork measures simultaneous work, waits for release/cancellation and simulates durable success.
func (f *dispatchFixture) ExecuteWork(ctx context.Context, candidate processing.Candidate) error {
	f.mu.Lock()
	f.active++
	f.maximum = max(f.maximum, f.active)
	f.starts = append(f.starts, candidate.ID)
	release, failure := f.release, f.failure
	f.mu.Unlock()
	if release != nil {
		select {
		case <-ctx.Done():
			failure = ctx.Err()
		case <-release:
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active--
	f.exited++
	if failure == nil {
		for i, item := range f.items {
			if item.ID == candidate.ID {
				f.items = append(f.items[:i], f.items[i+1:]...)
				break
			}
		}
	}
	return failure
}

// newDispatchFixture supplies only identities; native lifecycle validation has its own primary tests.
func newDispatchFixture(count int) *dispatchFixture {
	f := &dispatchFixture{}
	for range count {
		f.items = append(f.items, processing.Candidate{ID: uuid.New()})
	}
	return f
}

// newTestDispatcher binds the fixture to the actual production interval/concurrency.
func newTestDispatcher(t *testing.T, f *dispatchFixture) *Dispatcher {
	t.Helper()
	dispatcher, err := NewDispatcher(f, f, f, slog.New(slog.NewTextHandler(io.Discard, nil)), DispatchConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return dispatcher
}

// counts snapshots concurrent fixture observations under the same mutex as execution.
func (f *dispatchFixture) counts() (starts, remaining, active, maximum, exited int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.starts), len(f.items), f.active, f.maximum, f.exited
}

func TestDispatcherConcurrencyAndJoinedShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(5)
		f.release = make(chan struct{})
		dispatcher := newTestDispatcher(t, f)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		synctest.Wait()
		if starts, _, active, maximum, _ := f.counts(); maximum != 2 || active != 2 || starts != 2 {
			t.Fatalf("unbounded/absent dispatch: max=%d active=%d starts=%d", maximum, active, starts)
		}
		dispatcher.Wake()
		dispatcher.Wake()
		time.Sleep(2 * DefaultDispatchPollInterval)
		synctest.Wait()
		if starts, _, _, _, _ := f.counts(); starts != 2 {
			t.Fatal("poll/wake duplicated locally active work")
		}
		if err := dispatcher.Run(ctx); platform.FailureCode(err) != platform.CodeRevisionConflict {
			t.Fatalf("concurrent runner accepted: %v", err)
		}
		cancel()
		synctest.Wait()
		err := <-done
		if _, _, active, _, exited := f.counts(); err != nil || active != 0 || exited != 2 {
			t.Fatalf("shutdown did not join: active=%d exited=%d err=%v", active, exited, err)
		}
	})
}

func TestDispatcherFallbackWakeAndFailureCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(0)
		dispatcher := newTestDispatcher(t, f)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		synctest.Wait()
		f.mu.Lock()
		f.items = append(f.items, processing.Candidate{ID: uuid.New()})
		f.mu.Unlock()
		time.Sleep(DefaultDispatchPollInterval - time.Nanosecond)
		synctest.Wait()
		if starts, _, _, _, _ := f.counts(); starts != 0 {
			t.Fatal("fallback polled before its approved interval")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if starts, remaining, _, _, _ := f.counts(); starts != 1 || remaining != 0 {
			t.Fatal("fallback did not discover remote work")
		}
		f.mu.Lock()
		f.items = append(f.items, processing.Candidate{ID: uuid.New()})
		f.failure = errors.New("private guest data must not be logged")
		f.mu.Unlock()
		dispatcher.Wake()
		synctest.Wait()
		if starts, _, _, _, _ := f.counts(); starts != 2 {
			t.Fatal("local wake did not dispatch without waiting for poll")
		}
		synctest.Wait()
		if starts, _, _, _, _ := f.counts(); starts != 2 {
			t.Fatal("failed work entered a tight dispatch loop")
		}
		time.Sleep(DefaultDispatchPollInterval)
		synctest.Wait()
		if starts, _, _, _, _ := f.counts(); starts != 3 {
			t.Fatal("failed slot did not become available on fallback")
		}
		cancel()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

func TestDispatcherDrainsQueueAndSurfacesListFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(9)
		dispatcher := newTestDispatcher(t, f)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		synctest.Wait()
		if starts, remaining, _, maximum, _ := f.counts(); starts != 9 || remaining != 0 || maximum > DefaultDispatchConcurrency {
			t.Fatalf("queue did not drain within bound: starts=%d remaining=%d max=%d", starts, remaining, maximum)
		}

		failure := errors.New("native selection failed")
		f.mu.Lock()
		f.listErr = failure
		f.mu.Unlock()
		dispatcher.Wake()
		synctest.Wait()
		if err := <-done; !errors.Is(err, failure) {
			t.Fatalf("list failure was hidden: %v", err)
		}
	})
}

func TestDispatcherConfigurationValidationAndCustomScheduling(t *testing.T) {
	f := newDispatchFixture(0)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, config := range []DispatchConfig{{Concurrency: -1}, {PollInterval: -time.Second}} {
		if _, err := NewDispatcher(f, f, f, logger, config); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("invalid explicit configuration silently defaulted: %+v err=%v", config, err)
		}
	}
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(206)
		f.release = make(chan struct{})
		config := DispatchConfig{Concurrency: 205, PollInterval: 7 * time.Second}
		dispatcher, err := NewDispatcher(f, f, f, logger, config)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		synctest.Wait()
		if starts, _, active, _, _ := f.counts(); starts != config.Concurrency || active != config.Concurrency {
			t.Fatalf("custom concurrency ignored: starts=%d active=%d", starts, active)
		}
		cancel()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(0)
		config := DispatchConfig{Concurrency: 1, PollInterval: 7 * time.Second}
		dispatcher, err := NewDispatcher(f, f, f, logger, config)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		synctest.Wait()
		f.mu.Lock()
		f.items = append(f.items, processing.Candidate{ID: uuid.New()})
		f.mu.Unlock()
		time.Sleep(config.PollInterval - time.Nanosecond)
		synctest.Wait()
		if starts, _, _, _, _ := f.counts(); starts != 0 {
			t.Fatal("configured poll interval shortened")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if starts, _, _, _, _ := f.counts(); starts != 1 {
			t.Fatal("configured fallback did not run")
		}
		cancel()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

// flakyTransactions fails the first failures transactions with the given error, then delegates.
type flakyTransactions struct {
	mu       sync.Mutex
	inner    processing.ExecutionTransactions
	failures int
	err      error
	calls    int
}

// WithinTransaction simulates a transient storage outage before delegating to the fixture.
func (f *flakyTransactions) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
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
	return f.inner.WithinTransaction(ctx, callback)
}

func (f *flakyTransactions) snapshot() (calls, failures int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.failures
}

func TestDispatcherSurvivesTransientSelectionFailures(t *testing.T) {
	for _, code := range []platform.ErrorCode{platform.CodeStorageUnavailable,
		platform.CodeAuthorityUnavailable, platform.CodeCommitUncertain} {
		t.Run(string(code), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newDispatchFixture(3)
				transactions := &flakyTransactions{inner: f, failures: 3,
					err: platform.NewFailure(code, errors.New("database restarting"))}
				dispatcher, err := NewDispatcher(transactions, f, f,
					slog.New(slog.NewTextHandler(io.Discard, nil)), DispatchConfig{})
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan error, 1)
				go func() { done <- dispatcher.Run(ctx) }()
				synctest.Wait()
				if calls, _ := transactions.snapshot(); calls != 1 {
					t.Fatalf("selection attempts=%d want 1 before backoff", calls)
				}
				// Wake-ups during backoff must not bypass it.
				dispatcher.Wake()
				synctest.Wait()
				if calls, _ := transactions.snapshot(); calls != 1 {
					t.Fatalf("wake bypassed backoff: attempts=%d", calls)
				}
				// Backoff doubles from the poll interval: 5 s, 10 s, 20 s.
				time.Sleep(DefaultDispatchPollInterval)
				synctest.Wait()
				if calls, _ := transactions.snapshot(); calls != 2 {
					t.Fatalf("attempts=%d want 2 after first backoff", calls)
				}
				time.Sleep(2*DefaultDispatchPollInterval - time.Nanosecond)
				synctest.Wait()
				if calls, _ := transactions.snapshot(); calls != 2 {
					t.Fatalf("second backoff did not double: attempts=%d", calls)
				}
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				time.Sleep(4 * DefaultDispatchPollInterval)
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("dispatcher stopped on transient failure: %v", err)
				default:
				}
				if _, failures := transactions.snapshot(); failures != 0 {
					t.Fatalf("remaining injected failures=%d", failures)
				}
				// Recovered selection drains the queue at the approved concurrency.
				time.Sleep(DefaultDispatchPollInterval)
				synctest.Wait()
				if starts, remaining, _, _, _ := f.counts(); starts != 3 || remaining != 0 {
					t.Fatalf("work not processed after recovery: starts=%d remaining=%d", starts, remaining)
				}
				cancel()
				synctest.Wait()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestDispatcherStopsPromptlyWhileBackingOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(1)
		transactions := &flakyTransactions{inner: f, failures: 1000,
			err: platform.NewFailure(platform.CodeStorageUnavailable, errors.New("down"))}
		dispatcher, err := NewDispatcher(transactions, f, f, slog.New(slog.NewTextHandler(io.Discard, nil)), DispatchConfig{})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		time.Sleep(time.Hour)
		synctest.Wait()
		// Backoff caps at 30 s: roughly 120 attempts per hour, never a tight loop or a stall.
		if calls, _ := transactions.snapshot(); calls < 110 || calls > 130 {
			t.Fatalf("backoff not bounded: %d attempts in an hour", calls)
		}
		start := time.Now()
		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("dispatcher did not stop on cancellation during backoff")
		}
		if time.Since(start) != 0 {
			t.Fatal("cancellation waited for backoff")
		}
	})
}

func TestDispatcherSelectsAtMostConcurrencyRowsPerAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newDispatchFixture(5)
		f.release = make(chan struct{})
		dispatcher := newTestDispatcher(t, f)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- dispatcher.Run(ctx) }()
		synctest.Wait()
		// Rows running locally stay due in storage; asking for the full concurrency keeps free
		// slots reachable behind them without paging.
		f.mu.Lock()
		limits := append([]int(nil), f.limits...)
		f.mu.Unlock()
		if len(limits) != 1 || limits[0] != DefaultDispatchConcurrency {
			t.Fatalf("selection limits=%v want [%d]", limits, DefaultDispatchConcurrency)
		}
		cancel()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
