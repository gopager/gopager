package presentation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// stubRegistry serves the built-in field registry.
type stubRegistry struct{ err error }

func (s stubRegistry) Registry(context.Context, platform.TransactionContext) (*fields.Registry, error) {
	if s.err != nil {
		return nil, s.err
	}
	return fields.New(fields.Builtin()), nil
}

// stubLayouts resolves a fixed result or error and records the requests.
type stubLayouts struct {
	resolved processing.ResolvedLayout
	err      error
	requests []processing.LayoutRequest
}

func (s *stubLayouts) ResolveLayout(_ context.Context, _ platform.TransactionContext, request processing.LayoutRequest) (processing.ResolvedLayout, error) {
	s.requests = append(s.requests, request)
	return s.resolved, s.err
}

func cardRenderer(t *testing.T, layouts processing.LayoutResolver) *CardRenderer {
	t.Helper()
	renderer, err := NewCardRenderer(layouts, stubRegistry{}, delivery.DefaultLayouts())
	if err != nil {
		t.Fatal(err)
	}
	return renderer
}

func jsonLayout(blocks ...processing.LayoutBlock) processing.Layout {
	return processing.Layout{SchemaVersion: processing.LayoutSchemaVersion, OutputKind: processing.LayoutJSON, Blocks: blocks}
}

func projectionOf(text string, groups ...string) readmodel.DeliveryProjection {
	payload := json.RawMessage(`{"form":"text","value":` + quote(text) + `}`)
	protocol := "pocsag"
	return readmodel.DeliveryProjection{
		Item: readmodel.FeedItem{MessageID: uuid.New(), MessageTime: time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC),
			Payload: payload, Protocol: &protocol},
		Units:  []readmodel.UnitRecord{{UnitID: uuid.New(), Name: "Engine 1"}},
		Groups: groups,
	}
}

func quote(value string) string {
	out, _ := json.Marshal(value)
	return string(out)
}

func renderOne(t *testing.T, renderer *CardRenderer, projection readmodel.DeliveryProjection) *readmodel.Card {
	t.Helper()
	cards, err := renderer.RenderCards(context.Background(), fakeTx{context.Background()}, []readmodel.DeliveryProjection{projection})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0] == nil {
		t.Fatalf("cards = %v", cards)
	}
	return cards[0]
}

func TestCardUsesRegistryLabelsAndIcons(t *testing.T) {
	t.Parallel()
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{Layout: jsonLayout(
		processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: "message", Path: []string{"time"}}})}}
	card := renderOne(t, cardRenderer(t, layouts), projectionOf("FIRE", "payload", "unit"))
	registry, _ := stubRegistry{}.Registry(context.Background(), nil)
	label, _, ok := registry.Declarations(fields.ScopeDelivery, false).FieldPresentation(processing.FieldRef{Namespace: "message", Path: []string{"time"}})
	if !ok || label == "" {
		t.Fatalf("registry has no label for message.time")
	}
	if !strings.Contains(string(card.Items), `"label":`+quote(label)) {
		t.Fatalf("items = %s, want label %q from the registry", card.Items, label)
	}
	if card.Fallback {
		t.Fatal("a rendering Layout is not a fallback")
	}
}

func TestCardRendersUnreadableFieldAbsent(t *testing.T) {
	t.Parallel()
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{Layout: jsonLayout(
		processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: "message", Path: []string{"text"}}},
		processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: "message", Path: []string{"unitNames"}}})}}
	renderer := cardRenderer(t, layouts)
	hidden := renderOne(t, renderer, projectionOf("FIRE SECRET", "unit"))
	if strings.Contains(string(hidden.Items), "SECRET") || !strings.Contains(string(hidden.Items), "Engine 1") {
		t.Fatalf("items = %s: the payload group is unreadable, the unit group is readable", hidden.Items)
	}
	shown := renderOne(t, renderer, projectionOf("FIRE SECRET", "payload", "unit"))
	if !strings.Contains(string(shown.Items), "SECRET") {
		t.Fatalf("items = %s, want the payload text", shown.Items)
	}
}

func TestCardRendersNoEventFields(t *testing.T) {
	t.Parallel()
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{Layout: jsonLayout(
		processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: "event", Path: []string{"type"}}},
		processing.LayoutBlock{Kind: "text", Text: "marker"})}}
	card := renderOne(t, cardRenderer(t, layouts), projectionOf("FIRE", "payload", "unit"))
	if strings.Contains(string(card.Items), `"field"`) || !strings.Contains(string(card.Items), "marker") {
		t.Fatalf("items = %s, want only the text block", card.Items)
	}
}

func TestCardOmitsUnsafeLink(t *testing.T) {
	t.Parallel()
	link := processing.LayoutBlock{Kind: "link", Label: "open", URL: &processing.LayoutURL{Template: "{0}",
		Fields: []processing.FieldRef{{Namespace: "message", Path: []string{"text"}}}}}
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{Layout: jsonLayout(link)}}
	card := renderOne(t, cardRenderer(t, layouts), projectionOf("javascript:alert(1)", "payload"))
	if strings.Contains(string(card.Items), "javascript") || strings.Contains(string(card.Items), `"link"`) {
		t.Fatalf("items = %s, want the unsafe link omitted", card.Items)
	}
}

func TestCardFallsBackToTheDefaultWhenTheLayoutCannotRender(t *testing.T) {
	t.Parallel()
	var blocks []processing.LayoutBlock
	for range 200 {
		blocks = append(blocks, processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: "message", Path: []string{"text"}}})
	}
	id := uuid.New()
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{ID: id, Version: 3, Layout: jsonLayout(blocks...)}}
	card := renderOne(t, cardRenderer(t, layouts), projectionOf(strings.Repeat("x", 1000), "payload", "unit"))
	if !card.Fallback || card.LayoutID != nil || card.LayoutVersion != 0 {
		t.Fatalf("card = %+v, want the built-in default marked as fallback", card)
	}
	if !strings.Contains(string(card.Items), "xxx") {
		t.Fatalf("items = %s", card.Items)
	}
}

func TestCardKeepsTheLayoutConflictOnAFallback(t *testing.T) {
	t.Parallel()
	var blocks []processing.LayoutBlock
	for range 200 {
		blocks = append(blocks, processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: "message", Path: []string{"text"}}})
	}
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{Conflict: true, Layout: jsonLayout(blocks...)}}
	card := renderOne(t, cardRenderer(t, layouts), projectionOf(strings.Repeat("x", 1000), "payload", "unit"))
	if !card.Fallback || !card.LayoutConflict {
		t.Fatalf("card = %+v, want a fallback that still reports the Layout conflict", card)
	}
}

func TestCardFallsBackForUnresolvableLayouts(t *testing.T) {
	t.Parallel()
	unknown := jsonLayout(processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: "message", Path: []string{"nothing"}}})
	for name, layouts := range map[string]*stubLayouts{
		"deleted":      {err: authz.ErrNotFound},
		"wrong kind":   {err: authz.ErrInvalidInput},
		"not compiled": {resolved: processing.ResolvedLayout{ID: uuid.New(), Version: 1, Layout: unknown}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			card := renderOne(t, cardRenderer(t, layouts), projectionOf("FIRE", "payload"))
			if !card.Fallback || card.LayoutID != nil {
				t.Fatalf("card = %+v", card)
			}
		})
	}
}

func TestCardInfrastructureErrorsFailTheRead(t *testing.T) {
	t.Parallel()
	boom := platform.NewFailure(platform.CodeStorageUnavailable, errors.New("down"))
	renderer, err := NewCardRenderer(&stubLayouts{err: boom}, stubRegistry{}, delivery.DefaultLayouts())
	if err != nil {
		t.Fatal(err)
	}
	_, err = renderer.RenderCards(context.Background(), fakeTx{context.Background()}, []readmodel.DeliveryProjection{projectionOf("x", "payload")})
	if platform.FailureCode(err) != platform.CodeStorageUnavailable {
		t.Fatalf("err = %v", err)
	}
}

func TestCardResolvesForTheEligibleUnitsAndCarriesTheLayoutIdentity(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{ID: id, Version: 4, Layout: jsonLayout(
		processing.LayoutBlock{Kind: "text", Text: "hello"})}}
	projection := projectionOf("FIRE", "payload", "unit")
	cards, err := cardRenderer(t, layouts).RenderCards(context.Background(), fakeTx{context.Background()},
		[]readmodel.DeliveryProjection{projection, projection})
	if err != nil || len(cards) != 2 {
		t.Fatalf("cards = %v, err = %v", cards, err)
	}
	if cards[0].LayoutID == nil || *cards[0].LayoutID != id || cards[0].LayoutVersion != 4 {
		t.Fatalf("card = %+v", cards[0])
	}
	request := layouts.requests[0]
	if request.OutputKind != processing.LayoutJSON || len(request.UnitIDs) != 1 || request.UnitIDs[0] != projection.Units[0].UnitID {
		t.Fatalf("request = %+v", request)
	}
}

func TestCardReportsALayoutConflictOfTheUnits(t *testing.T) {
	t.Parallel()
	layouts := &stubLayouts{resolved: processing.ResolvedLayout{Conflict: true, Layout: jsonLayout(
		processing.LayoutBlock{Kind: "text", Text: "default"})}}
	card := renderOne(t, cardRenderer(t, layouts), projectionOf("FIRE", "payload", "unit"))
	if !card.LayoutConflict || card.Fallback {
		t.Fatalf("card = %+v, want a conflict without fallback", card)
	}
}

// shortBatch resolves fewer Layouts than requested.
type shortBatch struct{ stubLayouts }

func (s *shortBatch) ResolveLayouts(context.Context, platform.TransactionContext, []processing.LayoutRequest) ([]LayoutResolution, error) {
	return nil, nil
}

func TestCardFailsSafelyWhenABatchLosesRequests(t *testing.T) {
	t.Parallel()
	_, err := cardRenderer(t, &shortBatch{}).RenderCards(context.Background(), fakeTx{context.Background()},
		[]readmodel.DeliveryProjection{projectionOf("FIRE", "payload")})
	if platform.FailureCode(err) != platform.CodeInternalError {
		t.Fatalf("err = %v, want INTERNAL_ERROR", err)
	}
}
