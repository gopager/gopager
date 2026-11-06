package borrowed_test

import (
	"errors"
	"github.com/gopager/gopager/internal/testutil/passwords"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/platform"
)

func TestLocalBootstrapIsAtomicAndCreatesOnlyExplicitAuthority(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	permissions := identity.BootstrapPermissions{Actions: []string{"password.local", "session.manage-own"}, Fields: []string{"address"}}
	failure := errors.New("bootstrap fact failure")
	id, err := identity.Bootstrap(t.Context(), store, repository, store, failingLoginFacts{failure},
		"Operator", "initial-password", permissions, passwords.Option())
	if !errors.Is(err, failure) || id != uuid.Nil {
		t.Fatalf("bootstrap reported success after failed fact: %v", err)
	}
	id, err = identity.Bootstrap(t.Context(), store, repository, store, facts, "Operator", "initial-password", permissions, passwords.Option())
	if err != nil {
		t.Fatalf("failed bootstrap did not roll back one-time guard: %v", err)
	}
	if _, err := identity.Bootstrap(t.Context(), store, repository, store, facts,
		"replacement", "another-password", permissions, passwords.Option()); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("completed bootstrap was reopened: %v", err)
	}
	resolver, err := identity.NewResolver(repository, store, repository)
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginService(store, resolver, repository, facts, identity.DefaultLoginRateLimits(), passwords.Option())
	if err != nil {
		t.Fatal(err)
	}
	session, err := login.AuthenticateLocal(t.Context(), "test-source", "operator", "initial-password")
	if err != nil {
		t.Fatal(err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		authentication, err := resolver.Resolve(tx.Context(), tx, identity.Proof{Kind: identity.SessionProof, Secret: session.Secret})
		if err != nil {
			return err
		}
		if authentication.Authority.PrincipalID != id.String() || len(authentication.Authority.Grants) != 1 {
			t.Fatal("bootstrap did not preserve the explicitly selected Principal grants")
		}
		grant := authentication.Authority.Grants[0]
		if len(grant.Actions) != 2 || len(authentication.Authority.FieldPermissions) != 1 || grant.Delegable ||
			authz.AuthorizeFields(authentication.Authority, []string{"address"}) != nil {
			t.Fatal("bootstrap introduced implicit administrator capabilities")
		}
		if err := authz.AuthorizeFields(authentication.Authority, []string{"payload"}); !errors.Is(err, authz.ErrForbidden) {
			t.Fatal("bootstrap introduced an unselected global field")
		}
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		if err := authz.Authorize(authentication.Authority, "grant.manage", nil, authz.Target{AccountID: id}, now); !errors.Is(err, authz.ErrForbidden) {
			t.Fatal("local bootstrap created an implicit administrator bypass")
		}
		guest, err := resolver.Resolve(tx.Context(), tx, identity.Proof{Kind: identity.AnonymousProof})
		if err != nil {
			return err
		}
		if len(guest.Authority.Grants) != 0 || len(guest.Authority.FieldPermissions) != 0 || guest.Authority.PrincipalID == id.String() {
			t.Fatal("bootstrap Guest inherited initial account permissions")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentLocalBootstrapHasOneWinner(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, name := range []string{"first", "second"} {
		go func() {
			<-start
			_, err := identity.Bootstrap(t.Context(), store, repository, store, facts,
				name, "initial-password", identity.BootstrapPermissions{Actions: []string{"password.local"}}, passwords.Option())
			results <- err
		}()
	}
	close(start)
	winners, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			winners++
		case errors.Is(err, authz.ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected bootstrap result: %v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("bootstrap races produced %d winners and %d conflicts", winners, conflicts)
	}
}
