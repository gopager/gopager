package borrowed_test

import (
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/credential"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/platform"
)

// TestResolveWritesLastUsedAndReadsItBack expects a stale last_used_at to be stored, and the
// stored value to suppress a second write within five minutes. The interval boundaries are
// tested in identity.TestResolverThrottlesLastUsedWrites.
func TestResolveWritesLastUsedAndReadsItBack(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	clock := &loginClock{now: start}
	resolver, err := identity.NewResolver(repository, clock, repository)
	if err != nil {
		t.Fatal(err)
	}
	principal, credentialID := uuid.New(), uuid.New()
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Create: map[string]bool{"Account": true, "Credential": true}})
		if err := createAccount(ctx, client, principal, "touch-user", start); err != nil {
			return err
		}
		return client.Credential.Create().SetID(credentialID).SetAccountID(principal).
			SetKind(credential.KindSession).SetSecretHash(hash[:]).SetCsrfToken("csrf-test-token").
			SetCreatedAt(start.Add(-time.Hour)).SetLastUsedAt(start.Add(-10 * time.Minute)).
			SetExpiresAt(start.Add(24 * time.Hour)).Exec(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := identity.Proof{Kind: identity.SessionProof, Secret: secret}
	lastUsed := func() time.Time {
		var value time.Time
		err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			driver, err := borrowed.New(store, tx)
			if err != nil {
				return err
			}
			ctx := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
				Predicates: map[string]entql.P{"Credential": entql.FieldEQ("id", credentialID)}})
			record, err := ent.NewClient(ent.Driver(driver)).Credential.Get(ctx, credentialID)
			if err != nil {
				return err
			}
			value = record.LastUsedAt
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	resolveAt := func(at time.Time) {
		t.Helper()
		clock.now = at
		err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			_, err := resolver.Resolve(tx.Context(), tx, proof)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, step := range []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{"stale value is written", start, start},
		{"second resolution within five minutes is not written", start.Add(4 * time.Minute), start},
	} {
		resolveAt(step.at)
		if got := lastUsed(); !got.Equal(step.want) {
			t.Fatalf("%s: last_used_at=%v want %v", step.name, got, step.want)
		}
	}
}
