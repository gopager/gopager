package readmodel

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// AdmissionWindow is the fixed window in which search requests are counted.
const AdmissionWindow = time.Minute

// DefaultSearchAttempts is the default number of search requests admitted per window.
const DefaultSearchAttempts = 60

// AdmissionStore counts search requests in fixed windows.
type AdmissionStore interface {
	// TakeSearchAttempt counts one request of the bucket in the window starting at windowStart,
	// only while the window's count is below limit, and deletes the bucket's older windows in
	// the same transaction. It reports whether the request was admitted; a rejected request is
	// not counted.
	TakeSearchAttempt(ctx context.Context, tx platform.TransactionContext, bucket [32]byte,
		windowStart time.Time, limit int) (bool, error)
}

// Admission limits search requests (feed pages and live stream opens) per fixed one-minute
// window. Create it with NewAdmission; it is safe for concurrent use.
//
// Admit runs in its own short transaction, before and apart from the read it guards. A rejected
// request therefore never holds the read's snapshot or connection, and an admitted request that
// later fails (for example with FORBIDDEN or INVALID_INPUT) still counts, as every attempt does.
type Admission struct {
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
	store        AdmissionStore
	limit        int
}

// NewAdmission composes the admission with a per-window limit of at least 1. It fails with
// authz.ErrInvalidInput on a missing dependency or a smaller limit.
func NewAdmission(transactions identity.Transactions, resolver identity.AuthorityResolver, clock platform.Clock,
	store AdmissionStore, limit int) (*Admission, error) {
	if transactions == nil || resolver == nil || clock == nil || store == nil || limit < 1 {
		return nil, authz.ErrInvalidInput
	}
	return &Admission{transactions: transactions, resolver: resolver, clock: clock, store: store, limit: limit}, nil
}

// AdmissionBucket is the SHA-256 bucket of a caller: the credential for an access key, the
// Principal for a session and, for the guest, the client source. Logged-in users behind a
// shared address therefore keep their own buckets, and a key is separate from its owner's
// sessions.
func AdmissionBucket(authentication identity.Authentication, source string) [32]byte {
	switch authentication.Kind {
	case identity.AccessKeyProof:
		return sha256.Sum256([]byte("key:" + authentication.CredentialID.String()))
	case identity.SessionProof:
		return sha256.Sum256([]byte("principal:" + authentication.Authority.PrincipalID))
	}
	return sha256.Sum256([]byte("source:" + source))
}

// RetryAfter is the time from now until the fixed window containing now ends, rounded up to
// whole seconds by *identity.RateLimited.Seconds and never below one second.
func RetryAfter(now time.Time) time.Duration {
	return now.Truncate(AdmissionWindow).Add(AdmissionWindow).Sub(now)
}

// Admit counts one search request of the caller proved by proof. source is the client source
// (identity.ClientSource), used only for the guest. Call it after the request is parsed and
// before the read; the live stream calls it once per open. It returns *identity.RateLimited
// with the time until the window ends when the caller used up its window, and the
// authentication errors of identity.WithAuthority (identity.ErrLoginRequired) otherwise.
func (a *Admission) Admit(ctx context.Context, proof identity.Proof, source string) error {
	var authentication identity.Authentication
	remember := func(value identity.Authentication) error { authentication = value; return nil }
	var rejection *identity.RateLimited
	err := identity.WithAuthority(ctx, a.transactions, a.clock, a.resolver, proof, false, remember,
		func(tx platform.TransactionContext, _ authz.Authority, now time.Time) error {
			rejection = nil
			admitted, err := a.store.TakeSearchAttempt(tx.Context(), tx, AdmissionBucket(authentication, source),
				now.Truncate(AdmissionWindow), a.limit)
			if err != nil {
				return err
			}
			if !admitted {
				rejection = &identity.RateLimited{RetryAfter: RetryAfter(now)}
			}
			return nil
		})
	if err != nil {
		return err
	}
	if rejection != nil {
		return rejection
	}
	return nil
}
