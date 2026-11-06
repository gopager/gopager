package borrowed_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/units"
)

// TestUnitGrantStillDetectsHiddenConflictOnTheUnitsOwnAgencyNetwork covers the case that
// TestCurrentMatcherNativeMixedScopesAndRestrictingKeysMatchPointDecisions can no longer reach
// (a Unit-scoped key is refused outright on another Agency's Network): a manager who may read
// only one Unit of Agency A and may create rules on A's Network must still get a conflict when the
// new rule overlaps a rule of another Unit of A that the manager cannot read.
func TestUnitGrantStillDetectsHiddenConflictOnTheUnitsOwnAgencyNetwork(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	create := func(name string) commands.Result {
		t.Helper()
		result, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
			units.UnitInput{AgencyID: f.agencies[0], Name: name, CollectionIDs: []uuid.UUID{}}, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	own, hidden := create("Own"), create("Hidden")
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], hidden.ID, 50, 50), checkedMutation); err != nil {
		t.Fatal(err)
	}
	replaceManagementGrants(t, f, []authz.Grant{
		{Actions: []string{"unit.read", "address-rule.read", "address-rule.create"},
			Scope: authz.Scope{Kind: "unit", References: []string{own.ID.String()}}},
		{Actions: []string{"address-rule.create"},
			Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}}},
	})
	if err := f.rules.Preview(t.Context(), f.proof, uuid.Nil, 0, ruleInput(f.networks[0], own.ID, 50, 50), nil); !errors.Is(err, assignments.ErrRuleConflict) {
		t.Fatalf("hidden overlapping rule on the Unit's own Agency Network was ignored: %v", err)
	}
	if err := f.rules.Preview(t.Context(), f.proof, uuid.Nil, 0, ruleInput(f.networks[0], own.ID, 60, 60), nil); err != nil {
		t.Fatalf("non-overlapping preview was refused: %v", err)
	}
}
