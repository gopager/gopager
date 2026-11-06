package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// trackedTransactions reports whether a callback is currently running, so tests can prove
// that a collaborator was called inside the transaction.
type trackedTransactions struct {
	inside *bool
	began  *int
}

func (t trackedTransactions) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	*t.began++
	*t.inside = true
	defer func() { *t.inside = false }()
	return callback(fakeTransactionContext{ctx})
}

// fakeAuthorityResolver returns a fixed Authentication and records where it was called.
type fakeAuthorityResolver struct {
	inside         *bool
	calledInside   bool
	authentication Authentication
	err            error
}

func (r *fakeAuthorityResolver) Resolve(context.Context, platform.TransactionContext, Proof) (Authentication, error) {
	r.calledInside = *r.inside
	return r.authentication, r.err
}

func TestWithAuthorityResolvesAuthorityInsideTheTransaction(t *testing.T) {
	t.Parallel()
	var inside bool
	var began int
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	want := authz.Authority{}
	resolver := &fakeAuthorityResolver{inside: &inside, authentication: Authentication{Authority: want}}
	var gotNow time.Time
	err := WithAuthority(t.Context(), trackedTransactions{&inside, &began}, fixedClock{now}, resolver,
		Proof{}, false, nil, func(_ platform.TransactionContext, authority authz.Authority, at time.Time) error {
			if !inside {
				t.Error("operation ran outside the transaction")
			}
			gotNow = at
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !resolver.calledInside || began != 1 {
		t.Fatalf("resolver inside transaction = %t, transactions = %d, want true and 1", resolver.calledInside, began)
	}
	if !gotNow.Equal(now) {
		t.Fatalf("operation time = %v, want the primary clock %v", gotNow, now)
	}
}

func TestWithAuthorityRequiresACheckOnlyForMutations(t *testing.T) {
	t.Parallel()
	var inside bool
	var began int
	resolver := &fakeAuthorityResolver{inside: &inside}
	run := func(mutation bool, check func(Authentication) error) (ran bool, err error) {
		err = WithAuthority(t.Context(), trackedTransactions{&inside, &began}, fixedClock{}, resolver,
			Proof{}, mutation, check, func(platform.TransactionContext, authz.Authority, time.Time) error {
				ran = true
				return nil
			})
		return ran, err
	}

	began = 0
	if ran, err := run(true, nil); !errors.Is(err, authz.ErrInvalidInput) || ran || began != 0 {
		t.Fatalf("mutation without check: ran=%t transactions=%d err=%v, want rejected before any transaction", ran, began, err)
	}
	if ran, err := run(false, nil); err != nil || !ran {
		t.Fatalf("read without check: ran=%t err=%v, want it to run", ran, err)
	}
	var checked int
	if ran, err := run(true, func(Authentication) error { checked++; return nil }); err != nil || !ran || checked != 1 {
		t.Fatalf("mutation with check: ran=%t checked=%d err=%v, want one check and the operation", ran, checked, err)
	}
	refusal := errors.New("csrf refused")
	if ran, err := run(true, func(Authentication) error { return refusal }); !errors.Is(err, refusal) || ran {
		t.Fatalf("failing check: ran=%t err=%v, want the check error and no operation", ran, err)
	}
}

func TestWithAuthorityReturnsTheOperationErrorUnchanged(t *testing.T) {
	t.Parallel()
	var inside bool
	var began int
	resolver := &fakeAuthorityResolver{inside: &inside}
	failure := platform.NewFailure(platform.CodeStorageUnavailable, errors.New("operation failed"))
	err := WithAuthority(t.Context(), trackedTransactions{&inside, &began}, fixedClock{}, resolver,
		Proof{}, false, nil, func(platform.TransactionContext, authz.Authority, time.Time) error { return failure })
	if err != failure {
		t.Fatalf("error = %v, want the operation's own error value", err)
	}
}

func TestWithAuthorityStopsWhenResolutionFails(t *testing.T) {
	t.Parallel()
	var inside bool
	var began int
	resolver := &fakeAuthorityResolver{inside: &inside, err: ErrLoginRequired}
	err := WithAuthority(t.Context(), trackedTransactions{&inside, &began}, fixedClock{}, resolver,
		Proof{}, false, nil, func(platform.TransactionContext, authz.Authority, time.Time) error {
			t.Fatal("operation ran without authority")
			return nil
		})
	if !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("error = %v, want ErrLoginRequired", err)
	}
}

func TestWithAuthorityOptions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Second)
	noop := func(platform.TransactionContext, authz.Authority, time.Time) error { return nil }

	t.Run("expired authentication is rejected only when enforced", func(t *testing.T) {
		t.Parallel()
		var inside bool // per subtest: the tracked flag is written by every transaction
		var began int
		transactions := trackedTransactions{&inside, &began}
		resolver := &fakeAuthorityResolver{inside: &inside, authentication: Authentication{ExpiresAt: &past}}
		if err := WithAuthority(t.Context(), transactions, fixedClock{now}, resolver, Proof{}, false, nil, noop); err != nil {
			t.Fatalf("default policy rejected: %v", err)
		}
		err := WithAuthority(t.Context(), transactions, fixedClock{now}, resolver, Proof{}, false, nil, noop,
			RejectExpiredAuthentication())
		if !errors.Is(err, ErrLoginRequired) {
			t.Fatalf("enforced policy error = %v, want ErrLoginRequired", err)
		}
		resolver.authentication.ExpiresAt = &future
		if err := WithAuthority(t.Context(), transactions, fixedClock{now}, resolver, Proof{}, false, nil, noop,
			RejectExpiredAuthentication()); err != nil {
			t.Fatalf("unexpired authentication rejected: %v", err)
		}
	})

	t.Run("lock runs inside the transaction for mutations only", func(t *testing.T) {
		t.Parallel()
		var inside bool // per subtest: the tracked flag is written by every transaction
		var began int
		transactions := trackedTransactions{&inside, &began}
		resolver := &fakeAuthorityResolver{inside: &inside}
		var locks int
		lock := LockMutations(func(context.Context, platform.TransactionContext) error {
			if !inside {
				t.Error("lock ran outside the transaction")
			}
			locks++
			return nil
		})
		check := func(Authentication) error { return nil }
		if err := WithAuthority(t.Context(), transactions, fixedClock{now}, resolver, Proof{}, false, nil, noop, lock); err != nil || locks != 0 {
			t.Fatalf("read: locks=%d err=%v, want no lock", locks, err)
		}
		if err := WithAuthority(t.Context(), transactions, fixedClock{now}, resolver, Proof{}, true, check, noop, lock); err != nil || locks != 1 {
			t.Fatalf("mutation: locks=%d err=%v, want one lock", locks, err)
		}
	})

	t.Run("lock failure stops the operation", func(t *testing.T) {
		t.Parallel()
		var inside bool // per subtest: the tracked flag is written by every transaction
		var began int
		transactions := trackedTransactions{&inside, &began}
		resolver := &fakeAuthorityResolver{inside: &inside}
		failure := errors.New("lock unavailable")
		err := WithAuthority(t.Context(), transactions, fixedClock{now}, resolver, Proof{}, true,
			func(Authentication) error { return nil },
			func(platform.TransactionContext, authz.Authority, time.Time) error {
				t.Fatal("operation ran after the lock failed")
				return nil
			},
			LockMutations(func(context.Context, platform.TransactionContext) error { return failure }))
		if !errors.Is(err, failure) {
			t.Fatalf("error = %v, want the lock error", err)
		}
	})
}
