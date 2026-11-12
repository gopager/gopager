package attributestore_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// schemaFacts counts the configuration facts about Attribute schemas.
func (w *world) schemaFacts(t *testing.T) (count int) {
	t.Helper()
	err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events
			WHERE type = 'configuration.changed' AND envelope->'aggregateRef'->>'type' = 'attributeSchema'`).Scan(&count)
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestRemovingAnAgencyFromAUnitSchemaRecordsOneFact(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	other := w.exec(t, func(id uuid.UUID) (string, []any) {
		return `INSERT INTO agencies (id, name) VALUES ($1, 'Other')`, []any{id}
	})
	both := w.input("site", w.agency, other)
	both.Subject = attributes.SubjectUnit
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), both, allow); err != nil {
		t.Fatal(err)
	}
	if got := w.schemaFacts(t); got != 0 {
		t.Fatalf("create recorded %d schema facts", got)
	}
	removing := w.input("site", w.agency)
	removing.Subject = attributes.SubjectUnit
	if _, err := w.manager.Update(ctx, identity.Proof{}, "site", 1, removing, allow); err != nil {
		t.Fatal(err)
	}
	if got := w.schemaFacts(t); got != 1 {
		t.Fatalf("removing an Agency recorded %d schema facts, want 1", got)
	}
	removing.Label = "Renamed"
	if _, err := w.manager.Update(ctx, identity.Proof{}, "site", 2, removing, allow); err != nil {
		t.Fatal(err)
	}
	if got := w.schemaFacts(t); got != 1 {
		t.Fatalf("a label change recorded another schema fact: %d", got)
	}
}
