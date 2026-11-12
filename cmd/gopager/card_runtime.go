package main

import (
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/persistence/ent/layoutstore"
	"github.com/gopager/gopager/internal/processing/presentation"
	"github.com/gopager/gopager/internal/readmodel"
)

// composeCardRenderer returns the renderer that gives feed items, details and live updates their
// card: the json rendering of the Layout that applies, with the built-in default as fallback.
func composeCardRenderer(c *core) (readmodel.CardRenderer, error) {
	layouts, err := layoutstore.New(c.store)
	if err != nil {
		return nil, err
	}
	resolver, err := presentation.NewResolver(layouts, delivery.DefaultLayouts())
	if err != nil {
		return nil, err
	}
	return presentation.NewCardRenderer(resolver, c.fields, delivery.DefaultLayouts())
}
