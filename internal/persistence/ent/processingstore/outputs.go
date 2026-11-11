package processingstore

import (
	"context"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// outputDeclarations returns the Attribute fields Workflow results may hold, as visible in the
// transaction. Deprecated fields stay valid: a result of an active version is stored and
// published even after a deprecation.
func (s *Store) outputDeclarations(ctx context.Context, tx platform.TransactionContext) (processing.Declarations, error) {
	registry, err := s.registries.Registry(ctx, tx)
	if err != nil {
		return nil, err
	}
	return registry.Declarations(fields.ScopeMessage, false), nil
}
