package borrowed_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/units"
)

func TestRuleEncryptedMarkerIsLabelOnlyAndQueuesNoCorrection(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Station", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	values := ruleInput(f.networks[0], unit.ID, 10, 20, 1)
	created, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), values, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.rules.ReadRule(t.Context(), f.proof, created.ID)
	if err != nil || read.Encrypted {
		t.Fatalf("rule created without marker is encrypted or unreadable: %+v %v", read, err)
	}

	before := configurationCounts(t, f)
	values.Encrypted = true
	updated, err := f.rules.UpdateRule(t.Context(), f.proof, created.ID, created.Revision, values, checkedMutation)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("marker update failed: %+v %v", updated, err)
	}
	read, err = f.rules.ReadRule(t.Context(), f.proof, created.ID)
	if err != nil || !read.Encrypted || read.Revision != 2 {
		t.Fatalf("marker not persisted: %+v %v", read, err)
	}
	after := configurationCounts(t, f)
	if after[5] != before[5] || after[6] != before[6] {
		t.Fatalf("marker-only update queued a correction or bumped the assignment revision: %v -> %v", before, after)
	}
	if after[2] != before[2]+1 || after[3] != before[3]+1 {
		t.Fatalf("marker-only update did not record audit and configuration fact: %v -> %v", before, after)
	}

	values.Selector.Functions = []uint16{1, 2}
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, created.ID, updated.Revision, values, checkedMutation); err != nil {
		t.Fatal(err)
	}
	if again := configurationCounts(t, f); again[5] <= after[5] {
		t.Fatalf("selector change queued no correction scope: %v -> %v", after, again)
	}
}
