package borrowed_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

// seedAgencyScope inserts a fresh Agency and returns a Workflow scope assigned to exactly it
// without narrowing.
func seedAgencyScope(t *testing.T, primary *postgres.Store) processing.WorkflowScope {
	t.Helper()
	agency := uuid.New()
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := primary.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO agencies(id,name) VALUES($1,'Workflow fixture agency')`, agency)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return processing.WorkflowScope{Agencies: []uuid.UUID{agency}}
}

// agencyScope returns a Workflow scope assigned to the fixture's first Agency without narrowing.
func (f matcherFixture) agencyScope() processing.WorkflowScope {
	return processing.WorkflowScope{Agencies: []uuid.UUID{f.agencies[0]}}
}

// workflowNetworkScope returns a Workflow scope assigned to the fixture's first Agency and narrowed to
// the given Radio network.
func workflowNetworkScope(f matcherFixture, network uuid.UUID) processing.WorkflowScope {
	return processing.WorkflowScope{Agencies: []uuid.UUID{f.agencies[0]},
		Narrowing: &processing.WorkflowNarrowing{Kind: "network", References: []uuid.UUID{network}}}
}
