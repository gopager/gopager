package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

// inlineTransactions runs the callback without a database.
type inlineTransactions struct{ calls int }

type inlineTx struct{ ctx context.Context }

func (t inlineTx) Context() context.Context { return t.ctx }

func (t *inlineTransactions) WithinTransaction(ctx context.Context, fn platform.TxFunc) error {
	t.calls++
	return fn(inlineTx{ctx})
}

func TestResolverAuthenticateChecksCredentialWithoutGrantsOrTouch(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*authorityFixture)
		allow  bool
	}{
		{"valid session", func(*authorityFixture) {}, true},
		{"revoked session", func(f *authorityFixture) { f.credential.RevokedAt = &f.now }, false},
		{"expired session", func(f *authorityFixture) { f.credential.ExpiresAt = &f.now }, false},
		{"idle session", func(f *authorityFixture) { f.credential.LastUsedAt = f.now.Add(-7 * 24 * time.Hour) }, false},
		{"wrong kind", func(f *authorityFixture) { f.credential.Kind = AccessKeyProof }, false},
		{"unknown key", func(f *authorityFixture) { f.credentialErr = ErrLoginRequired }, false},
		{"inactive account", func(f *authorityFixture) { f.account.State = "suspended" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture, proof := newAuthorityFixture(t)
			test.change(fixture)
			resolver, err := NewResolver(fixture, fixture, fixture)
			if err != nil {
				t.Fatal(err)
			}
			err = resolver.Authenticate(t.Context(), nil, proof)
			if test.allow != (err == nil) || (!test.allow && !errors.Is(err, ErrLoginRequired)) {
				t.Fatalf("Authenticate = %v, want allow=%t", err, test.allow)
			}
			if fixture.touched || fixture.grantCalls != 0 || fixture.scopeCalls != 0 {
				t.Fatalf("touched=%t grantCalls=%d scopeCalls=%d, want no grant resolution or write",
					fixture.touched, fixture.grantCalls, fixture.scopeCalls)
			}
		})
	}
}

func TestPreauthenticatorSkipsAnonymousAndUsesOneTransaction(t *testing.T) {
	t.Parallel()
	fixture, proof := newAuthorityFixture(t)
	resolver, err := NewResolver(fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	transactions := &inlineTransactions{}
	preauth, err := NewPreauthenticator(transactions, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if err := preauth.Authenticate(t.Context(), Proof{Kind: AnonymousProof}); err != nil || transactions.calls != 0 {
		t.Fatalf("anonymous: err=%v transactions=%d, want nil and none", err, transactions.calls)
	}
	if err := preauth.Authenticate(t.Context(), proof); err != nil || transactions.calls != 1 {
		t.Fatalf("session: err=%v transactions=%d, want nil and one", err, transactions.calls)
	}
	fixture.credential.RevokedAt = &fixture.now
	if err := preauth.Authenticate(t.Context(), proof); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("revoked session: %v, want ErrLoginRequired", err)
	}
	if _, err := NewPreauthenticator(nil, resolver); err == nil {
		t.Fatal("nil transactions accepted")
	}
}
