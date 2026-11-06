package borrowed_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// noProcessingReadmission isolates correction derivation tests from Processing readmission, which is covered below.
type noProcessingReadmission struct{}

func (noProcessingReadmission) ReadmitProcessing(context.Context, platform.TransactionContext, uuid.UUID) error {
	return nil
}

// TestCorrectionReadmitsProcessingForChangedTargets expects a correction that changes a target's
// resolution to supersede the Transmission's completed candidate and admit a current one in the
// same batch transaction, leave untouched Transmissions alone, and never hide published evidence.
func TestCorrectionReadmitsProcessingForChangedTargets(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	intakeRepository, err := ingeststore.New(f.store, f.facts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	processingRepository, err := processingstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	host := processing.NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	principals := ingestTestPrincipalResolver{}
	executions, err := processing.NewInstanceExecutions(host, nil, f.store, processingRepository, principals)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executions.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	sourceOwner, err := ingeststore.NewProcessingSourceOwner(intakeRepository, f.rules, processingRepository,
		f.facts, f.store, ingestTestArtifacts{}, executions, principals)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, sourceOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, nil); err == nil {
		t.Fatal("correction runner accepted a missing readmission participant")
	}
	service, err := worker.NewWithPublisher(f.store, processingRepository, sourceOwner, processing.CoreRegex{}, uuid.New(), sourceOwner)
	if err != nil {
		t.Fatal(err)
	}

	networkID, gatewayID := f.networks[0], uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			_, err = executor.ExecContext(tx.Context(), query, args...)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO gateways(id,state) VALUES($1,'active')`, gatewayID)
	exec(`INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
		VALUES($1,$2,'default',$3,0)`, uuid.New(), gatewayID, networkID)
	unitA, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Readmission A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unitB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Readmission B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(),
		ruleInput(networkID, unitA.ID, 10, 20, 1), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	workflowID, versionID := uuid.New(), uuid.New()
	definition, err := json.Marshal(protocolSelectorDefinition(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	exec(`INSERT INTO processing_workflows(id,name,scope_kind,enabled,created_at,updated_at)
		VALUES($1,'correction readmission','all',true,$2,$2)`, workflowID, now)
	exec(`INSERT INTO processing_workflow_versions(id,workflow_id,definition,created_at,created_by)
		VALUES($1,$2,$3,$4,$5)`, versionID, workflowID, definition, now, uuid.New())
	exec(`UPDATE processing_workflows SET active_version_id=$2 WHERE id=$1`, workflowID, versionID)

	function := uint8(1)
	base := time.Now().UTC().Truncate(time.Second).Add(-2 * time.Hour)
	accept := func(id string, observed time.Time, address uint64) ingest.Receipt {
		t.Helper()
		target := protocols.Target{Address: protocols.NumericAddress(address), Function: &function}
		submission := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: observed.Format(time.RFC3339), GatewayNetworkIdentifier: "default",
			Payload: json.RawMessage(`{"form":"tone-only"}`), TransmissionTargets: []protocols.Target{target}}
		var receipt ingest.Receipt
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			acceptedAt, err := f.store.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			validated, err := submission.Validate(4096, acceptedAt)
			if err != nil {
				return err
			}
			validated.ResolvedNetwork, validated.Resolution = networkID, "declared"
			matched, err := f.rules.MatchTargets(tx.Context(), tx, networkID, protocols.POCSAG,
				validated.ObservedAt, validated.TransmissionTargets)
			if err != nil {
				return err
			}
			receipt, err = intakeRepository.AcceptTransmission(tx.Context(), tx, gatewayID, validated, acceptedAt, matched)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	type state struct {
		status                         string
		unit                           uuid.NullUUID
		superseded, pending, completed int
		currentContext                 int64
		published                      int
	}
	read := func(receipt ingest.Receipt) state {
		t.Helper()
		var value state
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT t.processing_status,
				(SELECT unit_id FROM transmission_targets WHERE transmission_id=t.id AND position=0),
				(SELECT count(*) FROM processing_candidates c WHERE c.transmission_id=t.id AND c.state='superseded'),
				(SELECT count(*) FROM processing_candidates c WHERE c.transmission_id=t.id AND c.state='pending'),
				(SELECT count(*) FROM processing_candidates c WHERE c.transmission_id=t.id AND c.state='completed'),
				(SELECT coalesce(max(c.context_revision),0) FROM processing_candidates c
					WHERE c.transmission_id=t.id AND c.state<>'superseded'),
				(SELECT count(*) FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
					WHERE c.transmission_id=t.id AND r.published_at IS NOT NULL AND r.message_id=t.message_id)
				FROM transmissions t WHERE t.id=$1`, receipt.TransmissionID).
				Scan(&value.status, &value.unit, &value.superseded, &value.pending, &value.completed,
					&value.currentContext, &value.published)
		}); err != nil {
			t.Fatal(err)
		}
		return value
	}
	expect := func(name string, receipt ingest.Receipt, want state) {
		t.Helper()
		if got := read(receipt); got != want {
			t.Fatalf("%s state=%+v, want %+v", name, got, want)
		}
	}
	runDue := func(want int) {
		t.Helper()
		var due []processing.Candidate
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			page, err := processingRepository.SelectDueWork(tx.Context(), tx, 10)
			due = page
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(due) != want {
			t.Fatalf("due candidates=%d want %d", len(due), want)
		}
		for _, candidate := range due {
			if err := service.ExecuteWork(t.Context(), candidate); err != nil {
				t.Fatal(err)
			}
		}
	}
	assignmentRevision := func() int64 {
		t.Helper()
		var revision int64
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT revision FROM assignment_control WHERE id=1`).Scan(&revision)
		}); err != nil {
			t.Fatal(err)
		}
		return revision
	}

	corrected := accept("corrected", base, 10)
	untouched := accept("untouched", base.Add(time.Hour), 30)
	initial := assignmentRevision()
	runDue(2)
	expect("processed corrected intake", corrected, state{status: "completed", unit: uuid.NullUUID{UUID: unitA.ID, Valid: true},
		completed: 1, currentContext: initial, published: 1})
	expect("processed untouched intake", untouched, state{status: "completed", completed: 1, currentContext: initial, published: 1})

	// The rule moves the address range to Unit B; the queued correction changes only the first target.
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, rule.ID, rule.Revision,
		ruleInput(networkID, unitB.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	changed := assignmentRevision()
	if changed == initial {
		t.Fatal("rule change did not advance the Assignment revision")
	}
	expect("evidence stays visible before correction", corrected, state{status: "completed",
		unit: uuid.NullUUID{UUID: unitA.ID, Valid: true}, completed: 1, currentContext: initial, published: 1})
	if drainCorrections(t, runner) == 0 {
		t.Fatal("correction did not run")
	}
	expect("readmitted after correction", corrected, state{status: "pending", unit: uuid.NullUUID{UUID: unitB.ID, Valid: true},
		superseded: 1, pending: 1, currentContext: changed, published: 1})
	expect("untouched after correction", untouched, state{status: "completed", completed: 1, currentContext: initial, published: 1})
	if worked, err := runner.RunOnce(t.Context()); err != nil || worked {
		t.Fatalf("idle correction runner reported work: worked=%t err=%v", worked, err)
	}
	runDue(1)
	expect("reprocessed after correction", corrected, state{status: "completed", unit: uuid.NullUUID{UUID: unitB.ID, Valid: true},
		superseded: 1, completed: 1, currentContext: changed, published: 2})
}

// failingCorrectionReads injects a derivation failure before any batch can publish.
type failingCorrectionReads struct {
	assignments.CorrectionWorkRepository
}

func (failingCorrectionReads) ReadCorrectionBatch(context.Context, platform.TransactionContext,
	assignments.CorrectionLease, int,
) ([]assignments.CorrectionBatchItem, error) {
	return nil, errors.New("injected correction derivation failure")
}

// TestCorrectionExhaustionFailsSafelyAndRetryCreatesCurrentWork covers correction failure and retry.
func TestCorrectionExhaustionFailsSafelyAndRetryCreatesCurrentWork(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	var revision int64
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		revision, err = f.ruleStore.MarkPending(tx.Context(), tx, []assignments.CorrectionScope{networkScope(f.networks[0])}, "rule-change")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	query := func(statement string, args []any, dest ...any) {
		t.Helper()
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			if len(dest) == 0 {
				_, err = executor.ExecContext(tx.Context(), statement, args...)
				return err
			}
			return executor.QueryRowContext(tx.Context(), statement, args...).Scan(dest...)
		}); err != nil {
			t.Fatal(err)
		}
	}
	var jobID uuid.UUID
	query(`SELECT id FROM assignment_pending_scopes WHERE assignment_revision=$1`, []any{revision}, &jobID)

	failing, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules,
		failingCorrectionReads{f.ruleStore}, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		if worked, err := failing.RunOnce(t.Context()); err != nil || !worked {
			t.Fatalf("attempt %d did not run: worked=%t err=%v", attempt, worked, err)
		}
		var state string
		var attempts int
		query(`SELECT state,attempts FROM assignment_pending_scopes WHERE id=$1`, []any{jobID}, &state, &attempts)
		want := "queued"
		if attempt == 5 {
			want = "failed"
		}
		if state != want || attempts != attempt {
			t.Fatalf("attempt %d left state=%s attempts=%d, want %s", attempt, state, attempts, want)
		}
		// Make the bounded backoff due without waiting for it.
		query(`UPDATE assignment_pending_scopes SET lease_until=clock_timestamp()-interval '1 second'
			WHERE id=$1 AND state='queued'`, []any{jobID})
	}
	if worked, err := failing.RunOnce(t.Context()); err != nil || worked {
		t.Fatalf("exhausted correction was claimed again: worked=%t err=%v", worked, err)
	}

	status, err := f.rules.ReadCorrection(t.Context(), f.proof, jobID)
	if err != nil || status.State != "failed" || status.FailureCode == "" {
		t.Fatalf("failed correction status=%+v err=%v", status, err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	// The scope (Network, selector) is exposed to readers authorized for it; fences and leases are not.
	for _, private := range []string{"fencing", "lease", "injected"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(private)) {
			t.Fatalf("correction status exposed %q: %s", private, encoded)
		}
	}

	// A caller covering only another Network cannot distinguish the job from an unavailable one.
	replaceManagementGrants(t, f, []authz.Grant{{
		Actions: []string{"assignment-correction.read", "assignment-correction.retry"},
		Scope:   authz.Scope{Kind: "agency", References: []string{f.agencies[1].String()}},
	}})
	if _, err := f.rules.ReadCorrection(t.Context(), f.proof, jobID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("hidden correction read was not NOT_FOUND: %v", err)
	}
	if _, err := f.rules.RetryCorrection(t.Context(), f.proof, jobID, uuid.New(), checkedMutation); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("hidden correction retry was not NOT_FOUND: %v", err)
	}
	if _, err := f.rules.ReadCorrection(t.Context(), f.proof, uuid.New()); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("unknown correction was not NOT_FOUND: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: authz.Actions(), Scope: authz.Scope{Kind: "all"}}})

	command := uuid.New()
	retried, err := f.rules.RetryCorrection(t.Context(), f.proof, jobID, command, checkedMutation)
	if err != nil || retried.ID == jobID || retried.State != "pending" || retried.AssignmentRev != revision {
		t.Fatalf("retry did not create current-revision work: %+v err=%v", retried, err)
	}
	again, err := f.rules.RetryCorrection(t.Context(), f.proof, jobID, command, checkedMutation)
	if err != nil || again.ID != retried.ID {
		t.Fatalf("identical retry did not return its receipt: %+v err=%v", again, err)
	}
	if _, err := f.rules.RetryCorrection(t.Context(), f.proof, jobID, uuid.New(), checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("superseded failed work was revived: %v", err)
	}
	var oldState string
	query(`SELECT state FROM assignment_pending_scopes WHERE id=$1`, []any{jobID}, &oldState)
	if oldState != "superseded" {
		t.Fatalf("retried job state=%s, want superseded", oldState)
	}

	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	if drainCorrections(t, runner) == 0 {
		t.Fatal("retried correction did not run")
	}
	completed, err := f.rules.ReadCorrection(t.Context(), f.proof, retried.ID)
	if err != nil || completed.State != "completed" {
		t.Fatalf("retried correction status=%+v err=%v", completed, err)
	}
}
