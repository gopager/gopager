// Package guard supplies mandatory native Ent privacy rules for persistence adapters.
package guard

import (
	"context"
	"maps"

	"entgo.io/ent"
	"entgo.io/ent/entql"

	"github.com/gopager/gopager/internal/persistence/ent/privacy"
)

type accessKey struct{}

// Access is an adapter-owned decision valid only while its transaction is active.
type Access struct {
	Predicates map[string]entql.P
	Create     map[string]bool
	Current    func() bool
}

// Bind attaches an adapter-validated decision, never a user-supplied privacy Allow.
func Bind(ctx context.Context, access Access) context.Context {
	access.Predicates = maps.Clone(access.Predicates)
	access.Create = maps.Clone(access.Create)
	return context.WithValue(ctx, accessKey{}, access)
}

// Policy denies absent/expired context and installs native row filters before access.
func Policy(entity string) ent.Policy {
	filter := privacy.FilterFunc(func(ctx context.Context, query privacy.Filter) error {
		access, ok := ctx.Value(accessKey{}).(Access)
		if !ok || access.Current == nil || !access.Current() || access.Predicates[entity] == nil {
			return privacy.Deny
		}
		query.Where(access.Predicates[entity])
		return privacy.Skip
	})
	return privacy.Policy{
		Query: privacy.QueryPolicy{filter},
		Mutation: privacy.MutationPolicy{privacy.MutationRuleFunc(func(ctx context.Context, mutation ent.Mutation) error {
			if mutation.Op().Is(ent.OpCreate) {
				access, ok := ctx.Value(accessKey{}).(Access)
				if !ok || access.Current == nil || !access.Current() || !access.Create[entity] {
					return privacy.Deny
				}
				return privacy.Skip
			}
			return filter.EvalMutation(ctx, mutation)
		})},
	}
}
