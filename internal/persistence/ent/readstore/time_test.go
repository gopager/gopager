package readstore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

func at(h, m, s int) time.Time { return time.Date(2026, 11, 8, h, m, s, 0, time.UTC) }

// TestMessageTimeFiltersUseTheStoredMessageTime checks: the interval never looks at the
// per-Unit first times, whatever Unit filter accompanies it.
func TestMessageTimeFiltersUseTheStoredMessageTime(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(at(12, 0, 0), "text", "alarm")
	w.assigned(message, network, a, agency, at(12, 0, 0), "")
	w.assigned(message, network, b, agency, at(12, 0, 20), "")
	from, until := at(12, 0, 10), at(12, 0, 30)
	caller := all()
	now := at(13, 0, 0)
	for name, units := range map[string][]uuid.UUID{"no unit filter": nil, "A": {a}, "B": {b}, "A or B": {a, b}} {
		result := w.filtered(caller, now, readmodel.Filters{From: &from, Until: &until, Units: units})
		if len(result.Items) != 0 {
			t.Errorf("%s: the Message at 12:00:00 must be outside [12:00:10, 12:00:30)", name)
		}
	}
	result := w.filtered(caller, now, readmodel.Filters{})
	if len(result.Items) != 1 || !result.Items[0].Time.Equal(at(12, 0, 0)) {
		t.Fatalf("unfiltered = %+v", result.Items)
	}
	detail, err := w.detail(caller, now, message)
	if err != nil {
		t.Fatal(err)
	}
	first := map[string]time.Time{}
	for _, unit := range detail.Units {
		first[unit.Name] = unit.FirstObservedAt
	}
	if !first["A"].Equal(at(12, 0, 0)) || !first["B"].Equal(at(12, 0, 20)) {
		t.Fatalf("per-Unit first times = %v", first)
	}
	// The interval is half-open: a Message exactly at until is outside, at from inside.
	atFrom, atUntil := at(12, 0, 0), at(12, 0, 0)
	if got := w.filtered(caller, now, readmodel.Filters{From: &atFrom}).Items; len(got) != 1 {
		t.Errorf("from is inclusive: %d", len(got))
	}
	if got := w.filtered(caller, now, readmodel.Filters{Until: &atUntil}).Items; len(got) != 0 {
		t.Errorf("until is exclusive: %d", len(got))
	}
}

// TestNetworkFilterNeverChangesTheMessageTime checks:
func TestNetworkFilterNeverChangesTheMessageTime(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a := w.unit(agency, "A", nil)
	n1, n2 := w.network(agency, "N1", "pocsag"), w.network(agency, "N2", "pocsag")
	message := w.message(at(12, 0, 0), "text", "alarm")
	w.assigned(message, n1, a, agency, at(12, 0, 0), "")
	second := w.transmission(transmissionSpec{message: message, network: n2, accepted: at(12, 0, 20)})
	w.target(second, 0, targetSpec{unit: &a, address: 2000})
	from, until := at(12, 0, 10), at(12, 0, 30)
	now := at(13, 0, 0)
	for name, networks := range map[string][]uuid.UUID{"without network filter": nil, "excluding network": {n1}, "later network": {n2}} {
		result := w.filtered(all(), now, readmodel.Filters{From: &from, Until: &until, Networks: networks})
		if len(result.Items) != 0 {
			t.Errorf("%s: the Message time 12:00:00 is outside the interval", name)
		}
	}
	detail, err := w.detail(all(), now, message)
	if err != nil || len(detail.Units) != 1 || !detail.Units[0].FirstObservedAt.Equal(at(12, 0, 0)) {
		t.Fatalf("detail = %+v, %v", detail.Units, err)
	}
}

// TestHiddenMemberMayDefineTheMessageTime checks:
func TestHiddenMemberMayDefineTheMessageTime(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(at(12, 0, 0), "text", "alarm")
	w.assigned(message, network, a, agency, at(12, 0, 0), `{"form":"text","value":"alarm"}`)
	w.assigned(message, network, b, agency, at(12, 0, 20), `{"form":"text","value":"alarm "}`)
	onlyB := principal(grant(scope("unit", b), nil, "message.read", "message.detail"))
	result := w.feed(onlyB, at(13, 0, 0), readmodel.FeedQuery{})
	if len(result.Items) != 1 || !result.Items[0].Time.Equal(at(12, 0, 0)) ||
		!slices.Equal(unitNames(result.Items[0]), []string{"B"}) || len(result.Items[0].WithoutUnit) != 0 {
		t.Fatalf("B-only row = %+v", result.Items)
	}
	if rep := result.Items[0].Representative; rep == nil || string(rep.Payload) != `{"form":"text","value":"alarm "}` {
		t.Fatalf("representative must come from B's Transmission: %+v", rep)
	}
	detail, err := w.detail(onlyB, at(13, 0, 0), message)
	if err != nil || len(detail.Units) != 1 || len(detail.Transmissions) != 1 || len(detail.Transmissions[0].Targets) != 1 {
		t.Fatalf("B-only detail = %+v, %v", detail, err)
	}
	neither := principal(grant(scope("unit", uuid.New()), nil, "message.read"))
	if got := w.feed(neither, at(13, 0, 0), readmodel.FeedQuery{}); len(got.Items) != 0 {
		t.Fatalf("hidden message appeared: %v", ids(got))
	}
}

// TestAgeIsEvaluatedPerAppearanceAnchor checks: a repeat renews
// nothing and another Unit's recent anchor lends no age.
func TestAgeIsEvaluatedPerAppearanceAnchor(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(at(12, 0, 0), "text", "alarm")
	w.assigned(message, network, a, agency, at(12, 0, 0), `{"form":"text","value":"for A"}`)
	w.assigned(message, network, b, agency, at(12, 0, 50), `{"form":"text","value":"for B"}`)
	now := at(12, 1, 0)
	thirty := principal(grant(scope("all"), seconds(30), "message.read", "message.detail"))
	result := w.feed(thirty, now, readmodel.FeedQuery{})
	if len(result.Items) != 1 || !slices.Equal(unitNames(result.Items[0]), []string{"B"}) {
		t.Fatalf("units = %+v", result.Items)
	}
	if string(result.Items[0].Representative.Payload) != `{"form":"text","value":"for B"}` {
		t.Fatalf("A's expired Transmission must not represent the Message: %s", result.Items[0].Representative.Payload)
	}
	if detail, _ := w.detail(thirty, now, message); len(detail.Transmissions) != 1 {
		t.Fatalf("detail transmissions = %d", len(detail.Transmissions))
	}
	// The cutoff is inclusive: A's anchor 12:00:00 is exactly now-60s.
	sixty := principal(grant(scope("all"), seconds(60), "message.read"))
	if got := w.feed(sixty, now, readmodel.FeedQuery{}).Items[0].Units; len(got) != 2 {
		t.Fatalf("anchor at the cutoff must be eligible: %d units", len(got))
	}
	fiftyNine := principal(grant(scope("all"), seconds(59), "message.read"))
	if got := w.feed(fiftyNine, now, readmodel.FeedQuery{}).Items[0].Units; len(got) != 1 {
		t.Fatalf("anchor before the cutoff must not be eligible: %d units", len(got))
	}
	// Separate grants keep their own age: B's unlimited grant does not lend A its age.
	split := principal(grant(scope("unit", a), seconds(30), "message.read"), grant(scope("unit", b), nil, "message.read"))
	if got := w.feed(split, now, readmodel.FeedQuery{}).Items[0].Units; len(got) != 1 || got[0].Name != "B" {
		t.Fatalf("split grants: %+v", got)
	}
	// Default unlimited age still shows first times.
	if got := w.feed(all(), now, readmodel.FeedQuery{}).Items[0].Units; len(got) != 2 {
		t.Fatalf("unlimited: %d", len(got))
	}
}
