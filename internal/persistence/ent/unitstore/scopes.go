package unitstore

import (
	"context"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// ScopesExist checks every restricting reference of agency, unit and unitCollection scopes
// against its current owning entity (Agencies, non-deleted Units, Unit collections). It
// issues one query per kind present, however many scopes there are, because authority
// resolution runs on every request. The result is aligned with scopes; a scope exists only
// if all of its references do. Scopes of other kinds or with malformed references fail
// with authz.ErrInvalidInput.
func (s *Store) ScopesExist(ctx context.Context, tx platform.TransactionContext, scopes []authz.Scope) ([]bool, error) {
	parsed := make([][]uuid.UUID, len(scopes))
	wanted := map[string]map[uuid.UUID]bool{}
	for i, target := range scopes {
		if !authz.ValidScope(target) || (target.Kind != "unit" && target.Kind != "unitCollection" && target.Kind != "agency") {
			return nil, authz.ErrInvalidInput
		}
		if wanted[target.Kind] == nil {
			wanted[target.Kind] = map[uuid.UUID]bool{}
		}
		for _, reference := range target.References {
			id, err := uuid.Parse(reference)
			if err != nil {
				return nil, authz.ErrInvalidInput
			}
			parsed[i] = append(parsed[i], id)
			wanted[target.Kind][id] = true
		}
	}
	found := make(map[string]map[uuid.UUID]bool, len(wanted))
	for kind, ids := range wanted {
		existing, err := s.existingReferences(ctx, tx, kind, ids)
		if err != nil {
			return nil, err
		}
		found[kind] = existing
	}
	result := make([]bool, len(scopes))
	for i, target := range scopes {
		result[i] = true
		for _, id := range parsed[i] {
			result[i] = result[i] && found[target.Kind][id]
		}
	}
	return result, nil
}

// existingReferences returns which of ids exist for one scope kind, in a single query.
func (s *Store) existingReferences(ctx context.Context, tx platform.TransactionContext, kind string, ids map[uuid.UUID]bool) (map[uuid.UUID]bool, error) {
	values := make([]any, 0, len(ids))
	for id := range ids {
		values = append(values, id)
	}
	predicate := entql.FieldIn("id", values...)
	entity := "UnitCollection"
	switch kind {
	case "unit":
		entity = "Unit"
		predicate = entql.And(predicate, entql.FieldEQ("deleted", false))
	case "agency":
		entity = "Agency"
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{entity: predicate}, nil)
	if err != nil {
		return nil, err
	}
	var existing []uuid.UUID
	switch entity {
	case "Unit":
		existing, err = client.Unit.Query().IDs(scope)
	case "Agency":
		existing, err = client.Agency.Query().IDs(scope)
	default:
		existing, err = client.UnitCollection.Query().IDs(scope)
	}
	if err != nil {
		return nil, err
	}
	result := make(map[uuid.UUID]bool, len(existing))
	for _, id := range existing {
		result[id] = true
	}
	return result, nil
}
