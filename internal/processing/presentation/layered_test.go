package presentation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// fakeLayers answers the layered resolver's questions from prepared data.
type fakeLayers struct {
	values  map[uuid.UUID]uuid.UUID
	def     DefaultLayout
	asked   []uuid.UUID
	askedAs string
}

func (f *fakeLayers) UnitLayouts(_ context.Context, _ platform.TransactionContext, unitIDs []uuid.UUID, outputKind string) (map[uuid.UUID]uuid.UUID, error) {
	f.asked, f.askedAs = unitIDs, outputKind
	result := map[uuid.UUID]uuid.UUID{}
	for _, id := range unitIDs {
		if layout, ok := f.values[id]; ok {
			result[id] = layout
		}
	}
	return result, nil
}

func (f *fakeLayers) InstallationDefault(context.Context, platform.TransactionContext) (DefaultLayout, error) {
	return f.def, nil
}

// layeredFixture stores one markdown and two text layouts and composes the layered resolver.
type layeredFixture struct {
	layers   *fakeLayers
	resolver *LayeredResolver
	textA    uuid.UUID
	textB    uuid.UUID
	markdown uuid.UUID
}

func newLayeredFixture(t *testing.T) *layeredFixture {
	t.Helper()
	store := newMemoryStore()
	f := &layeredFixture{layers: &fakeLayers{values: map[uuid.UUID]uuid.UUID{}}, textA: uuid.New(), textB: uuid.New(), markdown: uuid.New()}
	for id, kind := range map[uuid.UUID]string{f.textA: "text", f.textB: "text", f.markdown: "markdown"} {
		definition, err := encode(textLayout(kind))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Insert(context.Background(), nil, Record{ID: id, Revision: 1, OutputKind: kind, Definition: definition}); err != nil {
			t.Fatal(err)
		}
	}
	inner, err := NewResolver(store, map[string]processing.Layout{"text": textLayout("text"), "markdown": textLayout("markdown")})
	if err != nil {
		t.Fatal(err)
	}
	if f.resolver, err = NewLayeredResolver(inner, f.layers); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *layeredFixture) resolve(t *testing.T, request processing.LayoutRequest) processing.ResolvedLayout {
	t.Helper()
	resolved, err := f.resolver.ResolveLayout(context.Background(), fakeTx{context.Background()}, request)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestLayeredResolverExplicitLayoutWins(t *testing.T) {
	t.Parallel()
	f := newLayeredFixture(t)
	unit := uuid.New()
	f.layers.values[unit] = f.textB
	got := f.resolve(t, processing.LayoutRequest{LayoutID: &f.textA, OutputKind: "text", UnitIDs: []uuid.UUID{unit}})
	if got.ID != f.textA || got.Conflict || f.layers.asked != nil {
		t.Fatalf("explicit = %+v, asked %v", got, f.layers.asked)
	}
}

func TestLayeredResolverAgreeingUnitsUseTheirLayout(t *testing.T) {
	t.Parallel()
	f := newLayeredFixture(t)
	a, b, none := uuid.New(), uuid.New(), uuid.New()
	f.layers.values[a], f.layers.values[b] = f.textA, f.textA
	f.layers.def = DefaultLayout{ID: f.textB, OutputKind: "text"}
	got := f.resolve(t, processing.LayoutRequest{OutputKind: "text", UnitIDs: []uuid.UUID{a, none, b}})
	if got.ID != f.textA || got.Version != 1 || got.Conflict {
		t.Fatalf("agree = %+v", got)
	}
	if f.layers.askedAs != "text" {
		t.Fatalf("asked for output kind %q", f.layers.askedAs)
	}
}

func TestLayeredResolverDisagreementUsesInstallationDefaultAndFlagsConflict(t *testing.T) {
	t.Parallel()
	f := newLayeredFixture(t)
	a, b := uuid.New(), uuid.New()
	f.layers.values[a], f.layers.values[b] = f.textA, f.textB
	f.layers.def = DefaultLayout{ID: f.markdown, OutputKind: "markdown"}
	// The default has another output kind, so the built-in default of the requested kind applies.
	got := f.resolve(t, processing.LayoutRequest{OutputKind: "text", UnitIDs: []uuid.UUID{a, b}})
	if got.ID != uuid.Nil || !got.Conflict {
		t.Fatalf("conflict without matching default = %+v", got)
	}
	f.layers.def = DefaultLayout{ID: f.textA, OutputKind: "text"}
	got = f.resolve(t, processing.LayoutRequest{OutputKind: "text", UnitIDs: []uuid.UUID{b, a}})
	if got.ID != f.textA || !got.Conflict {
		t.Fatalf("conflict = %+v", got)
	}
}

func TestLayeredResolverWithoutUnitValuesFallsBack(t *testing.T) {
	t.Parallel()
	f := newLayeredFixture(t)
	unit := uuid.New()
	cases := []struct {
		name string
		def  DefaultLayout
		want uuid.UUID
	}{
		{"installation default of the kind", DefaultLayout{ID: f.textB, OutputKind: "text"}, f.textB},
		{"installation default of another kind", DefaultLayout{ID: f.markdown, OutputKind: "markdown"}, uuid.Nil},
		{"no installation default", DefaultLayout{}, uuid.Nil},
	}
	for _, c := range cases {
		f.layers.def = c.def
		got := f.resolve(t, processing.LayoutRequest{OutputKind: "text", UnitIDs: []uuid.UUID{unit}})
		if got.ID != c.want || got.Conflict {
			t.Fatalf("%s = %+v", c.name, got)
		}
	}
	f.layers.def = DefaultLayout{ID: f.textB, OutputKind: "text"}
	if got := f.resolve(t, processing.LayoutRequest{OutputKind: "text"}); got.ID != f.textB {
		t.Fatalf("no units = %+v", got)
	}
}

func TestLayeredResolverRejectsUnknownKindWithoutLayout(t *testing.T) {
	t.Parallel()
	f := newLayeredFixture(t)
	_, err := f.resolver.ResolveLayout(context.Background(), fakeTx{context.Background()}, processing.LayoutRequest{OutputKind: "json"})
	if !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("unknown default kind: %v", err)
	}
}

func TestNewLayeredResolverRequiresDependencies(t *testing.T) {
	t.Parallel()
	if _, err := NewLayeredResolver(nil, &fakeLayers{}); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("nil inner: %v", err)
	}
	if _, err := NewLayeredResolver(&Resolver{}, nil); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("nil layers: %v", err)
	}
}
