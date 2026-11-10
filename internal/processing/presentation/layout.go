// Package presentation manages persisted layout definitions: the named, versioned arrangements
// of fields that Subscriptions render Domain events with. Layout structure is validated when it
// is saved; each save appends an immutable version.
package presentation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Layout is a stored layout definition at its current version.
type Layout struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision,string"`
	Name     string    `json:"name"`
	// OutputKind is fixed at creation.
	OutputKind string            `json:"outputKind"`
	Definition processing.Layout `json:"definition"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// LayoutVersion is one immutable version of a layout; its number is the layout revision that
// created it.
type LayoutVersion struct {
	LayoutID   uuid.UUID         `json:"layoutId"`
	Version    int64             `json:"version,string"`
	Definition processing.Layout `json:"definition"`
	CreatedAt  time.Time         `json:"createdAt"`
}

// LayoutInput is the editable content of a layout.
type LayoutInput struct {
	Name       string
	Definition processing.Layout
}

// Record is a stored layout with the canonical JSON of its current version.
type Record struct {
	ID         uuid.UUID
	Revision   int64
	Name       string
	OutputKind string
	// Definition is the canonical definition JSON of the current version.
	Definition []byte
	CreatedBy  uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// VersionRecord is one stored version.
type VersionRecord struct {
	LayoutID   uuid.UUID
	Version    int64
	Definition []byte
	CreatedAt  time.Time
}

// Store is the persistence port of layouts. Every method runs in the caller's transaction.
type Store interface {
	// Insert stores a new layout at revision 1 together with its first version.
	Insert(ctx context.Context, tx platform.TransactionContext, record Record) error
	// Get returns the layout with its current version, or authz.ErrNotFound.
	Get(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (Record, error)
	// List returns one page ordered by name then ID, with current versions, and the total.
	List(ctx context.Context, tx platform.TransactionContext, offset, limit int) ([]Record, int, error)
	// Version returns one stored version, or authz.ErrNotFound.
	Version(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, version int64) (VersionRecord, error)
	// Update sets name and bumps the revision at the expected revision and appends the new
	// version (expected+1); otherwise it returns authz.ErrRevisionConflict.
	Update(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64, name string,
		definition []byte, by uuid.UUID, now time.Time) error
	// Delete removes the layout and its versions at the expected revision. A stale revision or
	// a layout that a subscription references is authz.ErrRevisionConflict.
	Delete(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64) error
}
