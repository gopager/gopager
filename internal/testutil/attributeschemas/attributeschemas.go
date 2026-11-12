// Package attributeschemas stores Attribute schemas straight into a test database, so that
// fixtures can reference them from Workflow outputs without going through the HTTP API.
package attributeschemas

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/attributestore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// Insert stores a `message` schema assigned to every existing Agency and increments the catalog revision. A schema
// with the key that already exists is left as it is.
func Insert(t testing.TB, store *postgres.Store, key string, fields map[string]attributes.Field) {
	t.Helper()
	schemas, err := attributestore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	schema := attributes.Schema{Key: key, Label: key, Subject: attributes.SubjectMessage, Fields: fields,
		Agencies: []uuid.UUID{}, Revision: 1, CreatedAt: now, UpdatedAt: now}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return schemas.Insert(tx.Context(), tx, schema, uuid.New())
	})
	if err != nil && !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatal(err)
	}
	// Referencing a schema needs the right in one of its Agencies, so the fixture belongs to
	// every existing Agency.
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO attribute_schema_agencies (schema_key, agency_id)
			SELECT $1, id FROM agencies ON CONFLICT DO NOTHING`, key)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
