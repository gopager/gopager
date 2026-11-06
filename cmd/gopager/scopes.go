package main

import (
	"context"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
)

// applicationScopeOwners dispatches scope existence checks to the owning stores: Agencies,
// Units and Unit collections to master data, accounts to identity.
type applicationScopeOwners struct {
	identity *identitystore.Store
	units    *unitstore.Store
}

// ScopeExists dispatches current references to their owners inside the protected transaction.
func (o applicationScopeOwners) ScopeExists(ctx context.Context, tx platform.TransactionContext, scope authz.Scope) (bool, error) {
	switch scope.Kind {
	case "agency", "unit", "unitCollection":
		return o.units.ScopeExists(ctx, tx, scope)
	default:
		return o.identity.ScopeExists(ctx, tx, scope)
	}
}
