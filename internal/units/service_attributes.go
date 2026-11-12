package units

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// Option configures a Service at construction.
type Option func(*Service)

// WithAttributeCatalog makes the Service check Unit-attribute values against the `unit`
// Attribute schemas of catalog. Without it the Service knows no schema, so every attribute entry
// is invalid input.
func WithAttributeCatalog(catalog AttributeCatalog) Option {
	return func(s *Service) { s.attributes = catalog }
}

// unitSchemas locks the schema catalog for the rest of the write transaction and returns the
// `unit` schemas. Without a catalog there are none.
func (s *Service) unitSchemas(ctx context.Context, tx platform.TransactionContext) (AttributeSchemas, error) {
	if s.attributes == nil {
		return AttributeSchemas{}, nil
	}
	if err := s.attributes.LockCatalog(ctx, tx); err != nil {
		return nil, err
	}
	return s.attributes.UnitSchemas(ctx, tx)
}

// checkUnitAttributes validates the attribute entries of a Unit in the Agency. Settings without
// entries need no schema and take no catalog lock.
func (s *Service) checkUnitAttributes(ctx context.Context, tx platform.TransactionContext, settings Settings, agency uuid.UUID) error {
	if settings.Attributes.Empty() {
		return nil
	}
	schemas, err := s.unitSchemas(ctx, tx)
	if err != nil {
		return err
	}
	return settings.Attributes.Check(schemas, &agency)
}

// checkTemplateAttributes validates the attribute entries of a template, which has no Agency.
func (s *Service) checkTemplateAttributes(ctx context.Context, tx platform.TransactionContext, settings Settings) error {
	if settings.Attributes.Empty() {
		return nil
	}
	schemas, err := s.unitSchemas(ctx, tx)
	if err != nil {
		return err
	}
	return settings.Attributes.Check(schemas, nil)
}

// checkDefaultAttributes validates the attribute defaults of an Organization.
func (s *Service) checkDefaultAttributes(ctx context.Context, tx platform.TransactionContext, defaults Defaults) error {
	if defaults.Attributes.Empty() {
		return nil
	}
	schemas, err := s.unitSchemas(ctx, tx)
	if err != nil {
		return err
	}
	return defaults.Attributes.Check(schemas)
}

// bulkAttributeSchemas returns the schemas a bulk update checks its items against, or nil when no
// item carries attribute entries.
func (s *Service) bulkAttributeSchemas(ctx context.Context, tx platform.TransactionContext, input BulkUnitInput) (AttributeSchemas, error) {
	for _, item := range input.Items {
		if item.Settings != nil && !item.Settings.Attributes.Empty() {
			return s.unitSchemas(ctx, tx)
		}
	}
	return nil, nil
}

// checkBulkItem validates the attribute entries of one bulk update item for its Unit's Agency.
func checkBulkItem(schemas AttributeSchemas, item BulkUnitItem, agency uuid.UUID) error {
	if item.Settings == nil || item.Settings.Attributes.Empty() {
		return nil
	}
	if err := item.Settings.Attributes.Check(schemas, &agency); err != nil {
		return authz.ErrInvalidInput
	}
	return nil
}
