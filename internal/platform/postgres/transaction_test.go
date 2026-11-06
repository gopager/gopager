package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gopager/gopager/internal/platform"
)

func TestTransactionOutcomesAndNoReplay(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		callbackErr error
		commitErr   error
		wantCode    platform.ErrorCode
		wantCommit  bool
	}{
		{"committed", nil, nil, "", true},
		{"callback failure", errors.New("private marker"), nil, platform.CodeStorageUnavailable, false},
		{"cancelled callback", context.Canceled, nil, platform.CodeStorageUnavailable, false},
		{"deadline", context.DeadlineExceeded, nil, platform.CodeStorageUnavailable, false},
		{"lost acknowledgement", nil, errors.New("private network marker"), platform.CodeCommitUncertain, true},
		{"transaction conflict", nil, &pgconn.PgError{Code: "40001"}, platform.CodeRevisionConflict, true},
		{"commit completion unknown", nil, &pgconn.PgError{Code: "40003"}, platform.CodeCommitUncertain, true},
		{"connection exception", nil, &pgconn.PgError{Code: "08006"}, platform.CodeCommitUncertain, true},
		{"server shutdown", nil, &pgconn.PgError{Code: "57P01"}, platform.CodeCommitUncertain, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := &Store{}
			raw := &testTransaction{commitErr: test.commitErr}
			calls := 0
			err := store.withinTransaction(context.Background(), func(capability platform.TransactionContext) error {
				calls++
				if _, err := store.Resolve(capability); err != nil {
					t.Fatalf("resolve active capability: %v", err)
				}
				deadline, ok := capability.Context().Deadline()
				if !ok || time.Until(deadline) > platform.DefaultCallTimeout {
					t.Fatal("ordinary-call deadline was not bounded by 30 seconds")
				}
				return test.callbackErr
			}, func(context.Context) (databaseTransaction, error) { return raw, nil })
			if platform.FailureCode(err) != test.wantCode || calls != 1 || raw.committed != test.wantCommit {
				t.Fatalf("outcome code=%s calls=%d committed=%t, want %s, 1, %t", platform.FailureCode(err), calls, raw.committed, test.wantCode, test.wantCommit)
			}
			if test.callbackErr != nil && !errors.Is(err, test.callbackErr) {
				t.Fatal("wrapped callback/cancellation cause was lost")
			}
			var failure *platform.Failure
			if errors.As(err, &failure) && failure.Code() == platform.CodeCommitUncertain && failure.Retryable() {
				t.Fatal("uncertain commit marked safe to retry")
			}
		})
	}
}

func TestCapabilitiesExpireAndCannotCrossStores(t *testing.T) {
	t.Parallel()
	store := &Store{}
	other := &Store{}
	var saved platform.TransactionContext
	raw := &testTransaction{}
	err := store.withinTransaction(context.Background(), func(capability platform.TransactionContext) error {
		saved = capability
		if _, err := other.Resolve(capability); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatal("foreign store resolved another store's capability")
		}
		return nil
	}, func(context.Context) (databaseTransaction, error) { return raw, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(saved); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatal("expired callback capability was resolved")
	}
}

func TestTransactionPanicRollsBackAndInvalidatesCapability(t *testing.T) {
	t.Parallel()
	store := &Store{}
	raw := &testTransaction{}
	var saved platform.TransactionContext
	func() {
		defer func() {
			if recover() == nil {
				t.Error("callback panic was swallowed")
			}
		}()
		_ = store.withinTransaction(context.Background(), func(capability platform.TransactionContext) error {
			saved = capability
			panic("synthetic callback panic")
		}, func(context.Context) (databaseTransaction, error) { return raw, nil })
	}()
	if !raw.rolledBack || raw.committed {
		t.Fatal("panicking callback did not roll back")
	}
	if _, err := store.Resolve(saved); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatal("panic left capability active")
	}
}

type testTransaction struct {
	Executor
	commitErr  error
	committed  bool
	rolledBack bool
}

func (t *testTransaction) Commit() error {
	t.committed = true
	return t.commitErr
}

func (t *testTransaction) Rollback() error {
	t.rolledBack = true
	if t.committed {
		return sql.ErrTxDone
	}
	return nil
}
