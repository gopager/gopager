package borrowed_test

import (
	"testing"

	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/platform"
)

// TestCorrectionLeaseExpiredOnFifthAttemptEmitsOneFailedFact covers the claim path that
// exhausts a scope: the next claim fails it with RETRY_LIMIT and exactly one
// correction.failed fact records that final failure.
func TestCorrectionLeaseExpiredOnFifthAttemptEmitsOneFailedFact(t *testing.T) {
	f := newMatcherFixture(t)
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := f.ruleStore.MarkPending(tx.Context(), tx, []assignments.CorrectionScope{networkScope(f.networks[0])}, "rule-change")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	query := func(statement string, dest ...any) {
		t.Helper()
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			if len(dest) == 0 {
				_, err = executor.ExecContext(tx.Context(), statement)
				return err
			}
			return executor.QueryRowContext(tx.Context(), statement).Scan(dest...)
		}); err != nil {
			t.Fatal(err)
		}
	}
	query(`UPDATE assignment_pending_scopes SET state='running',attempts=5,fencing_token=5,
		lease_until=clock_timestamp()-interval '1 second'`)
	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := runner.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	var state, code string
	query(`SELECT state,failure_code FROM assignment_pending_scopes`, &state, &code)
	if state != "failed" || code != "RETRY_LIMIT" {
		t.Fatalf("exhausted scope state=%s code=%s", state, code)
	}
	var facts int
	query(`SELECT count(*) FROM domain_events WHERE envelope->>'type'='correction.failed'
		AND envelope->'data'->>'failureCode'='RETRY_LIMIT'`, &facts)
	if facts != 1 {
		t.Fatalf("correction.failed facts=%d, want exactly 1", facts)
	}
}
