package borrowed_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
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

// setSchemaAgencies replaces the Agencies the Attribute schema key is assigned to.
func setSchemaAgencies(t *testing.T, f matcherFixture, key string, agencies ...uuid.UUID) {
	t.Helper()
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `DELETE FROM attribute_schema_agencies WHERE schema_key = $1`, key); err != nil {
			return err
		}
		for _, agency := range agencies {
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ($1, $2)`,
				key, agency); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestProcessingOutputReferenceNeedsTheRightInOneSchemaAgency expects an author restricted to
// Agency A to reference a schema only when it is assigned to A as well, and an existing
// reference to stay valid when the schema later loses A.
func TestProcessingOutputReferenceNeedsTheRightInOneSchemaAgency(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	service, _ := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	attributeschemas.Insert(t, f.store, "test", map[string]attributes.Field{
		"out": {Kind: processing.BooleanValue}, "text": {Kind: processing.StringValue}, "mode": {Kind: processing.StringValue}})
	actions := []string{"workflow.read", "workflow.create", "workflow.update", "workflow.activate"}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: actions,
		Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}}}})
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("author-scope", f.agencyScope()), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	truth, err := processing.Literal(processing.BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	writing := preparedProcessingDefinition(t)
	writing.Outputs = map[string]map[string]processing.Assignment{"test": {"out": {Literal: &truth}}}
	save := func(revision int64) (uuid.UUID, int64, error) {
		result, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, revision, writing, checkedMutation)
		return result.ID, result.Revision, err
	}

	setSchemaAgencies(t, f, "test", f.agencies[1])
	if _, _, err := save(1); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("author of Agency A referenced a schema of Agency B only: %v, want FORBIDDEN", err)
	}
	setSchemaAgencies(t, f, "test", f.agencies[0], f.agencies[1])
	_, revision, err := save(1)
	if err != nil {
		t.Fatalf("author of Agency A referenced a schema of A and B: %v", err)
	}
	setSchemaAgencies(t, f, "test", f.agencies[1])
	if _, _, err := save(revision); err != nil {
		t.Fatalf("re-saving an existing reference after the schema lost A: %v", err)
	}
}

// TestProcessingDeletedAndRecreatedSchemaIsANewReference expects a reference held by an earlier
// version to be exempt from the Agency check only for the schema that existed when that version
// was saved: after the schema was deleted and re-created for Agency B only, an author of Agency
// A cannot save a version that writes it.
func TestProcessingDeletedAndRecreatedSchemaIsANewReference(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	service, _ := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	fields := map[string]attributes.Field{"out": {Kind: processing.BooleanValue},
		"text": {Kind: processing.StringValue}, "mode": {Kind: processing.StringValue}}
	attributeschemas.Insert(t, f.store, "test", fields)
	actions := []string{"workflow.read", "workflow.create", "workflow.update", "workflow.activate"}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: actions,
		Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}}}})
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("recreated", f.agencyScope()), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	truth, err := processing.Literal(processing.BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	writing := preparedProcessingDefinition(t)
	writing.Outputs = map[string]map[string]processing.Assignment{"test": {"out": {Literal: &truth}}}
	saved, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1, writing, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `DELETE FROM attribute_schemas WHERE key = 'test'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	attributeschemas.Insert(t, f.store, "test", fields)
	setSchemaAgencies(t, f, "test", f.agencies[1])
	if _, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, saved.Revision, writing, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("referencing a re-created schema of Agency B only: %v, want FORBIDDEN", err)
	}
}
