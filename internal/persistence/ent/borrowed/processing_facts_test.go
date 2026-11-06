package borrowed_test

import (
	"errors"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestProcessingFactParticipantUsesCallerTransactionAndRejectsExpiredCapability(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	appender, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	configID, resultFactID := uuid.New(), uuid.New()
	workflowID, resultID := uuid.New(), uuid.New()
	var saved platform.TransactionContext
	write := func(tx platform.TransactionContext, fail bool) error {
		saved = tx
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := accountContext(tx.Context(), driver, entql.FieldNotNil("id"), true)
		if err := createAccount(ctx, client, uuid.New(), uuid.NewString(), now); err != nil {
			return err
		}
		if _, err := appender.AppendProcessingConfiguration(tx.Context(), tx, events.ProcessingConfigurationFact{
			ID: configID, WorkflowID: workflowID, Revision: 1, OccurredAt: now, ChangeKind: "version-saved",
		}); err != nil {
			return err
		}
		// These are explicit standalone publication-fixture references, not ready production originals.
		completion := events.ProcessingCompletionFact{ID: resultFactID, WorkflowID: workflowID,
			ResultID: resultID, ResultRevision: 1, TransmissionID: uuid.New(), MessageID: uuid.New(),
			DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1, OccurredAt: now,
			OutputSchema: map[string]processing.FieldSchema{},
			Result:       processing.Evaluation{Selected: true, Fields: map[string]processing.FieldValue{}}}
		if fail {
			completion.MessageID = uuid.Nil
		}
		_, err = appender.AppendProcessingCompletion(tx.Context(), tx, completion)
		return err
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error { return write(tx, true) })
	if platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("invalid completion did not abort every participant: %v", err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error { return write(tx, false) }); err != nil {
		t.Fatal(err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error { return write(tx, false) })
	if platform.FailureCode(err) != platform.CodeRevisionConflict {
		t.Fatalf("duplicate Processing fact did not abort caller state: %v", err)
	}
	if _, err := appender.AppendProcessingConfiguration(t.Context(), saved, events.ProcessingConfigurationFact{}); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("Processing append accepted an expired transaction: %v", err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var accounts, facts int
		if err := executor.QueryRowContext(tx.Context(),
			`SELECT (SELECT count(*) FROM identity_accounts), (SELECT count(*) FROM domain_events)`).
			Scan(&accounts, &facts); err != nil {
			return err
		}
		if accounts != 1 || facts != 2 {
			return errors.New("Processing fact failure/duplicate left partial caller state")
		}
		var producer, aggregate, replay, status string
		if err := executor.QueryRowContext(tx.Context(),
			`SELECT envelope->'producer'->>'domain', envelope->'aggregateRef'->>'id',
			        envelope->>'replayMode', envelope->'data'->>'status'
			 FROM domain_events WHERE id=$1`, resultFactID).
			Scan(&producer, &aggregate, &replay, &status); err != nil {
			return err
		}
		if producer != "processing" || aggregate != resultID.String() || replay != "normal" || status != "completed" {
			return errors.New("persisted Processing completion lost producer/result identity")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
