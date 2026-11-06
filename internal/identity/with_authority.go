package identity

import (
	"context"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// AuthorityResolver resolves a transport proof to the caller's current authentication inside an
// open transaction. *Resolver implements it; the interface exists so the shared transaction
// wrapper can be tested without a credential store.
type AuthorityResolver interface {
	Resolve(ctx context.Context, tx platform.TransactionContext, proof Proof) (Authentication, error)
}

// AuthorityOption selects a documented variation of WithAuthority's step order.
type AuthorityOption func(*authorityPolicy)

type authorityPolicy struct {
	lock          func(context.Context, platform.TransactionContext) error
	rejectExpired bool
}

// LockMutations runs lock for mutations right after the authority was resolved and before the
// caller's check, so a service can serialize its writes behind a revision row. Reads skip it.
func LockMutations(lock func(context.Context, platform.TransactionContext) error) AuthorityOption {
	return func(p *authorityPolicy) { p.lock = lock }
}

// RejectExpiredAuthentication makes WithAuthority read the primary clock before the caller's
// check and fail with ErrLoginRequired once the resolved authentication has expired. Without it
// the check runs first and expiry is left to the resolver, which is what the master-data and
// assignment services have always done.
func RejectExpiredAuthentication() AuthorityOption {
	return func(p *authorityPolicy) { p.rejectExpired = true }
}

// WithAuthority resolves the caller's current authority and runs operation in the same primary
// transaction, so a revoked grant can never authorize work that commits later.
//
// A mutation must carry a non-nil check (the transport's state-change check, for example CSRF);
// the missing check is rejected before any transaction starts. A non-nil check also runs for
// reads. The check, the primary clock reading and operation all run inside the transaction.
// The error of operation is returned unchanged, so callers keep matching their domain errors.
func WithAuthority(ctx context.Context, transactions Transactions, clock platform.Clock, resolver AuthorityResolver,
	proof Proof, mutation bool, check func(Authentication) error,
	operation func(platform.TransactionContext, authz.Authority, time.Time) error, options ...AuthorityOption) error {
	if mutation && check == nil {
		return authz.ErrInvalidInput
	}
	var policy authorityPolicy
	for _, option := range options {
		option(&policy)
	}
	return transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		if mutation && policy.lock != nil {
			if err := policy.lock(tx.Context(), tx); err != nil {
				return err
			}
		}
		if policy.rejectExpired {
			return runWithClockFirst(tx, clock, authentication, check, operation)
		}
		if check != nil {
			if err := check(authentication); err != nil {
				return err
			}
		}
		now, err := clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return operation(tx, authentication.Authority, now)
	})
}

// runWithClockFirst is the step order of RejectExpiredAuthentication: time, expiry, check, operation.
func runWithClockFirst(tx platform.TransactionContext, clock platform.Clock, authentication Authentication,
	check func(Authentication) error, operation func(platform.TransactionContext, authz.Authority, time.Time) error) error {
	now, err := clock.Now(tx.Context(), tx)
	if err != nil {
		return err
	}
	if authentication.ExpiresAt != nil && !now.Before(*authentication.ExpiresAt) {
		return ErrLoginRequired
	}
	if check != nil {
		if err := check(authentication); err != nil {
			return err
		}
	}
	return operation(tx, authentication.Authority, now)
}
