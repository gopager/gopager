package borrowed_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/units"
)

// TestRuleWritesNeedAuthorityOverTheNetworkAgency expects a Unit-scoped address-rule grant to be
// insufficient for a Network of another Agency: otherwise a manager of one Unit of Agency B could
// claim addresses on Agency A's Network. An additional Agency grant on the Network's Agency allows it.
func TestRuleWritesNeedAuthorityOverTheNetworkAgency(t *testing.T) {
	f := newMatcherFixture(t)
	unitB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[1], Name: "Unit of B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	// A rule on Agency A's Network that already points at the Unit of B, created with full authority.
	existing, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unitB.ID, 50, 60), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	actions := []string{"unit.read", "address-rule.read", "address-rule.create", "address-rule.update", "address-rule.delete"}
	unitGrant := authz.Grant{Actions: actions, Scope: authz.Scope{Kind: "unit", References: []string{unitB.ID.String()}}}
	replaceManagementGrants(t, f, []authz.Grant{unitGrant})
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unitB.ID, 10, 20), checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("create on a Network of another Agency with a Unit grant = %v, want forbidden", err)
	}
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, existing.ID, existing.Revision, ruleInput(f.networks[0], unitB.ID, 51, 60), checkedMutation); err == nil {
		t.Fatal("update of a rule on a Network of another Agency succeeded with a Unit grant only")
	}
	if _, err := f.rules.DeleteRule(t.Context(), f.proof, existing.ID, existing.Revision, checkedMutation); err == nil {
		t.Fatal("delete of a rule on a Network of another Agency succeeded with a Unit grant only")
	}
	replaceManagementGrants(t, f, []authz.Grant{unitGrant,
		{Actions: actions, Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}}}})
	created, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unitB.ID, 10, 20), checkedMutation)
	if err != nil {
		t.Fatalf("create with the Unit grant and an Agency grant on the Network's Agency: %v", err)
	}
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, created.ID, created.Revision, ruleInput(f.networks[0], unitB.ID, 11, 20), checkedMutation); err != nil {
		t.Fatalf("update with both grants: %v", err)
	}
}
