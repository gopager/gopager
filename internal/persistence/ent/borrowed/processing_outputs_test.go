package borrowed_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/testutil/attributeschemas"
)

// claimedWorkflowCandidate saves and activates a Workflow, admits a candidate for a new
// Transmission and claims it, so that the caller can complete it with an Evaluation.
func claimedWorkflowCandidate(t *testing.T, primary *postgres.Store, store *processingstore.Store) processing.CandidateLease {
	t.Helper()
	actor := uuid.New()
	authority := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	configuration := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1, Name: "outputs-" + uuid.NewString(),
		Scope: seedAgencyScope(t, primary), Enabled: true}
	version := processing.Version{ID: uuid.New(), WorkflowID: configuration.ID, CreatedBy: actor,
		Definition: preparedProcessingDefinition(t)}
	binding := processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: configuration.ID,
		DefinitionVersionID: version.ID, SourceRevision: 1, ContextRevision: 1, WorkflowRevision: 2}
	var lease processing.CandidateLease
	err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		configuration.CreatedAt, configuration.UpdatedAt, version.CreatedAt = now, now, now
		if err := store.SaveWorkflow(tx.Context(), tx, authority, "workflow.create", configuration, 0, now); err != nil {
			return err
		}
		if err := store.SaveVersion(tx.Context(), tx, version); err != nil {
			return err
		}
		configuration.Revision, configuration.ActiveVersionID = 2, &version.ID
		if err := store.SaveWorkflow(tx.Context(), tx, authority, "workflow.activate", configuration, 1, now); err != nil {
			return err
		}
		if err := store.RecordChange(tx.Context(), tx, actor, configuration.ID, "workflow.activate", now); err != nil {
			return err
		}
		if binding.ConfigurationRevision, err = store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		candidate, err := store.AdmitCandidate(tx.Context(), tx, binding)
		if err != nil {
			return err
		}
		if candidate, err = store.ClaimCandidate(tx.Context(), tx, candidate.ID, candidate.Revision, uuid.New()); err != nil {
			return err
		}
		lease, err = candidate.Lease()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

// alertsResult builds a selected Evaluation with the given fields of the schema "alerts".
func alertsResult(t *testing.T, fields map[string]processing.Value) processing.Evaluation {
	t.Helper()
	values := make(map[string]processing.FieldValue, len(fields))
	for name, value := range fields {
		values[name] = processing.FieldValue{State: processing.FieldPresent, Value: &value}
	}
	return processing.Evaluation{Selected: true, Fields: map[string]map[string]processing.FieldValue{"alerts": values}}
}

func TestProcessingResultsAreValidatedAgainstAttributeSchemas(t *testing.T) {
	t.Parallel()
	primary := newStore(t)
	store, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	attributeschemas.Insert(t, primary, "alerts", map[string]attributes.Field{
		"level": {Kind: processing.StringValue}, "count": {Kind: processing.IntegerValue}})
	level, err := processing.Literal(processing.StringValue, "high")
	if err != nil {
		t.Fatal(err)
	}
	count, err := processing.Literal(processing.IntegerValue, "3")
	if err != nil {
		t.Fatal(err)
	}
	complete := func(evaluation processing.Evaluation) (resultID uuid.UUID, stored string, err error) {
		lease := claimedWorkflowCandidate(t, primary, store)
		err = primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			_, result, err := store.CompleteCandidate(tx.Context(), tx, lease, 0, evaluation)
			if err != nil {
				return err
			}
			resultID = result.ID
			executor, err := primary.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT evaluation::text FROM processing_results WHERE id = $1`,
				result.ID).Scan(&stored)
		})
		return resultID, stored, err
	}

	// Two Workflows write the same field; each stores its own value.
	for range 2 {
		_, stored, err := complete(alertsResult(t, map[string]processing.Value{"level": level}))
		if err != nil || stored != `{"fields": {"alerts": {"level": {"state": "present", "value": {"kind": "string", "value": "high"}}}}, "selected": true}` {
			t.Fatalf("stored result = %s, err = %v", stored, err)
		}
	}

	rejected := map[string]processing.Evaluation{
		"unknown schema": {Selected: true, Fields: map[string]map[string]processing.FieldValue{
			"missing": {"level": {State: processing.FieldPresent, Value: &level}}}},
		"unknown field": alertsResult(t, map[string]processing.Value{"other": level}),
		"wrong type":    alertsResult(t, map[string]processing.Value{"count": level}),
		"absent value": {Selected: true, Fields: map[string]map[string]processing.FieldValue{
			"alerts": {"level": {State: processing.FieldAbsent}}}},
	}
	for name, evaluation := range rejected {
		if _, _, err := complete(evaluation); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s: result stored or wrong failure: %v", name, err)
		}
	}

	if _, _, err := complete(alertsResult(t, map[string]processing.Value{"count": count})); err != nil {
		t.Fatalf("integer field rejected before deprecation: %v", err)
	}
	// A deprecation never stops an active version from storing its results.
	deprecated := alertsResult(t, map[string]processing.Value{"count": count})
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := primary.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `UPDATE attribute_schemas SET deprecated = true WHERE key = 'alerts'`); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE attribute_catalog SET revision = revision + 1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := complete(deprecated); err != nil {
		t.Fatalf("deprecated schema stopped a result: %v", err)
	}
}
