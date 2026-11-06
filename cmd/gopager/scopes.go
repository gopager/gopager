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

// scopeOwner is the batched existence port every owning store implements.
type scopeOwner interface {
	ScopesExist(context.Context, platform.TransactionContext, []authz.Scope) ([]bool, error)
}

// ScopesExist splits scopes by owner and asks each owner once, so a resolution costs a
// fixed number of owner calls whatever the number of grants. The result is aligned with scopes.
func (o applicationScopeOwners) ScopesExist(ctx context.Context, tx platform.TransactionContext, scopes []authz.Scope) ([]bool, error) {
	var unitScopes, identityScopes []authz.Scope
	var unitAt, identityAt []int
	for i, scope := range scopes {
		switch scope.Kind {
		case "agency", "unit", "unitCollection":
			unitScopes, unitAt = append(unitScopes, scope), append(unitAt, i)
		default:
			identityScopes, identityAt = append(identityScopes, scope), append(identityAt, i)
		}
	}
	result := make([]bool, len(scopes))
	for _, part := range []struct {
		owner  scopeOwner
		scopes []authz.Scope
		at     []int
	}{{o.units, unitScopes, unitAt}, {o.identity, identityScopes, identityAt}} {
		if len(part.scopes) == 0 {
			continue
		}
		existing, err := part.owner.ScopesExist(ctx, tx, part.scopes)
		if err != nil {
			return nil, err
		}
		for j, i := range part.at {
			result[i] = existing[j]
		}
	}
	return result, nil
}
