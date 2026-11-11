package attributes

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// Store is the persistence port of Attribute schemas. Every method runs in the caller's
// transaction, and every change of the catalog (Insert, Update, Delete) also increments the
// catalog revision in that transaction.
type Store interface {
	// Insert stores a new schema at revision 1 with its Agencies. A key that exists is
	// authz.ErrRevisionConflict, an unknown Agency authz.ErrInvalidInput.
	Insert(ctx context.Context, tx platform.TransactionContext, schema Schema, by uuid.UUID) error
	// Get returns the schema with its Agencies (ordered by ID), or authz.ErrNotFound.
	Get(ctx context.Context, tx platform.TransactionContext, key string) (Schema, error)
	// Visible reports whether the schema exists and one of its Agencies is visible.
	Visible(ctx context.Context, tx platform.TransactionContext, key string, visibility authz.AgencyVisibility) (bool, error)
	// List returns one page of the visible schemas ordered by key, optionally only those
	// assigned to the Agency, and the total of such schemas.
	List(ctx context.Context, tx platform.TransactionContext, visibility authz.AgencyVisibility, agency *uuid.UUID,
		offset, limit int) ([]Schema, int, error)
	// Update replaces label, description, fields, deprecation and Agencies of the schema at the
	// expected revision and bumps the revision, otherwise authz.ErrRevisionConflict. An unknown
	// Agency is authz.ErrInvalidInput.
	Update(ctx context.Context, tx platform.TransactionContext, schema Schema, expected int64) error
	// Delete removes the schema at the expected revision. A stale revision or a schema that an
	// active Workflow version, a current Layout version, a Subscription filter or a saved view
	// references is authz.ErrRevisionConflict.
	Delete(ctx context.Context, tx platform.TransactionContext, key string, expected int64) error
}

// CatalogSource loads every schema together with the catalog revision, which changes with every
// create, update and delete. Schema metadata is not confidential, so the result is not
// filtered by Agency; callers cache it by revision.
type CatalogSource interface {
	Catalog(ctx context.Context, tx platform.TransactionContext) ([]Schema, int64, error)
	// CatalogRevision returns the current catalog revision without loading the schemas.
	CatalogRevision(ctx context.Context, tx platform.TransactionContext) (int64, error)
}
