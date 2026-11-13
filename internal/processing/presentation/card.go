package presentation

import (
	"context"
	"errors"
	"maps"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// RegistrySource supplies the field registry visible in a transaction.
type RegistrySource interface {
	Registry(ctx context.Context, tx platform.TransactionContext) (*fields.Registry, error)
}

// CardRenderer renders the GUI cards of Messages: the json rendering of the Layout that applies
// to a Message, read with the same field reader a Delivery of the Message uses, so card and
// Delivery show the same values, labels and icons. It implements readmodel.CardRenderer, runs in
// the caller's transaction and does no authorization: the projections it is given hold exactly
// what the caller may read.
type CardRenderer struct {
	layouts  processing.LayoutResolver
	registry RegistrySource
	defaults map[string]processing.Layout
}

var _ readmodel.CardRenderer = (*CardRenderer)(nil)

// NewCardRenderer binds the renderer to the Layout resolver, the field registry and the built-in
// default Layout per output kind, which must include the json kind. It fails with
// authz.ErrInvalidInput on a missing dependency.
func NewCardRenderer(layouts processing.LayoutResolver, registry RegistrySource, defaults map[string]processing.Layout) (*CardRenderer, error) {
	if layouts == nil || registry == nil {
		return nil, authz.ErrInvalidInput
	}
	if _, ok := defaults[processing.LayoutJSON]; !ok {
		return nil, authz.ErrInvalidInput
	}
	return &CardRenderer{layouts: layouts, registry: registry, defaults: maps.Clone(defaults)}, nil
}

// programKey identifies a resolved Layout version.
type programKey struct {
	id      uuid.UUID
	version int64
}

// cardProgram is a compiled Layout, or the fact that it could not be compiled.
type cardProgram struct {
	program *processing.LayoutProgram
	failed  bool
}

// cardRun holds what one RenderCards call shares between its projections.
type cardRun struct {
	renderer     *CardRenderer
	tx           platform.TransactionContext
	declarations fields.Declarations
	programs     map[programKey]cardProgram
}

// RenderCards returns one card per projection. A Layout that cannot be resolved, compiled or
// rendered for a Message (a limit, invalid input, a deleted Layout) yields the built-in default
// card marked as fallback, so one bad Layout never hides a Message; any other failure is
// returned. Resolved and compiled Layouts are shared between the projections of one call.
func (r *CardRenderer) RenderCards(ctx context.Context, tx platform.TransactionContext,
	projections []readmodel.DeliveryProjection) ([]*readmodel.Card, error) {
	if len(projections) == 0 {
		return nil, nil
	}
	registry, err := r.registry.Registry(ctx, tx)
	if err != nil {
		return nil, err
	}
	run := &cardRun{renderer: r, tx: tx, declarations: registry.Declarations(fields.ScopeDelivery, true),
		programs: map[programKey]cardProgram{}}
	resolutions, err := r.resolveLayouts(ctx, tx, projections)
	if err != nil {
		return nil, err
	}
	cards := make([]*readmodel.Card, 0, len(projections))
	for i, projection := range projections {
		card, err := run.card(ctx, projection, resolutions[i])
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, nil
}

// resolveLayouts resolves the Layout of every projection. A resolver that can resolve a batch
// reads each layer once for the page; any other resolver is asked per Message.
func (r *CardRenderer) resolveLayouts(ctx context.Context, tx platform.TransactionContext,
	projections []readmodel.DeliveryProjection) ([]LayoutResolution, error) {
	requests := make([]processing.LayoutRequest, len(projections))
	for i, projection := range projections {
		unitIDs := make([]uuid.UUID, 0, len(projection.Units))
		for _, unit := range projection.Units {
			unitIDs = append(unitIDs, unit.UnitID)
		}
		requests[i] = processing.LayoutRequest{OutputKind: processing.LayoutJSON, UnitIDs: unitIDs}
	}
	if batch, ok := r.layouts.(BatchLayoutResolver); ok {
		resolutions, err := batch.ResolveLayouts(ctx, tx, requests)
		if err == nil && len(resolutions) != len(requests) {
			err = platform.NewFailure(platform.CodeInternalError, errors.New("layout batch resolution lost requests"))
		}
		return resolutions, err
	}
	resolutions := make([]LayoutResolution, len(requests))
	for i, request := range requests {
		resolutions[i].Layout, resolutions[i].Err = r.layouts.ResolveLayout(ctx, tx, request)
	}
	return resolutions, nil
}

// card renders one Message, falling back to the built-in default when its Layout cannot render.
func (c *cardRun) card(ctx context.Context, projection readmodel.DeliveryProjection, resolution LayoutResolution) (*readmodel.Card, error) {
	reader := delivery.MessageFieldReader(projection, c.declarations)
	resolved, err := resolution.Layout, resolution.Err
	if err == nil && resolved.Layout.OutputKind == processing.LayoutJSON {
		var card *readmodel.Card
		if card, err = c.render(ctx, resolved, reader); err == nil && card != nil {
			return card, nil
		}
	}
	if err != nil && !renderable(err) {
		return nil, err
	}
	return c.fallback(ctx, reader, resolved.Conflict)
}

// render compiles (once per Layout version) and renders the Layout. It returns a nil card and
// no error for a Layout that does not compile.
func (c *cardRun) render(ctx context.Context, resolved processing.ResolvedLayout, reader processing.FieldReader) (*readmodel.Card, error) {
	key := programKey{id: resolved.ID, version: resolved.Version}
	entry, ok := c.programs[key]
	if !ok {
		program, err := processing.CompileLayout(ctx, resolved.Layout, c.declarations)
		switch {
		case err == nil:
			entry = cardProgram{program: program}
		case renderable(err):
			entry = cardProgram{failed: true}
		default:
			return nil, err
		}
		c.programs[key] = entry
	}
	if entry.failed {
		return nil, nil
	}
	rendered, err := entry.program.Render(ctx, reader)
	if err != nil {
		return nil, err
	}
	card := &readmodel.Card{LayoutVersion: resolved.Version, LayoutConflict: resolved.Conflict, Items: rendered.Body}
	if resolved.ID != uuid.Nil {
		id := resolved.ID
		card.LayoutID = &id
	}
	return card, nil
}

// fallback renders the built-in default json Layout. A default that cannot render either, for
// example because of an oversized Payload, yields an empty card rather than failing the read.
// The conflict flag of the failed resolution is kept, because it describes the Message's Units
// and not the rendering.
func (c *cardRun) fallback(ctx context.Context, reader processing.FieldReader, conflict bool) (*readmodel.Card, error) {
	card, err := c.render(ctx, processing.ResolvedLayout{Layout: c.renderer.defaults[processing.LayoutJSON]}, reader)
	switch {
	case err == nil && card != nil:
	case err == nil || renderable(err):
		card = &readmodel.Card{Items: []byte("[]")}
	default:
		return nil, err
	}
	card.Fallback = true
	card.LayoutConflict = conflict
	return card, nil
}

// renderable reports whether a failure only concerns the Layout or its output, so the built-in
// default may take its place. Everything else is an infrastructure failure.
func renderable(err error) bool {
	if errors.Is(err, authz.ErrNotFound) || errors.Is(err, authz.ErrInvalidInput) {
		return true
	}
	switch platform.FailureCode(err) {
	case platform.CodeResourceLimit, platform.CodeInvalidInput, platform.CodeNotFound:
		return true
	}
	return false
}
