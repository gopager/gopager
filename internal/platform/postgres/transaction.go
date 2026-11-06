package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gopager/gopager/internal/platform"
)

// Executor is the library SQL surface exposed only to infrastructure repositories.
type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type databaseTransaction interface {
	Executor
	Commit() error
	Rollback() error
}

type transaction struct {
	ctx    context.Context
	store  *Store
	sql    databaseTransaction
	active atomic.Bool
}

type transactionKey struct{}

func (t *transaction) Context() context.Context { return t.ctx }

// Resolve returns the current library transaction to an infrastructure adapter.
// Foreign-store and expired capabilities fail rather than falling back to a connection.
func (s *Store) Resolve(capability platform.TransactionContext) (Executor, error) {
	tx, ok := capability.(*transaction)
	if !ok || tx.store != s || !tx.active.Load() {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid transaction capability"))
	}
	if err := tx.ctx.Err(); err != nil {
		return nil, classifyFailure(err)
	}
	return tx.sql, nil
}

// WithinTransaction executes a synchronous callback once with the same transaction
// for all participating repositories. It never retries an uncertain commit. When a
// migration collection is registered, the transaction's first statement compares the
// applied schema version with the version recorded by Migrate and fails with
// CodeIncompatibleVersion on a difference; it does not take the migration lock.
func (s *Store) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	if callback == nil || ctx.Value(transactionKey{}) != nil {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("nil callback or nested transaction"))
	}
	ctx, cancel := context.WithTimeout(ctx, platform.DefaultCallTimeout)
	defer cancel()
	s.mu.RLock()
	registered, expected := s.registered, s.schemaVersion
	s.mu.RUnlock()
	return s.withinTransaction(ctx, callback, func(ctx context.Context) (databaseTransaction, error) {
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return nil, err
		}
		var primary bool
		var applied int64
		if registered {
			err = tx.QueryRowContext(ctx, `SELECT NOT pg_is_in_recovery() AND NOT current_setting('transaction_read_only')::boolean,
				(SELECT coalesce(max(version_id), 0) FROM platform_schema_migrations WHERE is_applied)`).Scan(&primary, &applied)
		} else {
			err = tx.QueryRowContext(ctx, "SELECT NOT pg_is_in_recovery() AND NOT current_setting('transaction_read_only')::boolean").Scan(&primary)
		}
		if err != nil {
			return nil, errors.Join(platform.NewFailure(platform.CodeAuthorityUnavailable, err), rollback(tx))
		}
		if !primary {
			return nil, errors.Join(platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("write authority unavailable")), rollback(tx))
		}
		if registered && applied != expected {
			return nil, errors.Join(platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("applied schema version differs from the registered collection")), rollback(tx))
		}
		return tx, nil
	})
}

func rollback(tx interface{ Rollback() error }) error {
	err := tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	if err != nil {
		return platform.NewFailure(platform.CodeStorageUnavailable, err)
	}
	return nil
}

func (s *Store) withinTransaction(ctx context.Context, callback platform.TxFunc, begin func(context.Context) (databaseTransaction, error)) error {
	if callback == nil || ctx.Value(transactionKey{}) != nil {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("nil callback or nested transaction"))
	}
	ctx, cancel := context.WithTimeout(ctx, platform.DefaultCallTimeout)
	defer cancel()
	raw, err := begin(ctx)
	if err != nil {
		return classifyFailure(err)
	}
	tx := &transaction{store: s, sql: raw}
	tx.ctx = context.WithValue(ctx, transactionKey{}, tx)
	tx.active.Store(true)
	defer func() {
		tx.active.Store(false)
		if err := raw.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("transaction cleanup failed", "code", platform.CodeStorageUnavailable)
		}
	}()
	if err := callback(tx); err != nil {
		return classifyFailure(err)
	}
	tx.active.Store(false)
	if err := ctx.Err(); err != nil {
		return classifyFailure(err)
	}
	if err := raw.Commit(); err != nil {
		if isUnknownCommit(err) {
			return platform.NewFailure(platform.CodeCommitUncertain, err)
		}
		return classifyFailure(err)
	}
	return nil
}

func classifyFailure(err error) error {
	var failure *platform.Failure
	if errors.As(err, &failure) {
		return err
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		switch databaseError.Code {
		case "40001", "40P01", "23505":
			return platform.NewFailure(platform.CodeRevisionConflict, err)
		case "25006":
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		case "22003", "53100", "53200", "53300", "53400":
			return platform.NewFailure(platform.CodeResourceLimit, err)
		}
	}
	return platform.NewFailure(platform.CodeStorageUnavailable, err)
}

// Now reads the primary's actual clock through the caller's transaction.
// No worker wall clock or cached transaction-start timestamp is used.
func (s *Store) Now(ctx context.Context, capability platform.TransactionContext) (time.Time, error) {
	tx, err := s.Resolve(capability)
	if err != nil {
		return time.Time{}, err
	}
	var now time.Time
	if err := tx.QueryRowContext(capability.Context(), "SELECT clock_timestamp()").Scan(&now); err != nil {
		return time.Time{}, classifyFailure(err)
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, classifyFailure(err)
	}
	return now.UTC().Truncate(time.Microsecond), nil
}

func isUnknownCommit(err error) bool {
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		// A transport/driver failure may follow a COMMIT already sent to the server.
		return true
	}
	return databaseError.Code == "40003" ||
		len(databaseError.Code) != 5 ||
		databaseError.Code[:2] == "08" ||
		databaseError.Code[:2] == "57" ||
		databaseError.Code[:2] == "58"
}
