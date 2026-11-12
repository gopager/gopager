package borrowed_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/platform"
)

func TestAccountPreferencesStore(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	accounts, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	account := uuid.New()
	within := func(fn func(tx platform.TransactionContext) error) error {
		return store.WithinTransaction(t.Context(), fn)
	}
	if err := within(func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO identity_accounts (id, kind, state, created_at) VALUES ($1, 'human', 'active', now())`, account)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read := func() (identity.Preferences, bool) {
		var got identity.Preferences
		var found bool
		if err := within(func(tx platform.TransactionContext) error {
			var err error
			got, found, err = accounts.ReadPreferences(tx.Context(), tx, account)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return got, found
	}
	save := func(expected int64, input identity.PreferencesInput) error {
		return within(func(tx platform.TransactionContext) error {
			return accounts.SavePreferences(tx.Context(), tx, account, expected, input, time.Now())
		})
	}
	if _, found := read(); found {
		t.Fatal("preferences exist before the first save")
	}
	if err := save(0, identity.PreferencesInput{Locale: "de-AT", TimeZone: "Europe/Vienna", TimeGapMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	if got, found := read(); !found || got != (identity.Preferences{Locale: "de-AT", TimeZone: "Europe/Vienna", TimeGapMinutes: 15, Revision: 1}) {
		t.Fatalf("stored = %+v, %v", got, found)
	}
	if err := save(0, identity.PreferencesInput{Locale: "en", TimeZone: "UTC"}); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("second first-save: %v", err)
	}
	if err := save(3, identity.PreferencesInput{Locale: "en", TimeZone: "UTC"}); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale: %v", err)
	}
	if err := save(1, identity.PreferencesInput{Locale: "en", TimeZone: "UTC", TimeGapMinutes: 1441}); err == nil {
		t.Fatal("the table accepted a gap above 1440")
	}
	if err := save(1, identity.PreferencesInput{Locale: "en", TimeZone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := read(); got.Revision != 2 || got.Locale != "en" {
		t.Fatalf("replaced = %+v", got)
	}
	deleteAccount := func() error {
		return within(func(tx platform.TransactionContext) error {
			return accounts.DeleteAccountPreferences(tx.Context(), tx, account)
		})
	}
	if err := deleteAccount(); err != nil {
		t.Fatal(err)
	}
	if _, found := read(); found {
		t.Fatal("preferences survived DeleteAccountPreferences")
	}
	if err := deleteAccount(); err != nil {
		t.Fatalf("deleting absent preferences: %v", err)
	}
}
