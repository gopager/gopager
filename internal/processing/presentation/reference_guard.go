package presentation

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// ReferenceGuard reports whether a layout assignment or the installation default references a Layout.
type ReferenceGuard interface {
	// LayoutReferenced locks the Layout row until the transaction ends, so a concurrent assignment
	// cannot add a reference while the Layout is deleted, and reports whether a layout assignment
	// or the installation default references the Layout. A missing Layout counts as unreferenced.
	LayoutReferenced(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (bool, error)
}

// ServiceOption configures optional collaborators of Service.
type ServiceOption func(*Service)

// WithReferenceGuard makes Delete refuse, with authz.ErrRevisionConflict, a Layout that a layout
// assignment or the installation default references.
func WithReferenceGuard(guard ReferenceGuard) ServiceOption {
	return func(s *Service) { s.guard = guard }
}

// checkUnreferenced fails with authz.ErrRevisionConflict when a configured guard finds a
// reference to the Layout.
func (s *Service) checkUnreferenced(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) error {
	if s.guard == nil {
		return nil
	}
	referenced, err := s.guard.LayoutReferenced(ctx, tx, id)
	if err != nil {
		return err
	}
	if referenced {
		return authz.ErrRevisionConflict
	}
	return nil
}
