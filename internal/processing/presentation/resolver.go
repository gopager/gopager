package presentation

import (
	"context"
	"encoding/json"
	"maps"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Resolver loads the layout a delivery renders with. It implements processing.LayoutResolver,
// runs in the caller's transaction and does no authorization; callers authorize.
type Resolver struct {
	store    Store
	defaults map[string]processing.Layout
}

var _ processing.LayoutResolver = (*Resolver)(nil)

// NewResolver binds the resolver to the store and the built-in default layout per output kind.
// It fails with authz.ErrInvalidInput on a nil store.
func NewResolver(store Store, defaults map[string]processing.Layout) (*Resolver, error) {
	if store == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Resolver{store: store, defaults: maps.Clone(defaults)}, nil
}

// ResolveLayout returns the current version of the requested layout, or the built-in default
// of the requested output kind (ID uuid.Nil, version 0) when no layout is requested. It fails
// with authz.ErrNotFound for a missing layout and authz.ErrInvalidInput for an unknown default
// kind or when the layout's output kind differs from a non-empty requested kind.
func (r *Resolver) ResolveLayout(ctx context.Context, tx platform.TransactionContext, request processing.LayoutRequest) (processing.ResolvedLayout, error) {
	if request.LayoutID == nil {
		layout, ok := r.defaults[request.OutputKind]
		if !ok {
			return processing.ResolvedLayout{}, authz.ErrInvalidInput
		}
		return processing.ResolvedLayout{ID: uuid.Nil, Version: 0, Layout: layout}, nil
	}
	record, err := r.store.Get(ctx, tx, *request.LayoutID)
	if err != nil {
		return processing.ResolvedLayout{}, err
	}
	if request.OutputKind != "" && record.OutputKind != request.OutputKind {
		return processing.ResolvedLayout{}, authz.ErrInvalidInput
	}
	layout, err := processing.DecodeLayout(record.Definition)
	if err != nil {
		return processing.ResolvedLayout{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	return processing.ResolvedLayout{ID: record.ID, Version: record.Revision, Layout: layout}, nil
}

// encode returns the canonical JSON of a layout.
func encode(layout processing.Layout) ([]byte, error) { return json.Marshal(layout) }
