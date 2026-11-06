package borrowed_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// TestAgencyLifecycleAndScope expects Agencies to be created, read, listed, renamed and deleted
// through the master-data service, a Unit to require an existing Agency, an Agency with
// Units to refuse deletion, and an Agency grant to scope Unit reads to that Agency.
func TestAgencyLifecycleAndScope(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	north, err := f.units.CreateAgency(t.Context(), f.proof, uuid.New(), units.AgencyInput{Name: "North"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	south, err := f.units.CreateAgency(t.Context(), f.proof, uuid.New(), units.AgencyInput{Name: "South"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	agency, err := f.units.ReadAgency(t.Context(), f.proof, north.ID)
	if err != nil || agency.Name != "North" || agency.Revision != 1 {
		t.Fatalf("read agency=%+v err=%v", agency, err)
	}
	page, err := f.units.ListAgencies(t.Context(), f.proof, units.PageInput{Limit: 10})
	if err != nil || page.Total != len(f.agencies)+2 {
		t.Fatalf("list agencies total=%d err=%v, want the fixture Agencies plus two", page.Total, err)
	}
	renamed, err := f.units.ChangeAgency(t.Context(), f.proof, north.ID, 1, units.AgencyInput{Name: "North County"}, checkedMutation)
	if err != nil || renamed.Revision != 2 {
		t.Fatalf("rename agency=%+v err=%v", renamed, err)
	}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{Name: "Station", AgencyID: north.ID, CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.units.ReadUnit(t.Context(), f.proof, unit.ID)
	if err != nil || read.AgencyID != north.ID {
		t.Fatalf("unit agency=%s err=%v, want %s", read.AgencyID, err, north.ID)
	}
	if _, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{Name: "Orphan", AgencyID: uuid.New(), CollectionIDs: []uuid.UUID{}}, checkedMutation); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("unit with an unknown Agency: %v, want invalid input", err)
	}
	if _, err := f.units.DeleteAgency(t.Context(), f.proof, north.ID, 2, checkedMutation); !errors.Is(err, units.ErrAgencyInUse) {
		t.Fatalf("deleting an Agency with a Unit: %v, want in use", err)
	}
	if _, err := f.units.DeleteAgency(t.Context(), f.proof, south.ID, 1, checkedMutation); err != nil {
		t.Fatalf("deleting an empty Agency: %v", err)
	}
	if _, err := f.units.ReadAgency(t.Context(), f.proof, south.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("deleted Agency still readable: %v", err)
	}

	east, err := f.units.CreateAgency(t.Context(), f.proof, uuid.New(), units.AgencyInput{Name: "East"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		scopes := []authz.Scope{
			{Kind: "agency", References: []string{north.ID.String()}},
			{Kind: "agency", References: []string{south.ID.String()}},
			{Kind: "agency", References: []string{north.ID.String(), uuid.NewString()}},
		}
		existing, err := f.unitStore.ScopesExist(tx.Context(), tx, scopes)
		if err != nil {
			return err
		}
		if want := []bool{true, false, false}; !slices.Equal(existing, want) {
			t.Fatalf("ScopesExist(%v)=%v want %v", scopes, existing, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A reader limited to East sees no Unit; limited to North sees the Unit.
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"unit.read", "agency.read"},
		Scope: authz.Scope{Kind: "agency", References: []string{east.ID.String()}}}})
	if page, err := f.units.ListUnits(t.Context(), f.proof, units.PageInput{Limit: 10}); err != nil || page.Total != 0 {
		t.Fatalf("East reader lists %d Units err=%v, want 0", page.Total, err)
	}
	if _, err := f.units.ReadUnit(t.Context(), f.proof, unit.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("East reader read a North Unit: %v", err)
	}
	if _, err := f.units.ReadAgency(t.Context(), f.proof, north.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("East reader read the North Agency: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"unit.read", "unit.update", "agency.read"},
		Scope: authz.Scope{Kind: "agency", References: []string{north.ID.String()}}}})
	if page, err := f.units.ListUnits(t.Context(), f.proof, units.PageInput{Limit: 10}); err != nil || page.Total != 1 {
		t.Fatalf("North reader lists %d Units err=%v, want 1", page.Total, err)
	}
	// Moving the Unit to East needs update rights on both Agencies.
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, 1,
		units.UnitInput{Name: "Station", AgencyID: east.ID, CollectionIDs: []uuid.UUID{}}, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("North-only manager moved a Unit into East: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: authz.Actions(), Scope: authz.Scope{Kind: "all"}}})
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, 1,
		units.UnitInput{Name: "Station", AgencyID: east.ID, CollectionIDs: []uuid.UUID{}}, checkedMutation); err != nil {
		t.Fatal(err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"unit.read"},
		Scope: authz.Scope{Kind: "agency", References: []string{north.ID.String()}}}})
	if page, err := f.units.ListUnits(t.Context(), f.proof, units.PageInput{Limit: 10}); err != nil || page.Total != 0 {
		t.Fatalf("North reader still lists %d Units after the move err=%v, want 0", page.Total, err)
	}
}
