package events

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

const (
	// DefaultPollInterval is the fallback interval at which a Runner looks for work when no
	// wake-up arrives.
	DefaultPollInterval = 5 * time.Second
	// batchSize is the maximum number of records one batch transaction reads.
	batchSize = 100
)

// Transactor runs a callback in a primary transaction. *postgres.Store implements it.
type Transactor interface {
	WithinTransaction(ctx context.Context, callback platform.TxFunc) error
}

// Runner drives one Consumer over the fact log. Each batch is one primary transaction that
// reads the consumer's offset, reads up to 100 records after it, handles them in order and
// advances the offset, so the consumer's effect and its offset commit atomically.
type Runner struct {
	consumer     Consumer
	log          Log
	transactions Transactor
	wakes        WakeSubscriber
	logger       *slog.Logger
	pollInterval time.Duration
	blocked      atomic.Bool
}

// NewRunner returns a Runner for consumer. wakes provides the wake-up signals and
// pollInterval the fallback tick; it fails with an invalid-input failure for a missing
// dependency, an empty consumer name or a non-positive pollInterval.
func NewRunner(consumer Consumer, log Log, transactions Transactor, wakes WakeSubscriber,
	logger *slog.Logger, pollInterval time.Duration) (*Runner, error) {
	if consumer == nil || consumer.Name() == "" || log == nil || transactions == nil || wakes == nil ||
		logger == nil || pollInterval <= 0 {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid event runner"))
	}
	return &Runner{consumer: consumer, log: log, transactions: transactions, wakes: wakes,
		logger: logger, pollInterval: pollInterval}, nil
}

// Blocked reports whether the last batch stopped at a record of a supported type with an
// unknown schema version. It clears once a batch completes without hitting such a record.
func (r *Runner) Blocked() bool { return r.blocked.Load() }

// Run processes the log on every wake-up and poll tick until ctx ends, then returns nil. It
// implements platform.Runner. Batch errors are logged and retried on the next tick.
func (r *Runner) Run(ctx context.Context) error {
	signals, release := r.wakes.Subscribe()
	defer release()
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		r.drain(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-signals:
		case <-ticker.C:
		}
	}
}

// drain runs batches until one reads no records, blocks or fails.
func (r *Runner) drain(ctx context.Context) {
	for ctx.Err() == nil {
		more, err := r.runBatch(ctx)
		if err != nil {
			if ctx.Err() == nil {
				r.logger.Error("event consumer batch failed", "consumer", r.consumer.Name(), "error", err)
			}
			return
		}
		if !more {
			return
		}
	}
}

// batchResult is what a committed batch reports.
type batchResult struct {
	// more is true when the batch consumed records and the next batch may find more.
	more bool
	// blocked is true when the batch stopped at blocker, a record of unknown version.
	blocked bool
	blocker Record
	version int
}

// runBatch processes one batch in one transaction and reports whether another batch should
// follow. The callback resets its state because the store may rerun it.
func (r *Runner) runBatch(ctx context.Context) (bool, error) {
	var result batchResult
	name := r.consumer.Name()
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		result = batchResult{}
		offset, err := r.log.Offset(tx.Context(), tx, name)
		if err != nil {
			return err
		}
		records, err := r.log.ReadAfter(tx.Context(), tx, offset, batchSize)
		if err != nil {
			return err
		}
		var last *Record
		for i := range records {
			record := records[i]
			version := SchemaVersion(record)
			switch r.consumer.Supports(record.Type, version) {
			case UnknownVersion:
				result.blocked, result.blocker, result.version = true, record, version
			case Supported:
				if err := r.consumer.Handle(tx.Context(), tx, record); err != nil {
					return err
				}
			case Unsupported:
			}
			if result.blocked {
				break
			}
			last = &records[i]
		}
		if last != nil {
			if err := r.log.Advance(tx.Context(), tx, name, last.Position()); err != nil {
				return err
			}
		}
		result.more = len(records) > 0 && !result.blocked
		return nil
	})
	if err != nil {
		return false, err
	}
	r.blocked.Store(result.blocked)
	if result.blocked {
		r.logger.Error("event consumer blocked", "consumer", name,
			"txId", result.blocker.TxID, "seq", result.blocker.Seq,
			"type", result.blocker.Type, "schemaVersion", result.version)
	}
	return result.more, nil
}
