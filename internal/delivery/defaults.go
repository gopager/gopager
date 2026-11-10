package delivery

import (
	"context"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Catalog declares the Delivery field catalog outside any transaction, for validating layouts
// when they are saved. Result fields are typed in a short transaction of their own.
type Catalog struct {
	transactions events.Transactor
	results      ResultSchemas
}

var _ processing.Declarations = Catalog{}

// catalogTimeout bounds the transaction of one result field lookup.
const catalogTimeout = 5 * time.Second

// NewCatalog returns the catalog. It fails with authz.ErrInvalidInput on a missing port.
func NewCatalog(transactions events.Transactor, results ResultSchemas) (Catalog, error) {
	if transactions == nil || results == nil {
		return Catalog{}, authz.ErrInvalidInput
	}
	return Catalog{transactions: transactions, results: results}, nil
}

// FieldSchema implements processing.Declarations.
func (c Catalog) FieldSchema(ref processing.FieldRef) (processing.FieldSchema, error) {
	if spec, ok := staticFields[staticKey(ref)]; ok {
		return spec.schema(), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), catalogTimeout)
	defer cancel()
	var schema processing.FieldSchema
	err := c.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		schema, err = newFieldSet(tx.Context(), tx, c.results).FieldSchema(ref)
		return err
	})
	return schema, err
}

// field returns a reference to a static catalog field.
func field(namespace, name string) *processing.FieldRef {
	return &processing.FieldRef{Namespace: namespace, Path: []string{name}}
}

// defaultBlocks are the blocks of every built-in layout: the event, the Message text or another
// Payload form, its Units, the Gateway state for Gateway facts and the Message time. A field the
// subscriber may not read is absent and renders nothing.
func defaultBlocks() []processing.LayoutBlock {
	return []processing.LayoutBlock{
		{Kind: "field", Field: field("event", "type"), Label: "Event"},
		{Kind: "fallback", Blocks: []processing.LayoutBlock{
			{Kind: "field", Field: field("message", "text")},
			{Kind: "field", Field: field("message", "numeric")},
			{Kind: "field", Field: field("message", "form"), Label: "Form"},
		}},
		{Kind: "field", Field: field("message", "unitNames"), Label: "Units"},
		{Kind: "field", Field: field("gateway", "name"), Label: "Gateway"},
		{Kind: "field", Field: field("gateway", "newState"), Label: "State"},
		{Kind: "field", Field: field("message", "time"), Label: "Time"},
	}
}

// DefaultLayouts returns the built-in layout per output kind, used when a Subscription names no
// layout.
func DefaultLayouts() map[string]processing.Layout {
	layouts := map[string]processing.Layout{}
	for _, kind := range []string{processing.LayoutText, processing.LayoutMarkdown, processing.LayoutJSON} {
		layouts[kind] = processing.Layout{SchemaVersion: processing.LayoutSchemaVersion, OutputKind: kind, Blocks: defaultBlocks()}
	}
	return layouts
}
