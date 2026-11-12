package presentation

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// DefaultLayout is the installation default Layout with the output kind it applies to. The zero
// value (ID uuid.Nil) means that no default is set.
type DefaultLayout struct {
	ID         uuid.UUID
	OutputKind string
}

// LayerStore answers the questions the layered resolver asks the persisted layers. Every method
// runs in the caller's transaction.
type LayerStore interface {
	// UnitLayouts returns, for the Units among unitIDs that exist and are not deleted, the
	// Layout each one's layers choose for the output kind: its Unit assignment, else its
	// Organization's assignment, where an assignment counts only while its Layout is still
	// assigned to the Unit's Agency. Units without a value are absent.
	UnitLayouts(ctx context.Context, tx platform.TransactionContext, unitIDs []uuid.UUID, outputKind string) (map[uuid.UUID]uuid.UUID, error)
	// InstallationDefault returns the installation default Layout, or the zero value.
	InstallationDefault(ctx context.Context, tx platform.TransactionContext) (DefaultLayout, error)
}

// LayeredResolver chooses the Layout of a rendering through the layers Unit, Organization,
// installation default and built-in default. An explicitly requested Layout bypasses the layers.
// It implements processing.LayoutResolver, runs in the caller's transaction and does no
// authorization.
type LayeredResolver struct {
	inner  *Resolver
	layers LayerStore
}

var _ processing.LayoutResolver = (*LayeredResolver)(nil)

// NewLayeredResolver wraps the resolver that loads Layouts and the built-in defaults. It fails
// with authz.ErrInvalidInput on a nil dependency.
func NewLayeredResolver(inner *Resolver, layers LayerStore) (*LayeredResolver, error) {
	if inner == nil || layers == nil {
		return nil, authz.ErrInvalidInput
	}
	return &LayeredResolver{inner: inner, layers: layers}, nil
}

// ResolveLayout returns the explicitly requested Layout unchanged. Otherwise the Units of the
// request decide for the output kind: when every Unit that has a layer value has the same Layout,
// that Layout applies at its current version; when they differ, the installation default applies
// (when its output kind matches, else the built-in default) and the result carries Conflict, so
// that neither order nor a combination ever chooses. Units without a value do not veto. With no
// value at all, the installation default applies when its output kind matches, else the
// built-in default. Errors are those of Resolver.ResolveLayout.
func (r *LayeredResolver) ResolveLayout(ctx context.Context, tx platform.TransactionContext, request processing.LayoutRequest) (processing.ResolvedLayout, error) {
	if request.LayoutID != nil || request.OutputKind == "" {
		return r.inner.ResolveLayout(ctx, tx, request)
	}
	chosen, err := r.unitChoice(ctx, tx, request)
	if err != nil {
		return processing.ResolvedLayout{}, err
	}
	conflict := len(chosen) > 1
	if len(chosen) == 1 {
		return r.load(ctx, tx, chosen[0], request.OutputKind, false)
	}
	installation, err := r.layers.InstallationDefault(ctx, tx)
	if err != nil {
		return processing.ResolvedLayout{}, err
	}
	if installation.ID != uuid.Nil && installation.OutputKind == request.OutputKind {
		return r.load(ctx, tx, installation.ID, request.OutputKind, conflict)
	}
	resolved, err := r.inner.ResolveLayout(ctx, tx, processing.LayoutRequest{OutputKind: request.OutputKind})
	resolved.Conflict = conflict
	return resolved, err
}

// unitChoice returns the distinct Layouts the Units' layers choose, sorted by ID.
func (r *LayeredResolver) unitChoice(ctx context.Context, tx platform.TransactionContext, request processing.LayoutRequest) ([]uuid.UUID, error) {
	if len(request.UnitIDs) == 0 {
		return nil, nil
	}
	values, err := r.layers.UnitLayouts(ctx, tx, request.UnitIDs, request.OutputKind)
	if err != nil {
		return nil, err
	}
	distinct := make([]uuid.UUID, 0, len(values))
	for _, layout := range values {
		distinct = append(distinct, layout)
	}
	slices.SortFunc(distinct, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return slices.Compact(distinct), nil
}

func (r *LayeredResolver) load(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, outputKind string, conflict bool) (processing.ResolvedLayout, error) {
	resolved, err := r.inner.ResolveLayout(ctx, tx, processing.LayoutRequest{LayoutID: &id, OutputKind: outputKind})
	resolved.Conflict = conflict
	return resolved, err
}

// CheckUse reports, as authz.ErrInvalidInput, that the Layout does not exist or that the
// authority may not use it, like Resolver.CheckUse.
func (r *LayeredResolver) CheckUse(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	layoutID uuid.UUID, now time.Time) error {
	return r.inner.CheckUse(ctx, tx, authority, layoutID, now)
}
