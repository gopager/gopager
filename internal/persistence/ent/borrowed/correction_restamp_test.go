package borrowed_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// restampFixture runs the production correction runner and Processing worker over a matcher
// fixture with one Network, one Gateway and one Unit.
type restampFixture struct {
	matcherFixture
	runner     *assignments.CorrectionRunner
	worker     *worker.Service
	processing *processingstore.Store
	intake     *ingeststore.Store
	network    uuid.UUID
	gateway    uuid.UUID
	unit       uuid.UUID
}

func newRestampFixture(t *testing.T) *restampFixture {
	t.Helper()
	f := &restampFixture{matcherFixture: newMatcherFixture(t), gateway: uuid.New()}
	var err error
	if f.intake, err = ingeststore.New(f.store, f.facts, f.facts); err != nil {
		t.Fatal(err)
	}
	if f.processing, err = processingstore.New(f.store); err != nil {
		t.Fatal(err)
	}
	host := processing.NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	principals := ingestTestPrincipalResolver{}
	executions, err := processing.NewInstanceExecutions(host, nil, f.store, f.processing, principals)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executions.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	owner, err := ingeststore.NewProcessingSourceOwner(f.intake, f.rules, f.processing, f.facts, f.store,
		ingestTestArtifacts{}, executions, principals)
	if err != nil {
		t.Fatal(err)
	}
	if f.runner, err = assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, owner); err != nil {
		t.Fatal(err)
	}
	if f.worker, err = worker.NewWithPublisher(f.store, f.processing, owner, processing.CoreRegex{}, uuid.New(), owner); err != nil {
		t.Fatal(err)
	}
	f.network = f.networks[0]
	f.exec(t, `INSERT INTO gateways(id,state) VALUES($1,'active')`, f.gateway)
	f.exec(t, `INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
		VALUES($1,$2,'default',$3,0)`, uuid.New(), f.gateway, f.network)
	created, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Restamp", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	f.unit = created.ID
	return f
}

func (f *restampFixture) exec(t *testing.T, query string, args ...any) {
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

func (f *restampFixture) queryRow(t *testing.T, query string, args []any, dest ...any) {
	t.Helper()
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), query, args...).Scan(dest...)
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *restampFixture) seedWorkflow(t *testing.T, definition processing.Definition) {
	t.Helper()
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	workflowID, versionID, now := uuid.New(), uuid.New(), time.Now().UTC().Truncate(time.Microsecond)
	f.exec(t, `INSERT INTO processing_workflows(id,name,scope_kind,enabled,created_at,updated_at)
		VALUES($1,'restamp','all',true,$2,$2)`, workflowID, now)
	f.exec(t, `INSERT INTO processing_workflow_versions(id,workflow_id,definition,created_at,created_by)
		VALUES($1,$2,$3,$4,$5)`, versionID, workflowID, encoded, now, uuid.New())
	f.exec(t, `UPDATE processing_workflows SET active_version_id=$2 WHERE id=$1`, workflowID, versionID)
}

// accept stores one Transmission carrying one target per address (function 1).
func (f *restampFixture) accept(t *testing.T, id string, addresses ...uint64) ingest.Receipt {
	t.Helper()
	function := uint8(1)
	targets := make([]protocols.Target, 0, len(addresses))
	for _, address := range addresses {
		targets = append(targets, protocols.Target{Address: protocols.NumericAddress(address), Function: &function})
	}
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
		ObservationTime: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), GatewayNetworkIdentifier: "default",
		Payload: json.RawMessage(`{"form":"tone-only"}`), TransmissionTargets: targets}
	var receipt ingest.Receipt
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		accepted, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		validated, err := submission.Validate(4096, accepted)
		if err != nil {
			return err
		}
		validated.ResolvedNetwork, validated.Resolution = f.network, "declared"
		matched, err := f.rules.MatchTargets(tx.Context(), tx, f.network, protocols.POCSAG,
			validated.ObservedAt, validated.TransmissionTargets)
		if err != nil {
			return err
		}
		receipt, err = f.intake.AcceptTransmission(tx.Context(), tx, f.gateway, validated, accepted, matched)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return receipt
}

// runDue executes all due candidates; worker failures are expected for failing Workflows.
func (f *restampFixture) runDue(t *testing.T) {
	t.Helper()
	var due []processing.Candidate
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		page, err := f.processing.SelectDueWork(tx.Context(), tx, 10)
		due = page
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range due {
		_ = f.worker.ExecuteWork(t.Context(), candidate)
	}
}

// candidateState is a Transmission's status with its candidate counts per state and the highest
// context revision among the candidates that are not superseded.
type candidateState struct {
	status                    string
	pending, failed, complete int
	current                   int64
}

func (f *restampFixture) candidates(t *testing.T, transmission uuid.UUID) candidateState {
	t.Helper()
	var value candidateState
	f.queryRow(t, `SELECT t.processing_status,
		count(c.id) FILTER (WHERE c.state='pending'), count(c.id) FILTER (WHERE c.state='failed'),
		count(c.id) FILTER (WHERE c.state='completed'),
		coalesce(max(c.context_revision) FILTER (WHERE c.state<>'superseded'),0)
		FROM transmissions t LEFT JOIN processing_candidates c ON c.transmission_id=t.id
		WHERE t.id=$1 GROUP BY t.id`, []any{transmission},
		&value.status, &value.pending, &value.failed, &value.complete, &value.current)
	return value
}

func (f *restampFixture) assignmentRevision(t *testing.T) int64 {
	t.Helper()
	var revision int64
	f.queryRow(t, `SELECT revision FROM assignment_control WHERE id=1`, nil, &revision)
	return revision
}

// TestRestampWithoutResolutionChangeReadmitsFailedCandidate expects a correction that restamps a
// Transmission's targets without changing their resolution to replace its stale failed candidate
// by a pending one at the new context, so the failure is neither permanent nor unretryable.
func TestRestampWithoutResolutionChangeReadmitsFailedCandidate(t *testing.T) {
	t.Parallel()
	f := newRestampFixture(t)
	f.seedWorkflow(t, undeclaredFieldDefinition(t))
	receipt := f.accept(t, "failed-restamp", 10)
	f.runDue(t)
	if got := f.candidates(t, receipt.TransmissionID); got.status != "failed" || got.failed != 1 {
		t.Fatalf("setup: %+v, want failed with one failed candidate", got)
	}
	// The rule's function selector excludes the target, so its resolution stays unassigned, but
	// the correction scope (which ignores the selector) restamps it.
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.network, f.unit, 10, 20, 2),
		checkedMutation); err != nil {
		t.Fatal(err)
	}
	revision := f.assignmentRevision(t)
	if drainCorrections(t, f.runner) == 0 {
		t.Fatal("correction did not run")
	}
	if got := f.candidates(t, receipt.TransmissionID); got.status != "pending" || got.pending != 1 || got.current != revision {
		t.Fatalf("after restamp: %+v, want pending with one pending candidate at context %d", got, revision)
	}
}

// TestSuppressionReprocessesCompletedCandidate expects a suppression, which never changes a
// target's resolution, to admit a fresh candidate so results computed with the suppressed target
// do not stay current.
func TestSuppressionReprocessesCompletedCandidate(t *testing.T) {
	t.Parallel()
	f := newRestampFixture(t)
	f.seedWorkflow(t, protocolSelectorDefinition(t))
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.network, f.unit, 10, 20, 1),
		checkedMutation); err != nil {
		t.Fatal(err)
	}
	receipt := f.accept(t, "suppressed", 10)
	f.runDue(t)
	if got := f.candidates(t, receipt.TransmissionID); got.status != "completed" || got.complete != 1 {
		t.Fatalf("setup: %+v, want completed with one completed candidate", got)
	}
	if _, err := f.rules.CreateSuppression(t.Context(), f.proof, uuid.New(),
		assignments.CreateSuppressionInput{TransmissionID: receipt.TransmissionID, TargetPosition: 0},
		checkedMutation); err != nil {
		t.Fatal(err)
	}
	revision := f.assignmentRevision(t)
	if drainCorrections(t, f.runner) == 0 {
		t.Fatal("correction did not run")
	}
	if got := f.candidates(t, receipt.TransmissionID); got.status != "pending" || got.pending != 1 || got.current != revision {
		t.Fatalf("after suppression: %+v, want pending with one pending candidate at context %d", got, revision)
	}
}

// TestMessageStaysCorrectingWhileAnotherScopeHoldsStaleTargets expects a Message whose targets
// has Transmissions in two different scopes to read correcting until the second scope has run.
// Batches of one scope cannot be split in tests (the batch size is fixed at 100), so two scopes
// cover the case.
func TestMessageStaysCorrectingWhileAnotherScopeHoldsStaleTargets(t *testing.T) {
	t.Parallel()
	f := newRestampFixture(t)
	other, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Restamp other", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.network, f.unit, 10, 20, 1), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.network, f.unit, 30, 40, 1), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	drainCorrections(t, f.runner)
	receipt := f.accept(t, "two-scopes-a", 10)
	if joined := f.accept(t, "two-scopes-b", 30); joined.MessageID != receipt.MessageID {
		t.Fatalf("intakes did not group into one Message: %s %s", receipt.MessageID, joined.MessageID)
	}
	for _, rule := range []struct {
		id       uuid.UUID
		revision int64
		from, to uint64
	}{{first.ID, first.Revision, 10, 20}, {second.ID, second.Revision, 30, 40}} {
		if _, err := f.rules.UpdateRule(t.Context(), f.proof, rule.id, rule.revision,
			ruleInput(f.network, other.ID, rule.from, rule.to, 1), checkedMutation); err != nil {
			t.Fatal(err)
		}
	}
	assertStatus := func(stage, want string) {
		t.Helper()
		rows := readMessageUnits(t, f.matcherFixture, receipt.MessageID)
		if len(rows) == 0 {
			t.Fatalf("%s: no message_units rows", stage)
		}
		for _, row := range rows {
			if row.status != want {
				t.Fatalf("%s: rows=%+v, want all %s", stage, rows, want)
			}
		}
	}
	assertStatus("before correction", "correcting")
	if did, err := f.runner.RunOnce(t.Context()); err != nil || !did {
		t.Fatalf("first scope: did=%t err=%v", did, err)
	}
	assertStatus("after the first scope, second still queued", "correcting")
	drainCorrections(t, f.runner)
	assertStatus("after both scopes", "current")
}
