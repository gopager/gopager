package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// DefaultDispatchConcurrency is the number of candidates processed in parallel when
// DispatchConfig.Concurrency is zero.
const DefaultDispatchConcurrency = 2

// DefaultDispatchPollInterval is the idle polling interval used when
// DispatchConfig.PollInterval is zero.
const DefaultDispatchPollInterval = 5 * time.Second

// maxDispatchBackoff bounds the delay before the next selection attempt after
// consecutive transient storage failures.
const maxDispatchBackoff = 30 * time.Second

// DispatchConfig separates scheduling policy from mechanism; the Operator sets it with
// --processing-concurrency and --processing-poll-interval.
type DispatchConfig struct {
	// Concurrency is the maximum number of candidates evaluated in parallel.
	Concurrency int
	// PollInterval is the fallback interval for discovering due work; notifications wake the
	// dispatcher earlier.
	PollInterval time.Duration
}

// WithDefaults supplies approved initial values only for omitted configuration.
func (c DispatchConfig) WithDefaults() DispatchConfig {
	if c.Concurrency == 0 {
		c.Concurrency = DefaultDispatchConcurrency
	}
	if c.PollInterval == 0 {
		c.PollInterval = DefaultDispatchPollInterval
	}
	return c
}

// Validate rejects invalid scheduling values without silently replacing explicit negative input.
func (c DispatchConfig) Validate() error {
	if c.Concurrency <= 0 || c.PollInterval <= 0 {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Processing dispatcher configuration"))
	}
	return nil
}

// WorkExecutor handles one due candidate, including owner-authorized recovery or supersession.
type WorkExecutor interface {
	ExecuteWork(context.Context, processing.Candidate) error
}

// Dispatcher implements the platform runner with bounded in-flight work and coalesced wake-ups.
// It selects due candidates without claiming them; WorkExecutor.ExecuteWork reconciles and claims
// each with a revision check, which remains the correctness guard when several dispatchers run.
type Dispatcher struct {
	transactions processing.ExecutionTransactions
	repository   processing.CandidateWorkRepository
	executor     WorkExecutor
	logger       *slog.Logger
	wake         chan struct{}
	running      atomic.Bool
	config       DispatchConfig
}

// NewDispatcher requires native work selection and the actual current-owner execution participant.
func NewDispatcher(transactions processing.ExecutionTransactions, repository processing.CandidateWorkRepository,
	executor WorkExecutor, logger *slog.Logger, config DispatchConfig) (*Dispatcher, error) {
	if transactions == nil || repository == nil || executor == nil || logger == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing dispatcher dependency"))
	}
	config = config.WithDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Dispatcher{transactions: transactions, repository: repository, executor: executor,
		logger: logger, wake: make(chan struct{}, 1), config: config}, nil
}

// Wake coalesces hints that new work may be due (a database notification or a local commit); the
// fallback poll covers a hint that was lost.
func (d *Dispatcher) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// slotState is a locally dispatched candidate's phase.
type slotState int

const (
	// slotActive is executing in a goroutine of this process.
	slotActive slotState = iota + 1
	// slotCooling failed and waits for the next wake or poll before it may run again.
	slotCooling
	// slotRetryable failed earlier and may run again once no fresh work is waiting.
	slotRetryable
)

type dispatchOutcome struct {
	id  uuid.UUID
	err error
}

// Run dispatches at most the approved concurrency and joins all work before returning.
// A failed candidate cools down until the next wake/poll and then runs again only behind fresh due work,
// avoiding both a tight loop on unchanged rejected work and starvation of later work.
// A transient selection failure (STORAGE_UNAVAILABLE, AUTHORITY_UNAVAILABLE, COMMIT_UNCERTAIN or
// REVISION_CONFLICT) is logged and retried after a backoff that starts at the poll interval and
// doubles up to 30 s; wake-ups and polls do not select while backing off. Run returns nil on
// cancellation and returns any other selection failure.
func (d *Dispatcher) Run(ctx context.Context) error {
	if !d.running.CompareAndSwap(false, true) {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("Processing dispatcher already running"))
	}
	defer d.running.Store(false)
	work, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer workers.Wait()
	defer cancel()
	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()
	done := make(chan dispatchOutcome, d.config.Concurrency)
	// slots tracks every candidate this process dispatched and that may still be due in storage.
	slots := make(map[uuid.UUID]slotState)
	fill := func() error {
		active, idle := 0, 0
		for _, state := range slots {
			if state == slotActive {
				active++
			} else {
				idle++
			}
		}
		if active == d.config.Concurrency || work.Err() != nil {
			return nil
		}
		// Locally active and failed rows stay due in storage and sort first by due time. Asking for
		// that many extra rows keeps rows behind them reachable without paging, so rejected early
		// candidates cannot starve later work.
		var due []processing.Candidate
		if err := d.transactions.WithinTransaction(work, func(tx platform.TransactionContext) error {
			var err error
			due, err = d.repository.SelectDueWork(tx.Context(), tx, d.config.Concurrency+idle)
			return err
		}); err != nil {
			return err
		}
		stillDue := make(map[uuid.UUID]bool, len(due))
		for _, candidate := range due {
			stillDue[candidate.ID] = true
		}
		for id, state := range slots {
			if state != slotActive && !stillDue[id] {
				delete(slots, id)
			}
		}
		// Fresh rows first; a failed row is retried only after a wake or poll marked it retryable.
		for _, retrying := range []bool{false, true} {
			for _, candidate := range due {
				if active == d.config.Concurrency || work.Err() != nil {
					return nil
				}
				state, known := slots[candidate.ID]
				if state == slotActive || state == slotCooling || known == !retrying {
					continue
				}
				slots[candidate.ID] = slotActive
				active++
				workers.Go(func() {
					done <- dispatchOutcome{id: candidate.ID, err: d.executor.ExecuteWork(work, candidate)}
				})
			}
		}
		return nil
	}
	// endCooldowns makes failed rows retryable so unchanged rejected work is retried on the next
	// wake or poll, not in a tight loop.
	endCooldowns := func() {
		for id, state := range slots {
			if state == slotCooling {
				slots[id] = slotRetryable
			}
		}
	}
	failures := 0
	var retry *time.Timer
	var retryC <-chan time.Time
	defer func() {
		if retry != nil {
			retry.Stop()
		}
	}()
	// attempt selects work unless backing off; it absorbs transient failures.
	attempt := func() error {
		if retryC != nil {
			return nil
		}
		err := fill()
		if err == nil {
			if failures > 0 {
				d.logger.Info("Processing dispatch selection recovered", "failures", failures)
				failures = 0
			}
			return nil
		}
		if ctx.Err() != nil || !transientFailure(err) {
			return err
		}
		failures++
		delay := dispatchBackoff(d.config.PollInterval, failures)
		d.logger.Warn("Processing dispatch selection failed; retrying",
			"code", platform.FailureCode(err), "failures", failures, "retryIn", delay)
		retry = time.NewTimer(delay)
		retryC = retry.C
		return nil
	}
	if err := attempt(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case outcome := <-done:
			if outcome.err != nil {
				slots[outcome.id] = slotCooling
				if work.Err() == nil {
					d.logger.Warn("Processing candidate dispatch failed",
						"candidateId", outcome.id, "code", platform.FailureCode(outcome.err))
				}
			} else {
				delete(slots, outcome.id)
			}
		case <-retryC:
			retryC = nil
		case <-ticker.C:
			endCooldowns()
		case <-d.wake:
			endCooldowns()
		}
		if err := attempt(); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
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

// dispatchBackoff doubles base per consecutive failure, bounded by maxDispatchBackoff.
func dispatchBackoff(base time.Duration, failures int) time.Duration {
	delay := base
	for i := 1; i < failures && delay < maxDispatchBackoff; i++ {
		delay *= 2
	}
	return min(delay, max(base, maxDispatchBackoff))
}
