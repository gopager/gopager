package readmodel

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// Card is the GUI card model of a Message: the json rendering of the Layout that applies to the
// Message for the caller. It reads only what the caller's projection holds, so it never widens
// field rights.
type Card struct {
	// LayoutID is the Layout the card was rendered with; absent for the built-in default.
	LayoutID *uuid.UUID `json:"layoutId,omitempty"`
	// LayoutVersion is the revision of the Layout; 0 for the built-in default.
	LayoutVersion int64 `json:"layoutVersion"`
	// LayoutConflict is true when the Units of the Message are assigned different Layouts.
	LayoutConflict bool `json:"layoutConflict"`
	// Fallback is true when the chosen Layout could not render for this caller and the built-in
	// default was rendered instead.
	Fallback bool `json:"fallback"`
	// Items are the ordered json items of the rendering.
	Items json.RawMessage `json:"items"`
}

// CardRenderer renders the cards of Messages as the caller reads them.
type CardRenderer interface {
	// RenderCards returns one card per projection, in order, inside the caller's transaction. A
	// Layout that cannot render yields the built-in default card; only infrastructure failures
	// are returned as errors. Implementations may share resolved Layouts across the call.
	RenderCards(ctx context.Context, tx platform.TransactionContext, projections []DeliveryProjection) ([]*Card, error)
}

// Option configures a Service.
type Option func(*Service)

// WithCards makes every feed item and detail of the Service carry a card rendered by renderer.
func WithCards(renderer CardRenderer) Option {
	return func(s *Service) { s.cards = renderer }
}

// projection builds the Delivery view of a record the way the caller reads it: the feed item,
// the eligible appearances and the readable groups, sorted.
func (v viewer) projection(record FeedRecord, groups []string) DeliveryProjection {
	return withItem(record, v.feedItem(record, false), groups)
}

// withItem builds the Delivery view of a record from its already built feed item.
func withItem(record FeedRecord, item FeedItem, groups []string) DeliveryProjection {
	return DeliveryProjection{Item: item, Units: slices.Clone(record.Units),
		WithoutUnit: slices.Clone(record.WithoutUnit), Groups: groups, ResultVisible: record.ResultVisible}
}

// sortedGroups returns the viewer's readable field groups, sorted. A page computes them once and
// shares them between its projections, which only read them.
func (v viewer) sortedGroups() []string {
	groups := make([]string, 0, len(v.groups))
	for group := range v.groups {
		groups = append(groups, group)
	}
	slices.Sort(groups)
	return groups
}

// attachCards renders the cards of the records and sets them on the items, which are the
// records' feed items in the same order. Without a renderer the items stay as they are.
func (s *Service) attachCards(ctx context.Context, tx platform.TransactionContext, current access,
	records []FeedRecord, items []FeedItem) error {
	if s.cards == nil || len(records) == 0 {
		return nil
	}
	if len(records) != len(items) {
		return platform.NewFailure(platform.CodeInternalError, nil)
	}
	projections := make([]DeliveryProjection, 0, len(records))
	groups := current.viewer.sortedGroups()
	for i, record := range records {
		projections = append(projections, withItem(record, items[i], groups))
	}
	cards, err := s.cards.RenderCards(ctx, tx, projections)
	if err != nil {
		return err
	}
	if len(cards) != len(items) {
		return platform.NewFailure(platform.CodeInternalError, nil)
	}
	for i := range items {
		items[i].Card = cards[i]
	}
	return nil
}

// attachCard renders the card of one record and sets it on its item.
func (s *Service) attachCard(ctx context.Context, tx platform.TransactionContext, current access,
	record FeedRecord, item *FeedItem) error {
	items := []FeedItem{*item}
	if err := s.attachCards(ctx, tx, current, []FeedRecord{record}, items); err != nil {
		return err
	}
	item.Card = items[0].Card
	return nil
}
