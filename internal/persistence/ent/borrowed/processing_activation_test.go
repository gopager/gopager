package borrowed_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// TestProcessingActivationValidatesReferencesForTheActivator expects a retained version to be
// judged by the activator's field rights exactly like a saved one: a version that reads a field
// of a group the activator may not read (here one stored before save-time validation existed)
// is refused with INVALID_INPUT, the Workflow revision does not move, and a readable version
// still activates.
func TestProcessingActivationValidatesReferencesForTheActivator(t *testing.T) {
	f := newMatcherFixture(t)
	service, _ := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("activation", authz.Scope{Kind: "all"}), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	store, err := processingstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	definition := preparedProcessingDefinition(t)
	secret := processing.FieldRef{Namespace: "source", Path: []string{"secret"}}
	definition.OutputSchema = map[string]processing.FieldSchema{"out": {Kind: processing.BooleanValue}}
	definition.Outputs = map[string]processing.Assignment{"out": {Field: &secret}}
	retained := uuid.New()
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return store.SaveVersion(tx.Context(), tx, processing.Version{ID: retained, WorkflowID: created.ID,
			Definition: definition, CreatedBy: f.principal, CreatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivateVersion(t.Context(), f.proof, created.ID, retained, 1, checkedMutation); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("activator without the referenced field group activated the version: %v", err)
	}
	readable, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1,
		preparedProcessingDefinition(t), checkedMutation)
	if err != nil {
		t.Fatalf("refused activation moved the Workflow revision: %v", err)
	}
	if _, err := service.ActivateVersion(t.Context(), f.proof, created.ID, readable.ID, 2, checkedMutation); err != nil {
		t.Fatalf("readable version did not activate: %v", err)
	}
}
