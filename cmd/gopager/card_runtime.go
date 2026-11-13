package main

import (
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/persistence/ent/layoutstore"
	"github.com/gopager/gopager/internal/persistence/ent/presentationstore"
	"github.com/gopager/gopager/internal/processing/presentation"
	"github.com/gopager/gopager/internal/readmodel"
)

// layoutComposition is the Layout persistence and the layered resolver of one renderer of
// Messages. Every renderer builds it with composeLayouts; the resolvers hold no state, so a GUI
// card and a Delivery choose the same Layout for the same Units (a Delivery split by Unit asks
// for its one Unit, a card for the caller's eligible Units).
type layoutComposition struct {
	layouts  *layoutstore.Store
	layers   *presentationstore.Store
	resolver *presentation.LayeredResolver
}

// composeLayouts builds the Layout stores and the layered resolver over the built-in defaults.
func composeLayouts(c *core) (layoutComposition, error) {
	layouts, err := layoutstore.New(c.store)
	if err != nil {
		return layoutComposition{}, err
	}
	layers, err := presentationstore.New(c.store)
	if err != nil {
		return layoutComposition{}, err
	}
	base, err := presentation.NewResolver(layouts, delivery.DefaultLayouts())
	if err != nil {
		return layoutComposition{}, err
	}
	resolver, err := presentation.NewLayeredResolver(base, layers)
	if err != nil {
		return layoutComposition{}, err
	}
	return layoutComposition{layouts: layouts, layers: layers, resolver: resolver}, nil
}

// composeCardRenderer returns the renderer that gives feed items, details and live updates their
// card: the json rendering of the Layout the layers choose, with the built-in default as fallback.
func composeCardRenderer(c *core) (readmodel.CardRenderer, error) {
	composition, err := composeLayouts(c)
	if err != nil {
		return nil, err
	}
	return presentation.NewCardRenderer(composition.resolver, c.fields, delivery.DefaultLayouts())
}
