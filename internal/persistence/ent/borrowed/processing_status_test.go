package borrowed_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
)

// processingSourceFixture composes the production intake store, Processing store, matcher,
// Processing source owner and worker over one test database with one Network and one Gateway.
type processingSourceFixture struct {
	store       *postgres.Store
	intake      *ingeststore.Store
	processing  *processingstore.Store
	assignments *assignmentstore.Store
	matcher     *assignments.Service
	owner       *ingeststore.ProcessingSourceOwner
	worker      *worker.Service
	network     uuid.UUID
	gateway     uuid.UUID
	now         time.Time
}

func newProcessingSourceFixture(t *testing.T) *processingSourceFixture {
	t.Helper()
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	intakeRepository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	processingRepository, err := processingstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	identityRepository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	assignmentRepository, err := assignmentstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := commandstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(identityRepository, store, ingestTestScopeReferences{})
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := assignments.NewService(store, resolver, store, assignmentRepository,
		unitRepository, intakeRepository, receipts, facts)
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
	executions, err := processing.NewInstanceExecutions(host, nil, store, processingRepository, principals)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executions.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	owner, err := ingeststore.NewProcessingSourceOwner(intakeRepository, matcher, processingRepository,
		facts, store, ingestTestArtifacts{}, executions, principals)
	if err != nil {
		t.Fatal(err)
	}
	service, err := worker.NewWithPublisher(store, processingRepository, owner, processing.CoreRegex{}, uuid.New(), owner)
	if err != nil {
		t.Fatal(err)
	}
	f := &processingSourceFixture{store: store, intake: intakeRepository, processing: processingRepository,
		assignments: assignmentRepository, matcher: matcher, owner: owner, worker: service,
		network: uuid.New(), gateway: uuid.New(), now: time.Now().UTC().Truncate(time.Second)}
	f.exec(t, `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id) INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'status network','pocsag',a.id FROM a`, f.network)
	f.exec(t, `INSERT INTO gateways(id,state) VALUES($1,'active')`, f.gateway)
	f.exec(t, `INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
		VALUES($1,$2,'default',$3,0)`, uuid.New(), f.gateway, f.network)
	return f
}

// exec runs one statement in its own transaction.
func (f *processingSourceFixture) exec(t *testing.T, query string, args ...any) {
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

// queryRow scans one row in its own transaction.
func (f *processingSourceFixture) queryRow(t *testing.T, query string, args []any, dest ...any) {
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

// seedWorkflow activates an enabled Workflow with scope all and the given definition.
func (f *processingSourceFixture) seedWorkflow(t *testing.T, name string, definition processing.Definition) uuid.UUID {
	t.Helper()
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	workflow, version := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO processing_workflows(id,name,scope_kind,enabled,created_at,updated_at)
		VALUES($1,$2,'all',true,$3,$3)`, workflow, name, f.now)
	f.exec(t, `INSERT INTO processing_workflow_versions(id,workflow_id,definition,created_at,created_by)
		VALUES($1,$2,$3,$4,$5)`, version, workflow, encoded, f.now, uuid.New())
	f.exec(t, `UPDATE processing_workflows SET active_version_id=$2 WHERE id=$1`, workflow, version)
	return workflow
}

// accept validates, matches and accepts one submission for the fixture Gateway.
func (f *processingSourceFixture) accept(t *testing.T, id string, observed time.Time, payload string,
	targets []protocols.Target,
) ingest.Receipt {
	t.Helper()
	if targets == nil {
		targets = []protocols.Target{}
	}
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
		ObservationTime: observed.Format(time.RFC3339), GatewayNetworkIdentifier: "default",
		Payload: json.RawMessage(payload), TransmissionTargets: targets}
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
		matched, err := f.matcher.MatchTargets(tx.Context(), tx, f.network, protocols.POCSAG,
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

// dueCandidates lists the currently due Processing work.
func (f *processingSourceFixture) dueCandidates(t *testing.T) []processing.Candidate {
	t.Helper()
	var items []processing.Candidate
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		page, err := f.processing.SelectDueWork(tx.Context(), tx, 50)
		items = page
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return items
}

// transmissionStatus reads the Transmission's processing status.
func (f *processingSourceFixture) transmissionStatus(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	f.queryRow(t, `SELECT processing_status FROM transmissions WHERE id=$1`, []any{id}, &status)
	return status
}

// publishedEvidence counts the published results of a Message and the processing.completed
// facts of a Transmission.
func (f *processingSourceFixture) publishedEvidence(t *testing.T, message, transmission uuid.UUID) (results, facts int) {
	t.Helper()
	f.queryRow(t, `SELECT
		(SELECT count(*) FROM processing_results WHERE message_id=$1 AND published_at IS NOT NULL),
		(SELECT count(*) FROM domain_events WHERE envelope->>'type'='processing.completed'
			AND envelope->'data'->>'transmissionId'=$2)`,
		[]any{message, transmission.String()}, &results, &facts)
	return results, facts
}

// protocolSelectorDefinition selects every POCSAG source and produces no fields.
func protocolSelectorDefinition(t *testing.T) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	value, err := processing.Literal(processing.StringValue, string(protocols.POCSAG))
	if err != nil {
		t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &field, Value: &value},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
}

// undeclaredFieldDefinition references a source field that no source declares, so compiling it
// fails with INVALID_INPUT, a failure no retry can overcome.
func undeclaredFieldDefinition(t *testing.T) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"not-declared"}}
	value, err := processing.Literal(processing.StringValue, "missing")
	if err != nil {
		t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &field, Value: &value},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
}

const toneOnlyPayload = `{"form":"tone-only"}`

// TestIntakeWithoutSelectedWorkflowReportsNoProcessing expects a Transmission that selects no
// Workflow to be accepted with processing status none, both in the receipt and in the row.
func TestIntakeWithoutSelectedWorkflowReportsNoProcessing(t *testing.T) {
	f := newProcessingSourceFixture(t)
	receipt := f.accept(t, "no-workflow", f.now, toneOnlyPayload, nil)
	if receipt.ProcessingStatus != "none" {
		t.Fatalf("receipt status=%q want none", receipt.ProcessingStatus)
	}
	if status := f.transmissionStatus(t, receipt.TransmissionID); status != "none" {
		t.Fatalf("stored status=%q want none", status)
	}
}

// TestCompletedCandidatePublishesWhileSiblingPending expects the result of a completed
// candidate to be published immediately, while the Transmission stays pending until the
// candidate of the second selected Workflow has completed as well.
func TestCompletedCandidatePublishesWhileSiblingPending(t *testing.T) {
	f := newProcessingSourceFixture(t)
	f.seedWorkflow(t, "first", protocolSelectorDefinition(t))
	f.seedWorkflow(t, "second", protocolSelectorDefinition(t))
	receipt := f.accept(t, "two-workflows", f.now, toneOnlyPayload, nil)
	if receipt.ProcessingStatus != "pending" {
		t.Fatalf("receipt status=%q want pending", receipt.ProcessingStatus)
	}
	candidates := f.dueCandidates(t)
	if len(candidates) != 2 {
		t.Fatalf("due candidates=%d want 2", len(candidates))
	}
	if err := f.worker.ExecuteWork(t.Context(), candidates[0]); err != nil {
		t.Fatal(err)
	}
	results, facts := f.publishedEvidence(t, receipt.MessageID, receipt.TransmissionID)
	if status := f.transmissionStatus(t, receipt.TransmissionID); status != "pending" || results != 1 || facts != 1 {
		t.Fatalf("after first completion: status=%s results=%d facts=%d; want pending/1/1", status, results, facts)
	}
	if err := f.worker.ExecuteWork(t.Context(), candidates[1]); err != nil {
		t.Fatal(err)
	}
	results, facts = f.publishedEvidence(t, receipt.MessageID, receipt.TransmissionID)
	if status := f.transmissionStatus(t, receipt.TransmissionID); status != "completed" || results != 2 || facts != 2 {
		t.Fatalf("after second completion: status=%s results=%d facts=%d; want completed/2/2", status, results, facts)
	}
}

// TestFailedCandidateMarksTransmissionFailed expects a candidate that fails terminally to mark
// its Transmission failed once no selected Workflow is pending, while the other Workflow's
// result stays published.
func TestFailedCandidateMarksTransmissionFailed(t *testing.T) {
	f := newProcessingSourceFixture(t)
	f.seedWorkflow(t, "sound", protocolSelectorDefinition(t))
	f.seedWorkflow(t, "broken", undeclaredFieldDefinition(t))
	receipt := f.accept(t, "one-broken", f.now, toneOnlyPayload, nil)
	candidates := f.dueCandidates(t)
	if len(candidates) != 2 {
		t.Fatalf("due candidates=%d want 2", len(candidates))
	}
	failures := 0
	for _, candidate := range candidates {
		if err := f.worker.ExecuteWork(t.Context(), candidate); err != nil {
			failures++
		}
	}
	results, facts := f.publishedEvidence(t, receipt.MessageID, receipt.TransmissionID)
	if status := f.transmissionStatus(t, receipt.TransmissionID); failures != 1 || status != "failed" || results != 1 || facts != 1 {
		t.Fatalf("failures=%d status=%s results=%d facts=%d; want 1/failed/1/1", failures, status, results, facts)
	}
}

// TestMergeMovesPublishedResultsToSurvivor expects a Message merge to move the published
// results of absorbed Messages to the surviving Message, and the schema to refuse a result
// that references no Message.
func TestMergeMovesPublishedResultsToSurvivor(t *testing.T) {
	f := newProcessingSourceFixture(t)
	f.exec(t, `UPDATE grouping_control SET window_seconds=30 WHERE id=1`)
	f.seedWorkflow(t, "merge", protocolSelectorDefinition(t))
	first := f.accept(t, "merge-first", f.now, toneOnlyPayload, nil)
	second := f.accept(t, "merge-second", f.now.Add(60*time.Second), toneOnlyPayload, nil)
	if first.MessageID == second.MessageID {
		t.Fatal("transmissions 60 s apart joined one Message with a 30 s window")
	}
	for _, candidate := range f.dueCandidates(t) {
		if err := f.worker.ExecuteWork(t.Context(), candidate); err != nil {
			t.Fatal(err)
		}
	}
	bridge := f.accept(t, "merge-bridge", f.now.Add(30*time.Second), toneOnlyPayload, nil)
	var survivorResults, danglingResults, messages int
	f.queryRow(t, `SELECT
		(SELECT count(*) FROM processing_results WHERE message_id=$1 AND published_at IS NOT NULL),
		(SELECT count(*) FROM processing_results r WHERE r.message_id IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.id=r.message_id)),
		(SELECT count(*) FROM messages)`, []any{bridge.MessageID}, &survivorResults, &danglingResults, &messages)
	if survivorResults != 2 || danglingResults != 0 || messages != 1 {
		t.Fatalf("after merge: survivor results=%d dangling=%d messages=%d; want 2/0/1", survivorResults, danglingResults, messages)
	}
	var candidate uuid.UUID
	f.queryRow(t, `SELECT id FROM processing_candidates WHERE transmission_id=$1`, []any{first.TransmissionID}, &candidate)
	err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO processing_results
			(id,candidate_id,revision,producing_fence,evaluation,completed_at,published_at,message_id)
			VALUES($1,$2,99,1,'{"selected":true,"fields":{}}',clock_timestamp(),clock_timestamp(),$3)`,
			uuid.New(), candidate, uuid.New())
		return err
	})
	if err == nil {
		t.Fatal("a result referencing no Message was accepted")
	}
}

// TestReconcileReadmitsAfterContextRevisionChange expects the worker to replace a candidate
// whose Assignment context is stale by a current one in the same dispatch, instead of
// leaving the Transmission without current work. The context is the assignment revision of
// the Transmission's own targets, so the test restamps them as a correction batch would.
func TestReconcileReadmitsAfterContextRevisionChange(t *testing.T) {
	f := newProcessingSourceFixture(t)
	f.seedWorkflow(t, "context", protocolSelectorDefinition(t))
	function := uint8(1)
	receipt := f.accept(t, "stale-context", f.now, toneOnlyPayload,
		[]protocols.Target{{Address: protocols.NumericAddress(10), Function: &function}})
	candidates := f.dueCandidates(t)
	if len(candidates) != 1 {
		t.Fatalf("due candidates=%d want 1", len(candidates))
	}
	stale := candidates[0]
	var revision int64
	f.exec(t, `UPDATE assignment_control SET revision=revision+1 WHERE id=1`)
	f.exec(t, `UPDATE transmission_targets SET assignment_revision=(SELECT revision FROM assignment_control WHERE id=1)
		WHERE transmission_id=$1`, receipt.TransmissionID)
	f.queryRow(t, `SELECT revision FROM assignment_control WHERE id=1`, nil, &revision)
	if err := f.worker.ExecuteWork(t.Context(), stale); err != nil {
		t.Fatalf("dispatch of a stale candidate failed instead of readmitting: %v", err)
	}
	var superseded, pending int
	var pendingContext int64
	f.queryRow(t, `SELECT count(*) FILTER (WHERE state='superseded'), count(*) FILTER (WHERE state='pending'),
		coalesce(max(context_revision) FILTER (WHERE state='pending'),0)
		FROM processing_candidates WHERE transmission_id=$1`, []any{receipt.TransmissionID}, &superseded, &pending, &pendingContext)
	if superseded != 1 || pending != 1 || pendingContext != revision {
		t.Fatalf("after reconcile: superseded=%d pending=%d pendingContext=%d want 1/1/%d", superseded, pending, pendingContext, revision)
	}
	if status := f.transmissionStatus(t, receipt.TransmissionID); status != "pending" {
		t.Fatalf("status=%s want pending", status)
	}
	current := f.dueCandidates(t)
	if len(current) != 1 || current[0].ID == stale.ID {
		t.Fatalf("due work after reconcile=%+v", current)
	}
	if err := f.worker.ExecuteWork(t.Context(), current[0]); err != nil {
		t.Fatal(err)
	}
	if status := f.transmissionStatus(t, receipt.TransmissionID); status != "completed" {
		t.Fatalf("status=%s want completed", status)
	}
}
