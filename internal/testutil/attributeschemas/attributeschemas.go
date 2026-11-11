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

// Insert stores a `message` schema without Agencies and increments the catalog revision. A schema
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
}
