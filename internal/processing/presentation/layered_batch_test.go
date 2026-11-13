package presentation

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// countingLayers wraps fake layer answers and counts the reads.
type countingLayers struct {
	values       map[uuid.UUID]uuid.UUID
	def          DefaultLayout
	unitCalls    int
	askedUnits   []uuid.UUID
	defaultCalls int
}

func (c *countingLayers) UnitLayouts(_ context.Context, _ platform.TransactionContext, unitIDs []uuid.UUID, _ string) (map[uuid.UUID]uuid.UUID, error) {
	c.unitCalls++
	c.askedUnits = append(c.askedUnits, unitIDs...)
	result := map[uuid.UUID]uuid.UUID{}
	for _, id := range unitIDs {
		if layout, ok := c.values[id]; ok {
			result[id] = layout
		}
	}
	return result, nil
}

func (c *countingLayers) InstallationDefault(context.Context, platform.TransactionContext) (DefaultLayout, error) {
	c.defaultCalls++
	return c.def, nil
}

// countingStore counts Layout loads by ID.
type countingStore struct {
	Store
	loads map[uuid.UUID]int
}

func (s *countingStore) Get(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (Record, error) {
	s.loads[id]++
	return s.Store.Get(ctx, tx, id)
}

// batchScene holds json Layouts, three Units (two with a layer value) and a counting resolver.
type batchScene struct {
	layers           *countingLayers
	store            *countingStore
	resolver         *LayeredResolver
	units            [3]uuid.UUID
	layoutA, layoutB uuid.UUID
}

func newBatchScene(t *testing.T) *batchScene {
	t.Helper()
	memory := newMemoryStore()
	s := &batchScene{layers: &countingLayers{values: map[uuid.UUID]uuid.UUID{}}, layoutA: uuid.New(), layoutB: uuid.New()}
	s.store = &countingStore{Store: memory, loads: map[uuid.UUID]int{}}
	for _, id := range []uuid.UUID{s.layoutA, s.layoutB} {
		definition, err := encode(jsonLayout(processing.LayoutBlock{Kind: "text", Text: id.String()}))
		if err != nil {
			t.Fatal(err)
		}
		if err := memory.Insert(context.Background(), nil, Record{ID: id, Revision: 1, OutputKind: processing.LayoutJSON, Definition: definition}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range s.units {
		s.units[i] = uuid.New()
	}
	s.layers.values[s.units[0]], s.layers.values[s.units[1]] = s.layoutA, s.layoutB
	s.layers.def = DefaultLayout{ID: s.layoutA, OutputKind: processing.LayoutJSON}
	inner, err := NewResolver(s.store, delivery.DefaultLayouts())
	if err != nil {
		t.Fatal(err)
	}
	if s.resolver, err = NewLayeredResolver(inner, s.layers); err != nil {
		t.Fatal(err)
	}
	return s
}

// requests mixes agreeing, conflicting, valueless and explicit requests over the scene's Units.
func (s *batchScene) requests() []processing.LayoutRequest {
	json := processing.LayoutJSON
	return []processing.LayoutRequest{
		{OutputKind: json, UnitIDs: []uuid.UUID{s.units[0]}},
		{OutputKind: json, UnitIDs: []uuid.UUID{s.units[1]}},
		{OutputKind: json, UnitIDs: []uuid.UUID{s.units[0], s.units[1]}},
		{OutputKind: json, UnitIDs: []uuid.UUID{s.units[2]}},
		{OutputKind: json},
		{OutputKind: json, UnitIDs: []uuid.UUID{s.units[0], s.units[2]}},
		{OutputKind: json, LayoutID: &s.layoutB, UnitIDs: []uuid.UUID{s.units[0]}},
	}
}

func TestResolveLayoutsReadsEachLayerOnceAndMatchesPerItemResolution(t *testing.T) {
	t.Parallel()
	s := newBatchScene(t)
	requests := s.requests()
	got, err := s.resolver.ResolveLayouts(context.Background(), fakeTx{context.Background()}, requests)
	if err != nil {
		t.Fatal(err)
	}
	if s.layers.unitCalls != 1 || s.layers.defaultCalls != 1 {
		t.Fatalf("unit lookups=%d installation reads=%d, want one each", s.layers.unitCalls, s.layers.defaultCalls)
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range s.layers.askedUnits {
		if seen[id] {
			t.Fatalf("Unit %s asked twice", id)
		}
		seen[id] = true
	}
	for id, count := range s.store.loads {
		if count != 1 {
			t.Fatalf("Layout %s loaded %d times, want once", id, count)
		}
	}
	for i, request := range requests {
		want, wantErr := s.resolver.ResolveLayout(context.Background(), fakeTx{context.Background()}, request)
		if !reflect.DeepEqual(got[i].Layout, want) || (got[i].Err != nil) != (wantErr != nil) {
			t.Fatalf("request %d: batch %+v / %v, per item %+v / %v", i, got[i].Layout, got[i].Err, want, wantErr)
		}
	}
}

func TestCardRenderingResolvesLayersOncePerPage(t *testing.T) {
	t.Parallel()
	s := newBatchScene(t)
	projections := make([]readmodel.DeliveryProjection, 0, 12)
	for i := range 12 {
		projection := projectionOf("page", "payload", "unit")
		projection.Units = []readmodel.UnitRecord{{UnitID: s.units[i%3], Name: "Unit"}}
		projections = append(projections, projection)
	}
	renderer := cardRenderer(t, s.resolver)
	cards, err := renderer.RenderCards(context.Background(), fakeTx{context.Background()}, projections)
	if err != nil {
		t.Fatal(err)
	}
	if s.layers.unitCalls != 1 || s.layers.defaultCalls > 1 {
		t.Fatalf("unit lookups=%d installation reads=%d for 12 Messages, want at most one each", s.layers.unitCalls, s.layers.defaultCalls)
	}
	if len(s.layers.askedUnits) != 3 {
		t.Fatalf("asked %d Units, want the 3 distinct ones", len(s.layers.askedUnits))
	}
	for id, count := range s.store.loads {
		if count != 1 {
			t.Fatalf("Layout %s loaded %d times", id, count)
		}
	}
	single := cardRenderer(t, s.resolver)
	for i, projection := range projections {
		want := renderOne(t, single, projection)
		if !reflect.DeepEqual(cards[i], want) {
			t.Fatalf("card %d = %+v, per item %+v", i, cards[i], want)
		}
	}
}
