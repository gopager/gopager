package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// TestDomainSentinelsKeepTheirOwnCodeThroughTransactions guards against classifyFailure
// reporting a domain outcome (for example a stale revision) as a storage outage.
func TestDomainSentinelsKeepTheirOwnCodeThroughTransactions(t *testing.T) {
	for _, test := range []struct {
		sentinel error
		want     platform.ErrorCode
	}{
		{authz.ErrRevisionConflict, "REVISION_CONFLICT"},
		{authz.ErrNotFound, "NOT_FOUND"},
		{authz.ErrForbidden, "FORBIDDEN"},
		{authz.ErrInvalidInput, "INVALID_INPUT"},
		{identity.ErrLoginRequired, "LOGIN_REQUIRED"},
		{identity.ErrInvalidInput, "INVALID_INPUT"},
		{commands.ErrIdempotencyConflict, "IDEMPOTENCY_CONFLICT"},
	} {
		t.Run(string(test.want)+"/"+test.sentinel.Error(), func(t *testing.T) {
			store := &Store{}
			err := store.withinTransaction(context.Background(), func(platform.TransactionContext) error {
				return fmt.Errorf("context: %w", test.sentinel)
			}, func(context.Context) (databaseTransaction, error) { return &testTransaction{}, nil })
			if platform.FailureCode(err) != test.want || !errors.Is(err, test.sentinel) {
				t.Fatalf("code=%s is(sentinel)=%t, want %s and a matching sentinel",
					platform.FailureCode(err), errors.Is(err, test.sentinel), test.want)
			}
		})
	}
}

// TestSerializationFailureIsRetriedOnce covers the rolled-back 40001/40P01 outcomes: a
// second attempt is safe, a third is not made.
func TestSerializationFailureIsRetriedOnce(t *testing.T) {
	for _, state := range []string{"40001", "40P01"} {
		t.Run(state+" then success", func(t *testing.T) {
			store := &Store{}
			var transactions []*testTransaction
			calls := 0
			err := store.withinTransaction(context.Background(), func(platform.TransactionContext) error {
				calls++
				if calls == 1 {
					return &pgconn.PgError{Code: state}
				}
				return nil
			}, func(context.Context) (databaseTransaction, error) {
				tx := &testTransaction{}
				transactions = append(transactions, tx)
				return tx, nil
			})
			if err != nil || calls != 2 || len(transactions) != 2 ||
				!transactions[0].rolledBack || transactions[0].committed || !transactions[1].committed {
				t.Fatalf("err=%v calls=%d transactions=%d", err, calls, len(transactions))
			}
		})
		t.Run(state+" twice", func(t *testing.T) {
			store := &Store{}
			calls := 0
			err := store.withinTransaction(context.Background(), func(platform.TransactionContext) error {
				calls++
				return &pgconn.PgError{Code: state}
			}, func(context.Context) (databaseTransaction, error) { return &testTransaction{}, nil })
			var failure *platform.Failure
			if calls != 2 || platform.FailureCode(err) != platform.CodeRevisionConflict ||
				!errors.As(err, &failure) || !failure.Retryable() {
				t.Fatalf("calls=%d err=%v, want two attempts and a retryable REVISION_CONFLICT", calls, err)
			}
		})
	}
	t.Run("other errors are not retried", func(t *testing.T) {
		store := &Store{}
		calls := 0
		err := store.withinTransaction(context.Background(), func(platform.TransactionContext) error {
			calls++
			return errors.New("storage failure")
		}, func(context.Context) (databaseTransaction, error) { return &testTransaction{}, nil })
		if calls != 1 || platform.FailureCode(err) != platform.CodeStorageUnavailable {
			t.Fatalf("calls=%d code=%s", calls, platform.FailureCode(err))
		}
	})
}

// TestUniqueViolationIsConflictButNotRetryable: replaying the same request hits the same
// constraint, so the client must change the request instead of retrying.
func TestUniqueViolationIsConflictButNotRetryable(t *testing.T) {
	store := &Store{}
	calls := 0
	err := store.withinTransaction(context.Background(), func(platform.TransactionContext) error {
		calls++
		return &pgconn.PgError{Code: "23505"}
	}, func(context.Context) (databaseTransaction, error) { return &testTransaction{}, nil })
	var failure *platform.Failure
	if !errors.As(err, &failure) || failure.Code() != platform.CodeRevisionConflict || failure.Retryable() || calls != 1 {
		t.Fatalf("err=%v calls=%d, want one attempt and a non-retryable REVISION_CONFLICT", err, calls)
	}
}
