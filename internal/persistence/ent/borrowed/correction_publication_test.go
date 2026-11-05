package borrowed_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
)

// noCorrectionPublication isolates correction derivation tests from C-11 re-entry, which is covered below.
type noCorrectionPublication struct{}

func (noCorrectionPublication) PublicationReentries(context.Context, platform.TransactionContext, int) ([]uuid.UUID, error) {
	return nil, nil
}

func (noCorrectionPublication) ReenterPublication(context.Context, platform.TransactionContext, uuid.UUID) (bool, error) {
	return false, nil
}

// TestCorrectionPublicationReentryWaitsForCompletionAndRepublishes covers C-05 post-correction publication
// re-entry, the F-83 unavailable-until-completion rule and the F-84 refusal of stale Processing publication.
func TestCorrectionPublicationReentryWaitsForCompletionAndRepublishes(t *testing.T) {
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
		t.Fatal("correction runner accepted a missing publication participant")
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
	exec(`INSERT INTO networks(id,name,protocol) VALUES($1,'Correction publication','pocsag')`, networkID)
	exec(`INSERT INTO gateways(id,state) VALUES($1,'active')`, gatewayID)
	exec(`INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,is_default)
		VALUES($1,$2,'default',$3,true)`, uuid.New(), gatewayID, networkID)
	unitA, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{Name: "Publication A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unitB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{Name: "Publication B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(),
		ruleInput(networkID, unitA.ID, 10, 20, 1), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}

	function := uint8(1)
	target := protocols.Target{Address: protocols.NumericAddress(10), Function: &function}
	base := time.Now().UTC().Truncate(time.Second).Add(-2 * time.Hour)
	accept := func(id string, observed time.Time) ingest.Receipt {
		t.Helper()
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
	type published struct {
		processing, publication, message string
		unit                             uuid.NullUUID
		ready                            bool
	}
	read := func(receipt ingest.Receipt) published {
		t.Helper()
		var value published
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT t.processing_status,t.publication_status,
				m.publication_status FROM transmissions t JOIN messages m ON m.id=t.message_id WHERE t.id=$1`,
				receipt.TransmissionID).Scan(&value.processing, &value.publication, &value.message); err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT c.unit_id,c.ready FROM message_contributions c
				JOIN contribution_supports s ON s.contribution_id=c.id
				WHERE s.transmission_id=$1 AND s.target_position=0`, receipt.TransmissionID).
				Scan(&value.unit, &value.ready)
		}); err != nil {
			t.Fatal(err)
		}
		return value
	}
	expect := func(name string, receipt ingest.Receipt, state string, unit uuid.UUID, ready bool) {
		t.Helper()
		got := read(receipt)
		want := published{processing: map[string]string{"ready": "completed", "pending": "pending"}[state],
			publication: state, message: state, unit: uuid.NullUUID{UUID: unit, Valid: true}, ready: ready}
		if got != want {
			t.Fatalf("%s publication state=%+v, want %+v", name, got, want)
		}
	}

	first := accept("reentry-first", base)
	expect("initial intake", first, "ready", unitA.ID, true)

	// r2 rule change: the affected scope becomes unavailable before any correction work runs.
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, rule.ID, rule.Revision,
		ruleInput(networkID, unitB.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	expect("pending correction", first, "pending", unitA.ID, false)

	// Concurrent intake into a new Message is published at intake, then rebuilt by dirty capture.
	second := accept("reentry-dirty", base.Add(time.Hour))
	if second.MessageID == first.MessageID {
		t.Fatal("dirty intake unexpectedly joined the first Message")
	}
	expect("dirty intake", second, "ready", unitB.ID, true)

	// F-84: stale Processing publication and re-entry are refused while correction is unfinished.
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return sourceOwner.Publish(tx.Context(), tx, first.TransmissionID)
	}); platform.FailureCode(err) != platform.CodeRevisionConflict {
		t.Fatalf("publication during unfinished correction was not refused: %v", err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		ids, err := sourceOwner.PublicationReentries(tx.Context(), tx, 100)
		if err == nil && len(ids) != 0 {
			t.Fatalf("re-entry selected Transmissions during unfinished correction: %v", ids)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Select a Workflow so re-entry must admit current Processing work instead of publishing directly.
	workflowID, versionID := uuid.New(), uuid.New()
	protocolField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	protocolValue, err := processing.Literal(processing.StringValue, string(protocols.POCSAG))
	if err != nil {
		t.Fatal(err)
	}
	definition, err := json.Marshal(processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &protocolField, Value: &protocolValue},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	exec(`INSERT INTO processing_workflows(id,name,scope_kind,enabled,created_at,updated_at)
		VALUES($1,'correction re-entry','all',true,$2,$2)`, workflowID, now)
	exec(`INSERT INTO processing_workflow_versions(id,workflow_id,definition,created_at,created_by)
		VALUES($1,$2,$3,$4,$5)`, versionID, workflowID, definition, now, uuid.New())
	exec(`UPDATE processing_workflows SET active_version_id=$2 WHERE id=$1`, workflowID, versionID)

	// The correction claims, rebuilds both Messages and completes, but does not itself publish.
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("correction did not run: worked=%t err=%v", worked, err)
	}
	expect("completed correction", first, "pending", unitB.ID, false)
	expect("rebuilt dirty intake", second, "pending", unitB.ID, false)

	// The next pass re-enters both into C-11 and admits current Workflow candidates.
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("publication re-entry did not run: worked=%t err=%v", worked, err)
	}
	expect("admitted re-entry", first, "pending", unitB.ID, false)
	if worked, err := runner.RunOnce(t.Context()); err != nil || worked {
		t.Fatalf("re-entry selected Transmissions with unfinished Processing work: worked=%t err=%v", worked, err)
	}
	var candidates []processing.Candidate
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		page, err := processingRepository.ListCandidateWork(tx.Context(), tx, processing.PageInput{Limit: 10})
		candidates = page.Items
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("re-entry admitted %d candidates, want 2", len(candidates))
	}
	service, err := worker.NewWithPublisher(f.store, processingRepository, sourceOwner, processing.CoreRegex{}, uuid.New(), sourceOwner)
	if err != nil {
		t.Fatal(err)
	}
	var assignmentRevision int64
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT revision FROM assignment_control WHERE id=1`).
			Scan(&assignmentRevision)
	}); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		if candidate.ContextRevision != assignmentRevision {
			t.Fatalf("candidate bound to a stale Assignment context: %+v", candidate)
		}
		if err := service.ExecuteWork(t.Context(), candidate); err != nil {
			t.Fatal(err)
		}
	}
	expect("republished", first, "ready", unitB.ID, true)
	expect("republished dirty intake", second, "ready", unitB.ID, true)
	if worked, err := runner.RunOnce(t.Context()); err != nil || worked {
		t.Fatalf("idle runner reported work after publication: worked=%t err=%v", worked, err)
	}
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

// TestCorrectionExhaustionFailsSafelyAndRetryCreatesCurrentWork covers the F-84 failure/retry slice of C-05.
func TestCorrectionExhaustionFailsSafelyAndRetryCreatesCurrentWork(t *testing.T) {
	f := newMatcherFixture(t)
	var revision int64
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		revision, err = f.ruleStore.MarkPending(tx.Context(), tx, []uuid.UUID{f.networks[0]}, nil, "rule-change")
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
		failingCorrectionReads{f.ruleStore}, f.facts, noCorrectionPublication{})
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
	for _, private := range []string{f.networks[0].String(), "fencing", "lease", "injected"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(private)) {
			t.Fatalf("correction status exposed %q: %s", private, encoded)
		}
	}

	// A caller covering only another Network cannot distinguish the job from an unavailable one.
	replaceManagementGrants(t, f, []authz.Grant{{
		Actions: []string{"assignment-correction.read", "assignment-correction.retry"},
		Scope:   authz.Scope{Kind: "network", References: []string{f.networks[1].String()}},
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

	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noCorrectionPublication{})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("retried correction did not run: worked=%t err=%v", worked, err)
	}
	completed, err := f.rules.ReadCorrection(t.Context(), f.proof, retried.ID)
	if err != nil || completed.State != "completed" {
		t.Fatalf("retried correction status=%+v err=%v", completed, err)
	}
}
