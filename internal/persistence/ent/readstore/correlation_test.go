package readstore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

// TestNonTimeFiltersMatchTheSameContribution checks: Network A with Unit X and Network B
// with Unit Y in one Message.
func TestNonTimeFiltersMatchTheSameContribution(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	x, y := w.unit(agency, "X", nil), w.unit(agency, "Y", nil)
	networkA, networkB := w.network(agency, "A", "pocsag"), w.network(agency, "B", "pocsag")
	message := w.message(base, "text", "alarm")
	w.assigned(message, networkA, x, agency, base, "")
	w.assigned(message, networkB, y, agency, base.Add(time.Second), "")
	caller := all()
	for name, test := range map[string]struct {
		filters readmodel.Filters
		want    int
	}{
		"network A and unit Y":       {readmodel.Filters{Networks: []uuid.UUID{networkA}, Units: []uuid.UUID{y}}, 0},
		"network A and unit X":       {readmodel.Filters{Networks: []uuid.UUID{networkA}, Units: []uuid.UUID{x}}, 1},
		"network B and unit Y":       {readmodel.Filters{Networks: []uuid.UUID{networkB}, Units: []uuid.UUID{y}}, 1},
		"network A and units X or Y": {readmodel.Filters{Networks: []uuid.UUID{networkA}, Units: []uuid.UUID{x, y}}, 1},
		"unit Y alone":               {readmodel.Filters{Units: []uuid.UUID{y}}, 1},
		"protocol flex":              {readmodel.Filters{Protocols: []string{"flex"}}, 0},
		"protocol pocsag and unit Y": {readmodel.Filters{Protocols: []string{"pocsag"}, Units: []uuid.UUID{y}}, 1},
	} {
		if got := len(w.filtered(caller, base.Add(time.Hour), test.filters).Items); got != test.want {
			t.Errorf("%s: %d rows, want %d", name, got, test.want)
		}
	}
}

// TestUnitlessAppearanceNeedsSupportingNetworkAgency checks the Unit-less side of the
// correlation: Network and withoutUnit filters meet on the Agency of the targetless Transmission.
func TestUnitlessAppearanceNeedsSupportingNetworkAgency(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency, other := w.agency("county"), w.agency("other")
	network, foreign := w.network(agency, "N", "pocsag"), w.network(other, "M", "pocsag")
	message := w.message(base, "text", "alarm")
	w.transmission(transmissionSpec{message: message, network: network, accepted: base})
	w.appearance(message, nil, agency, base, base)
	caller := all()
	for name, test := range map[string]struct {
		filters readmodel.Filters
		want    int
	}{
		"without unit":                 {readmodel.Filters{WithoutUnit: true}, 1},
		"without unit and own network": {readmodel.Filters{WithoutUnit: true, Networks: []uuid.UUID{network}}, 1},
		"foreign network":              {readmodel.Filters{Networks: []uuid.UUID{foreign}}, 0},
		"agency":                       {readmodel.Filters{Agencies: []uuid.UUID{agency}}, 1},
		"other agency":                 {readmodel.Filters{Agencies: []uuid.UUID{other}}, 0},
		"organization":                 {readmodel.Filters{Organizations: []uuid.UUID{uuid.New()}}, 0},
	} {
		if got := len(w.filtered(caller, base.Add(time.Hour), test.filters).Items); got != test.want {
			t.Errorf("%s: %d rows, want %d", name, got, test.want)
		}
	}
}

// TestOrganizationFilterIntersectsGrants checks that an Organization filter only intersects the grants.
func TestOrganizationFilterIntersectsGrants(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	org := w.organization("O")
	a, b := w.unit(agency, "A", &org), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	m1, m2 := w.message(base, "text", "one"), w.message(base.Add(time.Minute), "text", "two")
	w.assigned(m1, network, a, agency, base, "")
	w.assigned(m2, network, b, agency, base.Add(time.Minute), "")
	caller := principal(grant(scope("unit", a, b), nil, "message.read"))
	if got := ids(w.filtered(caller, base.Add(time.Hour), readmodel.Filters{})); !slices.Equal(got, []uuid.UUID{m2, m1}) {
		t.Fatalf("without filter = %v", got)
	}
	if got := ids(w.filtered(caller, base.Add(time.Hour), readmodel.Filters{Organizations: []uuid.UUID{org}})); !slices.Equal(got, []uuid.UUID{m1}) {
		t.Fatalf("organization filter = %v", got)
	}
	narrow := principal(grant(scope("unit", b), nil, "message.read"))
	if got := w.filtered(narrow, base.Add(time.Hour), readmodel.Filters{Organizations: []uuid.UUID{org}}); len(got.Items) != 0 {
		t.Fatalf("filter must never widen grants: %v", ids(got))
	}
}

// TestUnitCollectionAndAgencyScopes checks every scope kind against the same Message.
func TestUnitCollectionAndAgencyScopes(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency, other := w.agency("county"), w.agency("other")
	unit := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	w.assigned(message, network, unit, agency, base, "")
	collection := w.collection(unit)
	for name, test := range map[string]struct {
		scope string
		refs  []uuid.UUID
		want  int
	}{
		"collection": {"unitCollection", []uuid.UUID{collection}, 1}, "other collection": {"unitCollection", []uuid.UUID{uuid.New()}, 0},
		"agency": {"agency", []uuid.UUID{agency}, 1}, "other agency": {"agency", []uuid.UUID{other}, 0},
		"unit": {"unit", []uuid.UUID{unit}, 1}, "other unit": {"unit", []uuid.UUID{uuid.New()}, 0},
	} {
		caller := principal(grant(scope(test.scope, test.refs...), nil, "message.read"))
		if got := len(w.feed(caller, base.Add(time.Hour), readmodel.FeedQuery{}).Items); got != test.want {
			t.Errorf("%s: %d rows, want %d", name, got, test.want)
		}
	}
}
