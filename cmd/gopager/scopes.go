package main

import (
	"context"
	"errors"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
)

type applicationScopeOwners struct {
	identity *identitystore.Store
	units    *unitstore.Store
	networks identity.ScopeReferences
}

// ScopeExists dispatches current references to their owners inside the protected transaction.
func (o applicationScopeOwners) ScopeExists(ctx context.Context, tx platform.TransactionContext, scope authz.Scope) (bool, error) {
	switch scope.Kind {
	case "unit", "unitCollection":
		return o.units.ScopeExists(ctx, tx, scope)
	case "network":
		if o.networks == nil {
			return false, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Network scope owner is not wired"))
		}
		return o.networks.ScopeExists(ctx, tx, scope)
	default:
		return o.identity.ScopeExists(ctx, tx, scope)
	}
}
