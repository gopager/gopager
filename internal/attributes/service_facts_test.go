package attributes

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
)

func unitInput(key string, agencies ...uuid.UUID) SchemaInput {
	in := input(key, agencies...)
	in.Subject = SubjectUnit
	return in
}

func TestUpdateRecordsAgencyChangeOfUnitSchemaOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, store := serviceFor(t, agencyScope(agencyA, agencyB), ManageAction)
	if _, err := service.Create(ctx, identity.Proof{}, uuid.New(), unitInput("site", agencyA), allow); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, identity.Proof{}, uuid.New(), input("alarm", agencyA), allow); err != nil {
		t.Fatal(err)
	}

	renamed := unitInput("site", agencyA)
	renamed.Label = "Renamed"
	if _, err := service.Update(ctx, identity.Proof{}, "site", 1, renamed, allow); err != nil || len(store.facts) != 0 {
		t.Fatalf("an update that keeps the Agencies recorded %d facts, err %v", len(store.facts), err)
	}
	if _, err := service.Update(ctx, identity.Proof{}, "alarm", 1, input("alarm", agencyA, agencyB), allow); err != nil || len(store.facts) != 0 {
		t.Fatalf("an Agency change of a message schema recorded %d facts, err %v", len(store.facts), err)
	}

	if _, err := service.Update(ctx, identity.Proof{}, "site", 2, unitInput("site", agencyA, agencyB), allow); err != nil || len(store.facts) != 1 {
		t.Fatalf("adding an Agency recorded %d facts, err %v", len(store.facts), err)
	}
	if _, err := service.Update(ctx, identity.Proof{}, "site", 3, unitInput("site", agencyB, agencyA), allow); err != nil || len(store.facts) != 1 {
		t.Fatalf("the same Agencies in another order recorded %d facts, err %v", len(store.facts), err)
	}
	updated, err := service.Update(ctx, identity.Proof{}, "site", 4, unitInput("site", agencyB), allow)
	if err != nil || len(store.facts) != 2 {
		t.Fatalf("removing an Agency recorded %d facts, err %v", len(store.facts), err)
	}
	fact := store.facts[1]
	if fact.Aggregate.Type != "attributeSchema" || fact.Aggregate.ID != schemaID("site") ||
		fact.Revision != updated.Revision || fact.ChangeKind != "updated" || !fact.OccurredAt.Equal(updated.UpdatedAt) {
		t.Fatalf("fact = %+v", fact)
	}
}

// TestDeleteRecordsUnitSchemaDeletionOnly expects deleting a unit schema, whose entries leave
// deleted Units, to record one fact, and deleting a message schema none.
func TestDeleteRecordsUnitSchemaDeletionOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, store := serviceFor(t, agencyScope(agencyA), ManageAction)
	for _, schema := range []SchemaInput{unitInput("site", agencyA), input("alarm", agencyA)} {
		if _, err := service.Create(ctx, identity.Proof{}, uuid.New(), schema, allow); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Delete(ctx, identity.Proof{}, "alarm", 1, allow); err != nil || len(store.facts) != 0 {
		t.Fatalf("deleting a message schema recorded %d facts, err %v", len(store.facts), err)
	}
	deleted, err := service.Delete(ctx, identity.Proof{}, "site", 1, allow)
	if err != nil || len(store.facts) != 1 {
		t.Fatalf("deleting a unit schema recorded %d facts, err %v", len(store.facts), err)
	}
	if fact := store.facts[0]; fact.Aggregate.Type != "attributeSchema" || fact.Aggregate.ID != schemaID("site") ||
		fact.Revision != deleted.Revision || fact.ChangeKind != "deleted" {
		t.Fatalf("fact = %+v", fact)
	}
}
