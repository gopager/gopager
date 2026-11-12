package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/presentation"
	"github.com/gopager/gopager/internal/readmodel"
)

// cardOf extracts the card of a feed item or detail body.
func cardOf(t *testing.T, item map[string]any) map[string]any {
	t.Helper()
	card, ok := item["card"].(map[string]any)
	if !ok {
		t.Fatalf("item has no card: %v", item)
	}
	return card
}

// TestMessageCardsOverHTTP proves that feed items and details carry the built-in default card,
// rendered from what the caller may read.
func TestMessageCardsOverHTTP(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	unit := f.unit("Engine 1", nil)
	message := f.message(time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond), "FIRE ALARM", unit)
	reader, _ := f.caller([]string{"message.read", "message.detail"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	unitOnly, _ := f.caller([]string{"message.read", "message.detail"}, authz.Scope{Kind: "all"}, []string{"unit"})

	page := f.feed(reader, "", http.StatusOK)
	if len(page.Data.Items) != 1 {
		t.Fatalf("items = %v", page.Data.Items)
	}
	card := cardOf(t, page.Data.Items[0])
	items, _ := json.Marshal(card["items"])
	if _, hasID := card["layoutId"]; hasID || card["fallback"] != false || card["layoutVersion"] != float64(0) {
		t.Fatalf("card = %v, want the built-in default", card)
	}
	if !strings.Contains(string(items), "FIRE ALARM") || !strings.Contains(string(items), "Engine 1") {
		t.Fatalf("card items = %s", items)
	}

	status, body := f.get(reader, "/api/v1/messages/"+message.String())
	if status != http.StatusOK {
		t.Fatalf("detail = %d: %s", status, body)
	}
	var detail struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	if detailItems, _ := json.Marshal(cardOf(t, detail.Data)["items"]); string(detailItems) != string(items) {
		t.Fatalf("detail card %s differs from the feed card %s", detailItems, items)
	}

	hidden := cardOf(t, f.feed(unitOnly, "", http.StatusOK).Data.Items[0])
	hiddenItems, _ := json.Marshal(hidden["items"])
	if strings.Contains(string(hiddenItems), "FIRE ALARM") || !strings.Contains(string(hiddenItems), "Engine 1") {
		t.Fatalf("a caller without the payload group must not see the text on the card: %s", hiddenItems)
	}
}

// fixedLayout resolves one stored Layout whatever is asked for.
type fixedLayout struct{ resolved processing.ResolvedLayout }

func (f fixedLayout) ResolveLayout(context.Context, platform.TransactionContext, processing.LayoutRequest) (processing.ResolvedLayout, error) {
	return f.resolved, nil
}

// TestCardAndDeliveryRenderTheSameItems proves that one Layout rendered as json for the card and
// for a Delivery of the same Message to the same reader yields the same items.
func TestCardAndDeliveryRenderTheSameItems(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	layout := f.layoutCreated(f.layoutManager(), layoutBody("json", []any{
		map[string]any{"kind": "field", "field": map[string]any{"namespace": "message", "path": []string{"text"}}},
		map[string]any{"kind": "field", "field": map[string]any{"namespace": "message", "path": []string{"unitNames"}}},
		map[string]any{"kind": "field", "label": "When", "field": map[string]any{"namespace": "message", "path": []string{"time"}}},
		textBlock("end"),
	}))
	f.subCreated(bearer, subInput(instance, map[string]any{"layoutId": layout.ID}))
	message := f.message(f.minuteAgo().Truncate(time.Microsecond), "PARITY", f.unit("Engine 1", nil), f.unit("Engine 2", nil))
	f.messageFact("message.created", message, nil)
	f.fanOutAll()
	f.attempt()
	calls := f.targets.calls()
	if len(calls) != 1 || calls[0].Input.OutputKind != processing.LayoutJSON {
		t.Fatalf("invocations = %+v", calls)
	}

	renderer, err := presentation.NewCardRenderer(fixedLayout{processing.ResolvedLayout{ID: layout.ID, Version: layout.Revision,
		Layout: layout.Definition}}, f.core.fields, delivery.DefaultLayouts())
	if err != nil {
		t.Fatal(err)
	}
	service, err := readmodel.NewService(f.core.store, f.core.resolver, f.core.store, f.core.reads, f.core.reads, f.core.fields,
		readmodel.WithCards(renderer))
	if err != nil {
		t.Fatal(err)
	}
	proof := identity.Proof{Kind: identity.AccessKeyProof, Secret: strings.TrimPrefix(bearer, "Bearer ")}
	items, err := service.Evaluate(t.Context(), proof, readmodel.Filters{}, []uuid.UUID{message})
	if err != nil || len(items) != 1 || items[0].Card == nil {
		t.Fatalf("items = %+v, err = %v", items, err)
	}
	if items[0].Card.Fallback || items[0].Card.LayoutID == nil || *items[0].Card.LayoutID != layout.ID {
		t.Fatalf("card = %+v", items[0].Card)
	}
	if string(items[0].Card.Items) != calls[0].Input.Body {
		t.Fatalf("card items %s differ from the Delivery body %s", items[0].Card.Items, calls[0].Input.Body)
	}
}
