package borrowed_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

// TestConcurrentAgencyDeletionsKeepTheDefinitionAssigned expects two concurrent deletions of the
// two only Agencies of one definition to serialize: the second waits for the first, re-checks the
// committed state and is refused, so the definition keeps one Agency.
func TestConcurrentAgencyDeletionsKeepTheDefinitionAssigned(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	create := func(name string) commands.Result {
		t.Helper()
		created, err := f.units.CreateAgency(t.Context(), f.proof, uuid.New(), units.AgencyInput{Name: name}, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	first, second := create("First"), create("Second")
	layoutID := uuid.New()
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at)
			VALUES ($1, 1, 'l', 'text', $2, now(), now())`, layoutID, uuid.New()); err != nil {
			return err
		}
		for _, agencyID := range []uuid.UUID{first.ID, second.ID} {
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO layout_definition_agencies (layout_id, agency_id) VALUES ($1, $2)`, layoutID, agencyID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	remove := func(tx platform.TransactionContext, target commands.Result) error {
		authentication, err := f.resolver.Resolve(tx.Context(), tx, f.proof)
		if err != nil {
			return err
		}
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return f.unitStore.DeleteAgency(tx.Context(), tx, authentication.Authority, units.Agency{ID: target.ID, Revision: target.Revision}, now)
	}

	deleted := make(chan struct{})
	commit := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- f.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
			if err := remove(tx, first); err != nil {
				return err
			}
			close(deleted)
			<-commit
			return nil
		})
	}()
	select {
	case <-deleted:
	case err := <-firstDone:
		t.Fatalf("first deletion: %v", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- f.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
			return remove(tx, second)
		})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second deletion did not wait for the first: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	close(commit)
	if err := <-firstDone; err != nil {
		t.Fatalf("first deletion: %v", err)
	}
	if err := <-secondDone; !errors.Is(err, units.ErrAgencyInUse) {
		t.Fatalf("second deletion: %v, want in use", err)
	}
	var remaining int
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM layout_definition_agencies WHERE layout_id = $1 AND agency_id = $2`,
			layoutID, second.ID).Scan(&remaining)
	}); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("definition keeps %d assignments to the second Agency, want 1", remaining)
	}
}
