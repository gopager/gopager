package presentation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// caller is what a presentation operation needs to know about its caller.
type caller struct {
	principal uuid.UUID
	guest     bool
	authority authz.Authority
}

// callerScope resolves a caller and runs an operation in the same transaction. The services of
// this package embed it.
type callerScope struct {
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
}

// within resolves the caller and runs operation in the same transaction. A guest may not
// mutate (identity.ErrLoginRequired).
func (s callerScope) within(ctx context.Context, proof identity.Proof, mutation bool, check func(identity.Authentication) error,
	operation func(tx platform.TransactionContext, who caller, now time.Time) error) error {
	// The wrapper below is never nil, so WithAuthority cannot see a missing check itself.
	if mutation && check == nil {
		return authz.ErrInvalidInput
	}
	var authentication identity.Authentication
	remember := func(value identity.Authentication) error {
		authentication = value
		if check != nil {
			return check(value)
		}
		return nil
	}
	return identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, mutation, remember,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			// PostgreSQL keeps microseconds; a response must equal what a later read returns.
			now = now.Truncate(time.Microsecond)
			principal, err := uuid.Parse(authority.PrincipalID)
			if err != nil || principal == uuid.Nil {
				return authz.ErrForbidden
			}
			who := caller{principal: principal, guest: authentication.Kind == identity.AnonymousProof, authority: authority}
			if mutation && who.guest {
				return identity.ErrLoginRequired
			}
			return operation(tx, who, now)
		})
}
