package borrowed_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
)

// TestProcessingNewReadSchemaAgencies expects a `unit` schema that no version of the Workflow
// reads (`target.unit.attributes.<key>…`) to be reported with its Agencies, one that an earlier
// version reads to be exempt, and one read only by a version saved before the schema was
// re-created to count as new again.
func TestProcessingNewReadSchemaAgencies(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	repository, err := processingstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	workflow := uuid.New()
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
	for _, key := range []string{"station", "radio"} {
		exec(`INSERT INTO attribute_schemas (key, revision, label, subject, fields, created_by, created_at, updated_at)
			VALUES ($1, 1, 'Unit', 'unit', '{"town":{"kind":"string"}}'::jsonb, $2, now() - interval '1 hour', now())`, key, uuid.New())
	}
	setSchemaAgencies(t, f, "station", f.agencies[1])
	setSchemaAgencies(t, f, "radio", f.agencies[0])
	exec(`INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', now(), now())`, workflow)
	exec(`INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by) VALUES ($1, $2, $3::jsonb, now(), $4)`,
		uuid.New(), workflow, `{"outputs":{"test":{"text":{"field":{"namespace":"target","path":["unit","attributes","radio","town"]}}}}}`, uuid.New())

	read := func() map[string][]uuid.UUID {
		t.Helper()
		var result map[string][]uuid.UUID
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			result, err = repository.NewReadSchemaAgencies(tx.Context(), tx, workflow, []string{"station", "radio", "missing"})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return result
	}
	got := read()
	if len(got) != 1 || !slices.Equal(got["station"], []uuid.UUID{f.agencies[1]}) {
		t.Fatalf("new read references = %v, want only station with Agency B", got)
	}
	exec(`UPDATE attribute_schemas SET created_at = now() + interval '1 hour' WHERE key = 'radio'`)
	if got := read(); len(got) != 2 || !slices.Equal(got["radio"], []uuid.UUID{f.agencies[0]}) {
		t.Fatalf("after re-creating radio: %v, want station and radio", got)
	}
}
