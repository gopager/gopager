// Package permissions binds structured domain authorization to native Ent predicates.
package permissions

import (
	"slices"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

// Attributes names the consumer's Ent fields/edges, never caller-supplied SQL.
type Attributes struct {
	Agency          string
	Unit            string
	Account         string
	UnitCollections string
	AgeAnchor       string
	// Scope binds composite owner relations; it cannot change grant/key/field composition.
	Scope func(kind string, references []uuid.UUID) (entql.P, error)
}

// Plan holds mandatory EntQL predicates for one concrete contribution/resource row.
// Point authorization uses the same predicate restricted to the requested object ID.
type Plan struct {
	Predicate      entql.P
	PrincipalID    string
	AccessRevision int64
	Fields         []string
}

// ReadScope builds one native EntQL constraint for rows, fields and independent age union.
// Adapter field names are trusted schema bindings; data values remain library parameters.
func ReadScope(authority authz.Authority, action string, fields []string, attributes Attributes, now time.Time) (Plan, error) {
	if !authority.Active || authority.PrincipalID == "" || authority.Revision <= 0 {
		return Plan{}, authz.ErrForbidden
	}
	if !authz.ValidAction(action) {
		return Plan{}, authz.ErrInvalidInput
	}
	for _, field := range fields {
		if !authz.ValidField(field) {
			return Plan{}, authz.ErrInvalidInput
		}
	}
	if err := authz.AuthorizeFields(authority, fields); err != nil {
		return Plan{}, err
	}
	predicate, err := grantPredicate(authority.Grants, action, attributes, now)
	if err != nil {
		return Plan{}, err
	}
	if action == "message.detail" {
		read, err := grantPredicate(authority.Grants, "message.read", attributes, now)
		if err != nil {
			return Plan{}, err
		}
		predicate = entql.And(predicate, read)
	}
	if action == "without-unit.read" {
		if attributes.Unit == "" {
			return Plan{}, authz.ErrInvalidInput
		}
		predicate = entql.And(predicate, entql.FieldNil(attributes.Unit))
	}
	if authority.Key != nil {
		key := authority.Key
		if err := authz.ValidateRestrictions(key); err != nil {
			return Plan{}, err
		}
		if !slices.Contains(key.Actions, action) ||
			(action == "message.detail" && !slices.Contains(key.Actions, "message.read")) ||
			(key.MaxAgeSeconds != nil && *key.MaxAgeSeconds < 0) {
			return Plan{}, authz.ErrForbidden
		}
		for _, field := range fields {
			if !slices.Contains(key.Fields, field) {
				return Plan{}, authz.ErrForbidden
			}
		}
		keyScopes := make([]entql.P, 0, len(key.Scopes))
		for _, scope := range key.Scopes {
			if !authz.ValidScope(scope) {
				return Plan{}, authz.ErrInvalidInput
			}
			if !authz.ActionAllowsScope(action, scope.Kind) {
				continue
			}
			p, err := scopePredicate(scope, attributes)
			if err != nil {
				return Plan{}, err
			}
			keyScopes = append(keyScopes, p)
		}
		if len(keyScopes) == 0 {
			return Plan{}, authz.ErrForbidden
		}
		age, err := agePredicate(key.MaxAgeSeconds, attributes.AgeAnchor, now)
		if err != nil {
			return Plan{}, err
		}
		predicate = entql.And(predicate, disjunction(keyScopes), age)
	}
	return Plan{Predicate: predicate, PrincipalID: authority.PrincipalID,
		AccessRevision: authority.Revision, Fields: slices.Clone(fields)}, nil
}

func grantPredicate(grants []authz.Grant, action string, attributes Attributes, now time.Time) (entql.P, error) {
	var rows []entql.P
	for _, grant := range grants {
		if err := authz.ValidateGrant(grant); err != nil {
			return nil, err
		}
		if !slices.Contains(grant.Actions, action) {
			continue
		}
		scope, err := scopePredicate(grant.Scope, attributes)
		if err != nil {
			return nil, err
		}
		age, err := agePredicate(grant.MaxAgeSeconds, attributes.AgeAnchor, now)
		if err != nil {
			return nil, err
		}
		rows = append(rows, entql.And(scope, age))
	}
	if len(rows) == 0 {
		return nil, authz.ErrForbidden
	}
	return disjunction(rows), nil
}

func disjunction(predicates []entql.P) entql.P {
	if len(predicates) == 1 {
		return predicates[0]
	}
	return entql.Or(predicates[0], predicates[1], predicates[2:]...)
}

func scopePredicate(scope authz.Scope, attributes Attributes) (entql.P, error) {
	if scope.Kind == "all" {
		return entql.FieldNotNil("id"), nil
	}
	name := map[string]string{"agency": attributes.Agency, "unit": attributes.Unit, "account": attributes.Account}[scope.Kind]
	values := make([]any, len(scope.References))
	references := make([]uuid.UUID, len(scope.References))
	for i, ref := range scope.References {
		id, err := uuid.Parse(ref)
		if err != nil {
			return nil, authz.ErrInvalidInput
		}
		values[i] = id
		references[i] = id
	}
	if attributes.Scope != nil {
		return attributes.Scope(scope.Kind, references)
	}
	if scope.Kind == "unitCollection" {
		if attributes.UnitCollections == "" {
			return nil, authz.ErrInvalidInput
		}
		return entql.HasEdgeWith(attributes.UnitCollections, entql.FieldIn("id", values...)), nil
	}
	if name == "" {
		return nil, authz.ErrInvalidInput
	}
	return entql.FieldIn(name, values...), nil
}

func agePredicate(seconds *int64, anchor string, now time.Time) (entql.P, error) {
	if seconds == nil {
		return entql.FieldNotNil("id"), nil
	}
	if *seconds < 0 || anchor == "" {
		return nil, authz.ErrInvalidInput
	}
	cutoff, err := authz.AgeCutoff(*seconds, now)
	if err != nil {
		return nil, err
	}
	return entql.FieldGTE(anchor, cutoff), nil
}
