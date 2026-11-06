package borrowed_test

import (
	"errors"
	"testing"
	"time"

	"entgo.io/ent/entql"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/credential"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// TestMutationsWithAnExpiredSessionRequireLogin expects Unit and address-rule mutations to answer
// LOGIN_REQUIRED, and to commit nothing, once the caller's session has expired.
func TestMutationsWithAnExpiredSessionRequireLogin(t *testing.T) {
	f := newMatcherFixture(t)
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Before", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	before := configurationCounts(t, f)
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := f.owners.identity.LockAccessForChange(tx.Context(), tx); err != nil {
			return err
		}
		driver, err := borrowed.New(f.store, tx)
		if err != nil {
			return err
		}
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{"Credential": entql.FieldEQ("kind", string(credential.KindSession))}})
		_, err = ent.NewClient(ent.Driver(driver)).Credential.Update().SetExpiresAt(now.Add(-time.Second)).Save(scope)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "After", CollectionIDs: []uuid.UUID{}}, checkedMutation); !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("unit create with expired session: %v, want ErrLoginRequired", err)
	}
	if _, err := f.units.DeleteUnit(t.Context(), f.proof, unit.ID, unit.Revision, checkedMutation); !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("unit delete with expired session: %v, want ErrLoginRequired", err)
	}
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unit.ID, 10, 20, 1), checkedMutation); !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("rule create with expired session: %v, want ErrLoginRequired", err)
	}
	if after := configurationCounts(t, f); after != before {
		t.Fatalf("expired-session mutations committed state: before=%v after=%v", before, after)
	}
}
