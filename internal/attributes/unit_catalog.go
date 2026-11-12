package attributes

import (
	"context"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/units"
)

// UnitCatalog adapts a CatalogSource to the units.AttributeCatalog port, so that Unit-attribute
// values are validated against the stored `unit` schemas.
type UnitCatalog struct{ source CatalogSource }

var _ units.AttributeCatalog = UnitCatalog{}

// NewUnitCatalog returns the units.AttributeCatalog backed by source.
func NewUnitCatalog(source CatalogSource) UnitCatalog { return UnitCatalog{source: source} }

// UnitSchemas implements units.AttributeCatalog: it returns the `unit` schemas by key with
// their Agencies and the value type of each top-level field.
func (c UnitCatalog) UnitSchemas(ctx context.Context, tx platform.TransactionContext) (units.AttributeSchemas, error) {
	schemas, _, err := c.source.Catalog(ctx, tx)
	if err != nil {
		return nil, err
	}
	result := make(units.AttributeSchemas)
	for _, schema := range schemas {
		if schema.Subject != SubjectUnit {
			continue
		}
		fields := make(map[string]processing.FieldSchema, len(schema.Fields))
		for name, field := range schema.Fields {
			fields[name] = field.Type()
		}
		result[schema.Key] = units.AttributeSchema{Agencies: schema.Agencies, Fields: fields}
	}
	return result, nil
}

// LockCatalog implements units.AttributeCatalog by holding the shared catalog lock.
func (c UnitCatalog) LockCatalog(ctx context.Context, tx platform.TransactionContext) error {
	return c.source.LockCatalog(ctx, tx)
}
