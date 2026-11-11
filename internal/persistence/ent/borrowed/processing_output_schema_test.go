package borrowed_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/persistence/ent/attributestore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/testutil/attributeschemas"
)

// TestProcessingVersionOfADeletedSchemaCannotBeActivated expects a schema that only an inactive
// version writes to be deletable, and that version to be refused activation afterwards with
// INVALID_INPUT, leaving the active version untouched. A version saved after the schema was
// deleted is refused as well.
func TestProcessingVersionOfADeletedSchemaCannotBeActivated(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	service, _ := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	attributeschemas.Insert(t, f.store, "test", map[string]attributes.Field{
		"out": {Kind: processing.BooleanValue}, "text": {Kind: processing.StringValue}, "mode": {Kind: processing.StringValue}})
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("output-schema", f.agencyScope()), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1,
		preparedProcessingDefinition(t), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.ActivateVersion(t.Context(), f.proof, created.ID, first.ID, 2, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	writing := preparedProcessingDefinition(t)
	truth, err := processing.Literal(processing.BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	writing.Outputs = map[string]map[string]processing.Assignment{"test": {"out": {Literal: &truth}}}
	second, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, active.Revision, writing, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := attributestore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return schemas.Delete(tx.Context(), tx, "test", 1)
	}); err != nil {
		t.Fatalf("deleting a schema that only an inactive version writes: %v", err)
	}
	if _, err := service.ActivateVersion(t.Context(), f.proof, created.ID, second.ID, second.Revision, checkedMutation); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("activating a version of a deleted schema: %v, want INVALID_INPUT", err)
	}
	if _, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, second.Revision, writing, checkedMutation); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("saving a version of a deleted schema: %v, want INVALID_INPUT", err)
	}
	var activeVersion uuid.UUID
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT active_version_id FROM processing_workflows WHERE id = $1`, created.ID).Scan(&activeVersion)
	}); err != nil {
		t.Fatal(err)
	}
	if activeVersion != first.ID {
		t.Fatalf("active version = %s, want the first %s", activeVersion, first.ID)
	}
}
