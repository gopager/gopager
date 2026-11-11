package readmodel

import (
	"context"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// FieldRegistry supplies the field registry that predicates are checked against. The registry
// can change between calls, so callers ask for it inside each transaction instead of keeping it.
type FieldRegistry interface {
	// Registry returns the current registry.
	Registry(ctx context.Context, tx platform.TransactionContext) (*fields.Registry, error)
}

// CompileSelection checks a predicate against the Message-scope fields of the registry. With
// strict, deprecated fields are rejected like unknown ones, as a new reference must be; a
// stored predicate is compiled without it. A nil predicate compiles to nil. Failures are those
// of fields.CompileFilter: INVALID_INPUT for a malformed predicate or an unknown or
// non-filterable field, RESOURCE_LIMIT for an exceeded bound.
func CompileSelection(registry *fields.Registry, predicate *processing.Predicate, strict bool) (*fields.Filter, error) {
	if predicate == nil {
		return nil, nil
	}
	return fields.CompileFilter(*predicate, registry.Declarations(fields.ScopeMessage, strict))
}

// selection compiles the predicate of the filters, if any, with the current registry.
func (s *Service) selection(ctx context.Context, tx platform.TransactionContext, filters Filters) (*fields.Filter, error) {
	if filters.Filter == nil {
		return nil, nil
	}
	registry, err := s.registry.Registry(ctx, tx)
	if err != nil {
		return nil, err
	}
	return CompileSelection(registry, filters.Filter, true)
}

// prepare compiles the predicate of the filters and checks that the caller can read the field
// groups that the filters need.
func (s *Service) prepare(ctx context.Context, tx platform.TransactionContext, current access,
	filters Filters) (*fields.Filter, error) {
	selection, err := s.selection(ctx, tx, filters)
	if err != nil {
		return nil, err
	}
	return selection, current.requireGroups(filters.RequiredFieldGroups(selection))
}
