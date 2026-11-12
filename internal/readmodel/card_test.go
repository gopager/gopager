package readmodel

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// recordingRenderer renders a card per projection and remembers what it was asked to render.
type recordingRenderer struct {
	projections []DeliveryProjection
}

func (r *recordingRenderer) RenderCards(_ context.Context, _ platform.TransactionContext, projections []DeliveryProjection) ([]*Card, error) {
	r.projections = append(r.projections, projections...)
	cards := make([]*Card, 0, len(projections))
	for range projections {
		cards = append(cards, &Card{Items: json.RawMessage(`[]`)})
	}
	return cards, nil
}

func TestAttachCardsSetsACardPerItemFromTheCallersProjection(t *testing.T) {
	t.Parallel()
	renderer := &recordingRenderer{}
	service := &Service{cards: renderer}
	records := []FeedRecord{sampleRecord(), sampleRecord()}
	current := access{viewer: viewer{groups: map[string]bool{"unit": true, "payload": true}}}
	items := []FeedItem{current.viewer.feedItem(records[0], false), current.viewer.feedItem(records[1], false)}
	if err := service.attachCards(context.Background(), stubTx{}, current, records, items); err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		if item.Card == nil {
			t.Fatalf("item %d has no card", i)
		}
	}
	if len(renderer.projections) != 2 {
		t.Fatalf("projections = %d", len(renderer.projections))
	}
	got := renderer.projections[0]
	if !slices.Equal(got.Groups, []string{"payload", "unit"}) || len(got.Units) != 1 || len(got.WithoutUnit) != 1 {
		t.Fatalf("projection = %+v", got)
	}
	if got.Item.Card != nil {
		t.Fatal("the projection a card is rendered from must not carry a card")
	}
}

func TestAttachCardsWithoutRendererLeavesItemsAlone(t *testing.T) {
	t.Parallel()
	record := sampleRecord()
	current := access{viewer: viewer{groups: map[string]bool{}}}
	items := []FeedItem{current.viewer.feedItem(record, false)}
	if err := (&Service{}).attachCards(context.Background(), stubTx{}, current, []FeedRecord{record}, items); err != nil {
		t.Fatal(err)
	}
	if items[0].Card != nil {
		t.Fatal("no renderer, no card")
	}
}

func TestCardSerializesWithoutLayoutIDForTheBuiltInDefault(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(Card{Items: json.RawMessage(`[{"kind":"text","text":"x"}]`), Fallback: true})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"layoutVersion":0,"layoutConflict":false,"fallback":true,"items":[{"kind":"text","text":"x"}]}`
	if string(out) != want {
		t.Fatalf("card = %s", out)
	}
}

func TestProjectForDeliveryCarriesNoCard(t *testing.T) {
	t.Parallel()
	record := sampleRecord()
	renderer := &recordingRenderer{}
	service := &Service{store: feedStub{result: FeedResult{Items: []FeedRecord{record}}}, cards: renderer}
	id := uuid.NewString()
	authority := authz.Authority{PrincipalID: id, Revision: 1, Active: true, Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: id,
		SubjectKind: "principal", Actions: []string{"message.read"}, Scope: authz.Scope{Kind: "all"}}}}
	projection, ok, err := service.ProjectForDelivery(context.Background(), stubTx{}, authority, time.Now(), record.MessageID, uuid.Nil)
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if projection.Item.Card != nil || len(renderer.projections) != 0 {
		t.Fatal("a Delivery projection must carry no card")
	}
}
