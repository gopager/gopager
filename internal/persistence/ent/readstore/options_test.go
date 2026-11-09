package readstore_test

import (
	"context"
	"github.com/gopager/gopager/internal/platform"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/readmodel"
)

func (w *world) options(authority authz.Authority) readmodel.OptionsRecord {
	w.t.Helper()
	access, err := authz.NewMessageAccess(authority, base)
	if err != nil {
		w.t.Fatal(err)
	}
	var record readmodel.OptionsRecord
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		var err error
		record, err = w.reads.Options(ctx, tx, readmodel.OptionsQuery{Access: access, Units: true, Organizations: true, Agencies: true, Networks: true})
		if err != nil {
			w.t.Fatal(err)
		}
	})
	return record
}

func optionIDs(options []readmodel.NamedOption) []uuid.UUID {
	var out []uuid.UUID
	for _, option := range options {
		out = append(out, option.ID)
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return out
}

func sorted(ids ...uuid.UUID) []uuid.UUID {
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return ids
}

// TestFilterOptionsFollowTheReadScopes checks the option scope rules.
func TestFilterOptionsFollowTheReadScopes(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	county, other := w.agency("county"), w.agency("other")
	orgA, orgC := w.organization("OA"), w.organization("OC")
	unused := w.organization("unused")
	a, b := w.unit(county, "A", &orgA), w.unit(county, "B", nil)
	c := w.unit(other, "C", &orgC)
	deleted := w.unit(county, "gone", nil)
	w.exec(`UPDATE units SET deleted = true WHERE id = $1`, deleted)
	networkCounty, networkOther := w.network(county, "N1", "pocsag"), w.network(other, "N2", "flex")
	collection := w.collection(a)

	t.Run("unit scopes reach units, their organizations and agencies, no networks", func(t *testing.T) {
		t.Parallel()
		record := w.options(principal(grant(scope("unit", a, b), nil, "message.read")))
		if got := []uuid.UUID{record.Units[0].ID, record.Units[1].ID}; !slices.Equal(sorted(got...), sorted(a, b)) || len(record.Units) != 2 {
			t.Fatalf("units = %+v", record.Units)
		}
		if !slices.Equal(optionIDs(record.Organizations), sorted(orgA)) || !slices.Equal(optionIDs(record.Agencies), sorted(county)) || len(record.Networks) != 0 {
			t.Fatalf("options = %+v", record)
		}
	})
	t.Run("collection scope", func(t *testing.T) {
		t.Parallel()
		record := w.options(principal(grant(scope("unitCollection", collection), nil, "message.read")))
		if len(record.Units) != 1 || record.Units[0].ID != a || len(record.Networks) != 0 {
			t.Fatalf("options = %+v", record)
		}
	})
	t.Run("agency scope adds the agency's networks", func(t *testing.T) {
		t.Parallel()
		record := w.options(principal(grant(scope("agency", county), nil, "message.read")))
		if len(record.Units) != 2 || !slices.Equal(optionIDs(record.Networks), sorted(networkCounty)) ||
			!slices.Equal(optionIDs(record.Organizations), sorted(orgA)) || !slices.Equal(optionIDs(record.Agencies), sorted(county)) {
			t.Fatalf("options = %+v", record)
		}
	})
	t.Run("all scope lists every non-deleted object", func(t *testing.T) {
		t.Parallel()
		record := w.options(all())
		if len(record.Units) != 3 || !slices.Equal(optionIDs(record.Networks), sorted(networkCounty, networkOther)) ||
			!slices.Equal(optionIDs(record.Organizations), sorted(orgA, orgC, unused)) ||
			!slices.Equal(optionIDs(record.Agencies), sorted(county, other)) {
			t.Fatalf("options = %+v", record)
		}
	})
	t.Run("key intersects", func(t *testing.T) {
		t.Parallel()
		keyed := all()
		keyed.Key = &authz.Restrictions{Actions: []string{"message.read"}, Scopes: []authz.Scope{scope("agency", other)}}
		record := w.options(keyed)
		if len(record.Units) != 1 || record.Units[0].ID != c || !slices.Equal(optionIDs(record.Networks), sorted(networkOther)) ||
			!slices.Equal(optionIDs(record.Organizations), sorted(orgC)) {
			t.Fatalf("options = %+v", record)
		}
	})
	t.Run("other reads open no options", func(t *testing.T) {
		t.Parallel()
		record := w.options(principal(grant(scope("unit", uuid.New()), nil, "message.read")))
		if len(record.Units)+len(record.Organizations)+len(record.Agencies)+len(record.Networks) != 0 {
			t.Fatalf("options = %+v", record)
		}
	})
}
