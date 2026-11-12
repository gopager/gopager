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

// LayoutResolution is the outcome of one request of a batch: the resolved Layout or the error
// ResolveLayout would have returned for it.
type LayoutResolution struct {
	Layout processing.ResolvedLayout
	Err    error
}

// BatchLayoutResolver resolves the Layouts of many renderings at once.
type BatchLayoutResolver interface {
	// ResolveLayouts returns one resolution per request, in order. A failure that concerns a
	// single request is that request's Err; a failure of the layer reads is returned as the
	// error.
	ResolveLayouts(ctx context.Context, tx platform.TransactionContext, requests []processing.LayoutRequest) ([]LayoutResolution, error)
}

var _ BatchLayoutResolver = (*LayeredResolver)(nil)

// ResolveLayout returns the explicitly requested Layout unchanged. Otherwise the Units of the
// request decide for the output kind: when every Unit that has a layer value has the same Layout,
// that Layout applies at its current version; when they differ, the installation default applies
// (when its output kind matches, else the built-in default) and the result carries Conflict, so
// that neither order nor a combination ever chooses. Units without a value do not veto. With no
// value at all, the installation default applies when its output kind matches, else the
// built-in default. Errors are those of Resolver.ResolveLayout.
func (r *LayeredResolver) ResolveLayout(ctx context.Context, tx platform.TransactionContext, request processing.LayoutRequest) (processing.ResolvedLayout, error) {
	resolutions, err := r.ResolveLayouts(ctx, tx, []processing.LayoutRequest{request})
	if err != nil {
		return processing.ResolvedLayout{}, err
	}
	return resolutions[0].Layout, resolutions[0].Err
}

// ResolveLayouts applies the rules of ResolveLayout to every request and reads each layer once
// for the whole batch: the Unit layers of all distinct Units per output kind in one lookup, the
// installation default once, and every distinct Layout once. ResolveLayout is the one-request
// case, so both always agree.
func (r *LayeredResolver) ResolveLayouts(ctx context.Context, tx platform.TransactionContext, requests []processing.LayoutRequest) ([]LayoutResolution, error) {
	batch := &layoutBatch{resolver: r, ctx: ctx, tx: tx,
		units: map[string]map[uuid.UUID]uuid.UUID{}, loaded: map[loadKey]LayoutResolution{}}
	if err := batch.lookUpUnits(requests); err != nil {
		return nil, err
	}
	resolutions := make([]LayoutResolution, len(requests))
	for i, request := range requests {
		layout, err := batch.resolve(request)
		if err != nil {
			return nil, err
		}
		resolutions[i] = layout
	}
	return resolutions, nil
}

// loadKey identifies a Layout load; an unset id is the built-in default of the output kind.
type loadKey struct {
	id         uuid.UUID
	outputKind string
}

// layoutBatch holds what one ResolveLayouts call shares between its requests.
type layoutBatch struct {
	resolver     *LayeredResolver
	ctx          context.Context
	tx           platform.TransactionContext
	units        map[string]map[uuid.UUID]uuid.UUID
	installation *DefaultLayout
	loaded       map[loadKey]LayoutResolution
}

// lookUpUnits reads the layer values of every distinct Unit of the requests that the layers
// decide, once per output kind.
func (b *layoutBatch) lookUpUnits(requests []processing.LayoutRequest) error {
	wanted := map[string]map[uuid.UUID]bool{}
	for _, request := range requests {
		if request.LayoutID != nil || request.OutputKind == "" {
			continue
		}
		if wanted[request.OutputKind] == nil {
			wanted[request.OutputKind] = map[uuid.UUID]bool{}
		}
		for _, id := range request.UnitIDs {
			wanted[request.OutputKind][id] = true
		}
	}
	for kind, set := range wanted {
		if len(set) == 0 {
			continue
		}
		ids := make([]uuid.UUID, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, func(a, c uuid.UUID) int { return slices.Compare(a[:], c[:]) })
		values, err := b.resolver.layers.UnitLayouts(b.ctx, b.tx, ids, kind)
		if err != nil {
			return err
		}
		b.units[kind] = values
	}
	return nil
}

func (b *layoutBatch) installationDefault() (DefaultLayout, error) {
	if b.installation == nil {
		installation, err := b.resolver.layers.InstallationDefault(b.ctx, b.tx)
		if err != nil {
			return DefaultLayout{}, err
		}
		b.installation = &installation
	}
	return *b.installation, nil
}

// load resolves a Layout (or the built-in default of the kind) once per batch.
func (b *layoutBatch) load(id uuid.UUID, outputKind string, conflict bool) LayoutResolution {
	key := loadKey{id: id, outputKind: outputKind}
	resolution, ok := b.loaded[key]
	if !ok {
		request := processing.LayoutRequest{OutputKind: outputKind}
		if id != uuid.Nil {
			request.LayoutID = &id
		}
		resolution.Layout, resolution.Err = b.resolver.inner.ResolveLayout(b.ctx, b.tx, request)
		b.loaded[key] = resolution
	}
	resolution.Layout.Conflict = conflict
	return resolution
}

func (b *layoutBatch) resolve(request processing.LayoutRequest) (LayoutResolution, error) {
	if request.LayoutID != nil {
		return b.load(*request.LayoutID, request.OutputKind, false), nil
	}
	if request.OutputKind == "" {
		layout, err := b.resolver.inner.ResolveLayout(b.ctx, b.tx, request)
		return LayoutResolution{Layout: layout, Err: err}, nil
	}
	chosen := b.unitChoice(request)
	if len(chosen) == 1 {
		return b.load(chosen[0], request.OutputKind, false), nil
	}
	conflict := len(chosen) > 1
	installation, err := b.installationDefault()
	if err != nil {
		return LayoutResolution{}, err
	}
	if installation.ID != uuid.Nil && installation.OutputKind == request.OutputKind {
		return b.load(installation.ID, request.OutputKind, conflict), nil
	}
	return b.load(uuid.Nil, request.OutputKind, conflict), nil
}

// unitChoice returns the distinct Layouts the request's Units' layers choose, sorted by ID.
func (b *layoutBatch) unitChoice(request processing.LayoutRequest) []uuid.UUID {
	var distinct []uuid.UUID
	for _, id := range request.UnitIDs {
		if layout, ok := b.units[request.OutputKind][id]; ok {
			distinct = append(distinct, layout)
		}
	}
	slices.SortFunc(distinct, func(a, c uuid.UUID) int { return slices.Compare(a[:], c[:]) })
	return slices.Compact(distinct)
}

// CheckUse reports, as authz.ErrInvalidInput, that the Layout does not exist or that the
// authority may not use it, like Resolver.CheckUse.
func (r *LayeredResolver) CheckUse(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	layoutID uuid.UUID, now time.Time) error {
	return r.inner.CheckUse(ctx, tx, authority, layoutID, now)
}
