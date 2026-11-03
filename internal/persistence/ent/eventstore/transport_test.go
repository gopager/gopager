package eventstore

import (
	"context"
	"testing"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// foreignCapability cannot convey ownership of the store's actual transaction.
type foreignCapability struct{}

// Context supplies a live context without manufacturing transaction authority.
func (foreignCapability) Context() context.Context { return context.Background() }

func TestTransportExecutorNeverFallsBackFromInvalidCapability(t *testing.T) {
	for _, capability := range []platform.TransactionContext{nil, foreignCapability{}} {
		executor := transactionExecutor{primary: &postgres.Store{}, capability: capability}
		if _, err := executor.ExecContext(t.Context(), "must not execute"); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("ExecContext accepted invalid ownership: %v", err)
		}
		if _, err := executor.QueryContext(t.Context(), "must not query"); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("QueryContext accepted invalid ownership: %v", err)
		}
	}
}
