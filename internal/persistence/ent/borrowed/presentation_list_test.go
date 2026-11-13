package borrowed_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// TestAssignmentListCarriesItsAuthorizationContext expects one listing to return every assignment
// with its Layout's Agencies and, for a Unit, the Unit's Agency and collections, and no
// assignment of a deleted scope.
func TestAssignmentListCarriesItsAuthorizationContext(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	first, second := w.agency(), w.agency()
	org := w.organization()
	unit := w.unit(first, uuid.Nil)
	gone := w.unit(first, uuid.Nil)
	collection := uuid.New()
	w.exec(`INSERT INTO unit_collections (id, name) VALUES ($1, 'Collection')`, collection)
	w.exec(`INSERT INTO unit_collection_memberships (collection_id, unit_id) VALUES ($1, $2)`, collection, unit)
	layout := w.layout("text", first, second)
	for _, scope := range []struct {
		kind string
		id   uuid.UUID
	}{{presentation.ScopeUnit, unit}, {presentation.ScopeUnit, gone}, {presentation.ScopeOrganization, org}} {
		if err := w.put(scope.kind, scope.id, "text", layout, 0); err != nil {
			t.Fatal(err)
		}
	}
	// Deleting a Unit removes its assignments (trigger); the listing never shows a deleted scope.
	w.exec(`UPDATE units SET deleted = true WHERE id = $1`, gone)

	var listed []presentation.ListedAssignment
	if err := w.within(func(tx platform.TransactionContext) error {
		var err error
		listed, err = w.layers.List(tx.Context(), tx, "", uuid.Nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ScopeKind != presentation.ScopeOrganization || listed[1].ScopeID != unit {
		t.Fatalf("listed = %+v, want the Organization and the live Unit", listed)
	}
	agencies := []uuid.UUID{first, second}
	slices.SortFunc(agencies, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	for _, entry := range listed {
		if !slices.Equal(entry.LayoutAgencies, agencies) {
			t.Fatalf("%s Layout Agencies = %v, want %v", entry.ScopeKind, entry.LayoutAgencies, agencies)
		}
	}
	if listed[1].Unit.AgencyID != first || !slices.Equal(listed[1].Unit.CollectionIDs, []uuid.UUID{collection}) {
		t.Fatalf("Unit context = %+v", listed[1].Unit)
	}
	if listed[0].Unit.AgencyID != uuid.Nil || len(listed[0].Unit.CollectionIDs) != 0 {
		t.Fatalf("Organization carries a Unit context: %+v", listed[0].Unit)
	}
}
