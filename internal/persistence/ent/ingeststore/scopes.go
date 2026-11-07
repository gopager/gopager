package ingeststore

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// ScopesExist reports for each "gateway" scope whether every referenced Gateway exists
// undeleted, with one query for all references. The result is aligned with scopes; any other
// scope kind is an explicit authority failure, because this store does not own it.
func (s *Store) ScopesExist(ctx context.Context, tx platform.TransactionContext, scopes []authz.Scope) ([]bool, error) {
	parsed := make([][]uuid.UUID, len(scopes))
	var wanted []uuid.UUID
	for i, scope := range scopes {
		if !authz.ValidScope(scope) {
			return nil, authz.ErrInvalidInput
		}
		if scope.Kind != "gateway" {
			return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("scope owner is not wired"))
		}
		for _, reference := range scope.References {
			id, err := uuid.Parse(reference)
			if err != nil {
				return nil, authz.ErrInvalidInput
			}
			parsed[i] = append(parsed[i], id)
			wanted = append(wanted, id)
		}
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id FROM gateways WHERE id = ANY($1::uuid[]) AND NOT deleted`, wanted)
	if err != nil {
		return nil, err
	}
	found := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		found[id] = true
	}
	if err := closeRows(rows); err != nil {
		return nil, err
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
