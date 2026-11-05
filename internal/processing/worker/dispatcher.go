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

// DispatchConfig separates scheduling policy from mechanism without exposing an Operator setting yet.
type DispatchConfig struct {
	Concurrency  int
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

// Wake coalesces local post-commit hints; fallback polling discovers work from other processes.
func (d *Dispatcher) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

type dispatchOutcome struct {
	id  uuid.UUID
	err error
}

// Run dispatches at most the approved concurrency and joins all work before returning.
// Failed slots cool down until the next wake/poll, avoiding a tight loop on unchanged rejected work.
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
	// false retains only a failed slot's cooldown, not an active goroutine.
	reserved := make(map[uuid.UUID]bool)
	nextOffset := 0
	fill := func() error {
		if len(reserved) == d.config.Concurrency || work.Err() != nil {
			return nil
		}
		var selected []processing.Candidate
		if err := d.transactions.WithinTransaction(work, func(tx platform.TransactionContext) error {
			offset, scanned, total := nextOffset, 0, -1
			for {
				page, err := d.repository.ListCandidateWork(tx.Context(), tx,
					processing.PageInput{Offset: offset, Limit: 200})
				if err != nil {
					return err
				}
				if total < 0 {
					total = page.Total
				}
				if total == 0 {
					nextOffset = 0
					return nil
				}
				if offset >= total {
					offset = 0
					continue
				}
				for _, candidate := range page.Items {
					scanned++
					offset++
					nextOffset = offset % total
					if _, exists := reserved[candidate.ID]; !exists {
						selected = append(selected, candidate)
						if len(selected) == d.config.Concurrency-len(reserved) {
							return nil
						}
					}
					if scanned == total {
						return nil
					}
				}
				if len(page.Items) == 0 || scanned >= total {
					return nil
				}
				if offset >= total {
					offset = 0
				}
			}
		}); err != nil {
			return err
		}
		for _, candidate := range selected {
			if len(reserved) == d.config.Concurrency || work.Err() != nil {
				break
			}
			if _, exists := reserved[candidate.ID]; exists {
				continue
			}
			reserved[candidate.ID] = true
			workers.Go(func() {
				done <- dispatchOutcome{id: candidate.ID, err: d.executor.ExecuteWork(work, candidate)}
			})
		}
		return nil
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
				reserved[outcome.id] = false
				if work.Err() == nil {
					d.logger.Warn("Processing candidate dispatch failed",
						"candidateId", outcome.id, "code", platform.FailureCode(outcome.err))
				}
			} else {
				delete(reserved, outcome.id)
			}
		case <-retryC:
			retryC = nil
		case <-ticker.C:
			for id, active := range reserved {
				if !active {
					delete(reserved, id)
				}
			}
		case <-d.wake:
			for id, active := range reserved {
				if !active {
					delete(reserved, id)
				}
			}
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
