package identitystore

import (
	"context"
	"errors"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/platform"
)

// ScopesExist supplies the identity owner's references (all and account scopes) with at
// most one query for every account reference in scopes. The result is aligned with scopes.
// Other domain kinds require composition; an unwired owner is an explicit authority
// failure, not an unrestricted or empty substitute.
func (s *Store) ScopesExist(ctx context.Context, tx platform.TransactionContext, scopes []authz.Scope) ([]bool, error) {
	parsed := make([][]uuid.UUID, len(scopes))
	wanted := map[uuid.UUID]bool{}
	for i, target := range scopes {
		if !authz.ValidScope(target) {
			return nil, authz.ErrInvalidInput
		}
		if target.Kind == "all" {
			continue
		}
		if target.Kind != "account" {
			return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("scope owner is not wired"))
		}
		for _, reference := range target.References {
			id, err := uuid.Parse(reference)
			if err != nil {
				return nil, authz.ErrInvalidInput
			}
			parsed[i] = append(parsed[i], id)
			wanted[id] = true
		}
	}
	found := map[uuid.UUID]bool{}
	if len(wanted) > 0 {
		values := make([]any, 0, len(wanted))
		for id := range wanted {
			values = append(values, id)
		}
		client, scope, err := s.client(ctx, tx, "Account", entql.FieldIn("id", values...))
		if err != nil {
			return nil, err
		}
		ids, err := client.Account.Query().
			Where(account.StateNotIn(account.StateDeleting, account.StateDeleted)).IDs(scope)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			found[id] = true
		}
	}
	result := make([]bool, len(scopes))
	for i := range scopes {
		result[i] = true
		for _, id := range parsed[i] {
			result[i] = result[i] && found[id]
		}
	}
	return result, nil
}
