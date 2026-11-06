package borrowed_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/units"
)

// TestUnitMoveNeedsAuthorityOverBothAgencies expects a Unit move between Agencies to require
// unit.update authority over the old and the new Agency, because the move hands the Unit's
// whole history to the new Agency; a grant on the Unit alone may edit it but not move it.
func TestUnitMoveNeedsAuthorityOverBothAgencies(t *testing.T) {
	f := newMatcherFixture(t)
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[0], Name: "Station", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	actions := []string{"unit.read", "unit.update"}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: actions,
		Scope: authz.Scope{Kind: "unit", References: []string{unit.ID.String()}}}})
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, unit.Revision,
		units.UnitInput{AgencyID: f.agencies[0], Name: "Renamed", CollectionIDs: []uuid.UUID{}}, checkedMutation); err != nil {
		t.Fatalf("a Unit grant could not rename its Unit: %v", err)
	}
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, unit.Revision+1,
		units.UnitInput{AgencyID: f.agencies[1], Name: "Renamed", CollectionIDs: []uuid.UUID{}}, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("a Unit grant moved its Unit into another Agency: %v, want forbidden", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{
		{Actions: actions, Scope: authz.Scope{Kind: "unit", References: []string{unit.ID.String()}}},
		{Actions: actions, Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[1].String()}}},
	})
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, unit.Revision+1,
		units.UnitInput{AgencyID: f.agencies[1], Name: "Renamed", CollectionIDs: []uuid.UUID{}}, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("a move without authority over the old Agency succeeded: %v, want forbidden", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: actions,
		Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String(), f.agencies[1].String()}}}})
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, unit.Revision+1,
		units.UnitInput{AgencyID: f.agencies[1], Name: "Renamed", CollectionIDs: []uuid.UUID{}}, checkedMutation); err != nil {
		t.Fatalf("a move with authority over both Agencies failed: %v", err)
	}
}
