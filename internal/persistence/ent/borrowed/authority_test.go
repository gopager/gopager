package borrowed_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/credential"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

func TestResolverLoadsDirectAndGroupGrantsAndRejectsRevokedSession(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(repository, store, repository)
	if err != nil {
		t.Fatal(err)
	}
	principal, guest, group := uuid.New(), uuid.New(), uuid.New()
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	credentialID := uuid.New()
	setup := func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Create: map[string]bool{"Account": true, "UserGroup": true, "Membership": true, "Grant": true, "Credential": true}})
		if err := createAccount(ctx, client, principal, "resolver-user", now); err != nil {
			return err
		}
		if err := client.Account.Create().SetID(guest).SetKind(account.KindGuest).SetCreatedAt(now).Exec(ctx); err != nil {
			return err
		}
		if err := client.UserGroup.Create().SetID(group).SetName("operators").Exec(ctx); err != nil {
			return err
		}
		if err := client.Membership.Create().SetAccountID(principal).SetGroupID(group).Exec(ctx); err != nil {
			return err
		}
		for _, assignment := range []struct {
			subject uuid.UUID
			kind    grant.SubjectKind
			action  string
		}{
			{principal, grant.SubjectKindPrincipal, "account.read"},
			{group, grant.SubjectKindUserGroup, "gateway.update"},
		} {
			if err := client.Grant.Create().SetSubjectID(assignment.subject).SetSubjectKind(assignment.kind).
				SetActions([]string{assignment.action}).SetScope(authz.Scope{Kind: "all"}).
				Exec(ctx); err != nil {
				return err
			}
		}
		return client.Credential.Create().SetID(credentialID).SetAccountID(principal).
			SetKind(credential.KindSession).SetSecretHash(hash[:]).SetCsrfToken("csrf-test-token").
			SetCreatedAt(now).SetLastUsedAt(now).SetExpiresAt(now.Add(30 * 24 * time.Hour)).Exec(ctx)
	}
	if err := store.WithinTransaction(t.Context(), setup); err != nil {
		t.Fatal(err)
	}
	resolve := func(proof identity.Proof) (identity.Authentication, error) {
		var result identity.Authentication
		err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			result, err = resolver.Resolve(tx.Context(), tx, proof)
			return err
		})
		return result, err
	}
	sessionProof := identity.Proof{Kind: identity.SessionProof, Secret: secret}
	resolved, err := resolve(sessionProof)
	if err != nil || resolved.Authority.PrincipalID != principal.String() ||
		len(resolved.Authority.Grants) != 2 || resolved.CredentialID != credentialID ||
		resolved.CSRFToken != "csrf-test-token" {
		t.Fatalf("server-derived group/direct authority failed: %+v %v", resolved.LogValue(), err)
	}
	anonymous, err := resolve(identity.Proof{Kind: identity.AnonymousProof})
	if err != nil || anonymous.Authority.PrincipalID != guest.String() || len(anonymous.Authority.Grants) != 0 {
		t.Fatalf("guest gained grants or failed resolution: %v", err)
	}
	for _, invalid := range []identity.Proof{
		{}, {Kind: identity.SessionProof, Secret: "malformed"},
		{Kind: identity.AccessKeyProof, Secret: secret}, {Kind: identity.AnonymousProof, Secret: secret},
	} {
		if _, err := resolve(invalid); !errors.Is(err, identity.ErrLoginRequired) {
			t.Fatalf("bad proof became guest/effective authority: %v", err)
		}
	}
	protected := make(chan struct{})
	release := make(chan struct{})
	protectedResult := make(chan error, 1)
	go func() {
		protectedResult <- store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			if _, err := resolver.Resolve(tx.Context(), tx, sessionProof); err != nil {
				close(protected)
				return err
			}
			close(protected)
			select {
			case <-release:
				return nil
			case <-tx.Context().Done():
				return tx.Context().Err()
			}
		})
	}()
	<-protected
	defer close(release)
	revokerPID := make(chan int, 1)
	revokerResult := make(chan error, 1)
	go func() {
		revokerResult <- store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			var pid int
			if err := executor.QueryRowContext(tx.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				return err
			}
			revokerPID <- pid
			return revokeResolverCredential(tx.Context(), tx, store, repository, credentialID)
		})
	}()
	var pid int
	select {
	case pid = <-revokerPID:
	case err := <-revokerResult:
		t.Fatalf("revoker did not reach lock: %v", err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	waitForAuthorityLock(t, store, pid)
	release <- struct{}{}
	if err := <-protectedResult; err != nil {
		t.Fatal(err)
	}
	if err := <-revokerResult; err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(sessionProof); !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("revoked session still resolved: %v", err)
	}
}

// revokeResolverCredential takes the exclusive access lock that waits for protected work.
func revokeResolverCredential(ctx context.Context, tx platform.TransactionContext, store *postgres.Store, repository *identitystore.Store, id uuid.UUID) error {
	if _, err := repository.LockAccessForChange(tx.Context(), tx); err != nil {
		return err
	}
	driver, err := borrowed.New(store, tx)
	if err != nil {
		return err
	}
	now, err := store.Now(tx.Context(), tx)
	if err != nil {
		return err
	}
	scope := guard.Bind(ctx, guard.Access{Current: driver.Current,
		Predicates: map[string]entql.P{"Credential": entql.FieldEQ("id", id)}})
	_, err = ent.NewClient(ent.Driver(driver)).Credential.Update().SetRevokedAt(now).Save(scope)
	return err
}

// waitForAuthorityLock verifies actual PostgreSQL blocking instead of relying on scheduling sleeps.
func waitForAuthorityLock(t *testing.T, store *postgres.Store, pid int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(),
				"SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $1 AND wait_event_type = 'Lock')", pid).Scan(&waiting)
		})
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("revoker did not block on authority lock: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestResolverChecksKeyRestrictionAndDeletedScope(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(repository, store, repository)
	if err != nil {
		t.Fatal(err)
	}
	principal, scopedAccount := uuid.New(), uuid.New()
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Create: map[string]bool{"Account": true, "Grant": true, "Credential": true}})
		if err := createAccount(ctx, client, principal, "key-owner", now); err != nil {
			return err
		}
		if err := createAccount(ctx, client, scopedAccount, "scope-target", now); err != nil {
			return err
		}
		if err := client.Grant.Create().SetSubjectID(principal).SetSubjectKind(grant.SubjectKindPrincipal).
			SetActions([]string{"account.read"}).SetScope(authz.Scope{Kind: "all"}).
			Exec(ctx); err != nil {
			return err
		}
		return client.Credential.Create().SetAccountID(principal).SetKind(credential.KindAccessKey).
			SetSecretHash(hash[:]).SetCreatedAt(now).SetLastUsedAt(now).
			SetRestrictions(&authz.Restrictions{Actions: []string{"account.read"},
				Scopes: []authz.Scope{{Kind: "account", References: []string{scopedAccount.String()}}}}).Exec(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(tx platform.TransactionContext) error {
		result, err := resolver.Resolve(tx.Context(), tx, identity.Proof{Kind: identity.AccessKeyProof, Secret: secret})
		if err != nil {
			return err
		}
		if result.Authority.Key == nil || result.Authority.Key.Scopes[0].References[0] != scopedAccount.String() {
			t.Fatal("key restrictions were discarded")
		}
		return nil
	}
	if err := store.WithinTransaction(t.Context(), resolve); err != nil {
		t.Fatal(err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := repository.LockAccess(tx.Context(), tx); err != nil {
			return err
		}
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := accountContext(tx.Context(), driver, entql.FieldEQ("id", scopedAccount), false)
		_, err = client.Account.Update().SetState(account.StateDeleting).
			ClearUsername().ClearUsernameKey().Save(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithinTransaction(t.Context(), resolve); !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("deleted restricting account widened key: %v", err)
	}
}
