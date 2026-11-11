package borrowed_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// TestAgencyDeletionKeepsEveryDefinitionAssigned expects an Agency that is the only Agency of an
// undeleted Workflow, a Layout or an Attribute schema to refuse deletion, and an Agency shared
// with another Agency (or held only by a soft-deleted Workflow) to be deleted together with its
// assignment rows.
func TestAgencyDeletionKeepsEveryDefinitionAssigned(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
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
			t.Fatalf("%s: %v", query, err)
		}
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), query, args...).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	agency := func(name string) commands.Result {
		t.Helper()
		created, err := f.units.CreateAgency(t.Context(), f.proof, uuid.New(), units.AgencyInput{Name: name}, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	deleteAgency := func(value commands.Result) error {
		_, err := f.units.DeleteAgency(t.Context(), f.proof, value.ID, value.Revision, checkedMutation)
		return err
	}
	workflow := func(deleted bool, agencies ...uuid.UUID) {
		id := uuid.New()
		exec(`INSERT INTO processing_workflows (id, name, deleted, created_at, updated_at) VALUES ($1, 'w', $2, now(), now())`, id, deleted)
		for _, a := range agencies {
			exec(`INSERT INTO processing_workflow_agencies (workflow_id, agency_id) VALUES ($1, $2)`, id, a)
		}
	}
	layout := func(agencies ...uuid.UUID) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at)
			VALUES ($1, 1, 'l', 'text', $2, now(), now())`, id, uuid.New())
		for _, a := range agencies {
			exec(`INSERT INTO layout_definition_agencies (layout_id, agency_id) VALUES ($1, $2)`, id, a)
		}
		return id
	}
	schema := func(key string, agencies ...uuid.UUID) {
		exec(`INSERT INTO attribute_schemas (key, revision, label, subject, fields, created_by, created_at, updated_at)
			VALUES ($1, 1, 'S', 'message', '{"a":{"kind":"string"}}', $2, now(), now())`, key, uuid.New())
		for _, a := range agencies {
			exec(`INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ($1, $2)`, key, a)
		}
	}

	sole := map[string]func(uuid.UUID){
		"workflow": func(a uuid.UUID) { workflow(false, a) },
		"layout":   func(a uuid.UUID) { layout(a) },
		"schema":   func(a uuid.UUID) { schema("sole-"+a.String()[:8], a) },
	}
	for name, assign := range sole {
		only := agency("Only " + name)
		assign(only.ID)
		if err := deleteAgency(only); !errors.Is(err, units.ErrAgencyInUse) {
			t.Fatalf("deleting the only Agency of a %s: %v, want in use", name, err)
		}
	}

	shared, other := agency("Shared"), agency("Other")
	workflow(false, shared.ID, other.ID)
	workflow(true, shared.ID)
	sharedLayout := layout(shared.ID, other.ID)
	schema("shared", shared.ID, other.ID)
	if err := deleteAgency(shared); err != nil {
		t.Fatalf("deleting a shared Agency: %v", err)
	}
	if n := count(`SELECT count(*) FROM processing_workflow_agencies WHERE agency_id = $1`, shared.ID) +
		count(`SELECT count(*) FROM layout_definition_agencies WHERE agency_id = $1`, shared.ID) +
		count(`SELECT count(*) FROM attribute_schema_agencies WHERE agency_id = $1`, shared.ID); n != 0 {
		t.Fatalf("%d assignment rows of the deleted Agency remain", n)
	}
	if n := count(`SELECT count(*) FROM layout_definition_agencies WHERE layout_id = $1`, sharedLayout); n != 1 {
		t.Fatalf("shared Layout keeps %d Agencies, want 1", n)
	}
}
