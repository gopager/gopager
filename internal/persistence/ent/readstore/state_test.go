package readstore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

// TestAssignmentStateIsDistinguishable checks: an encrypted rule, an unassigned target, a
// failed target and a correcting Message.
func TestAssignmentStateIsDistinguishable(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	rule := uuid.New()
	w.exec(`INSERT INTO address_rules (id, network_id, unit_id, protocol, selector, encrypted)
		VALUES ($1, $2, $3, 'pocsag', '{}', true)`, rule, network, a)
	message := w.message(base, "text", "alarm")
	transmission := w.transmission(transmissionSpec{message: message, network: network, accepted: base, unresolved: true,
		quality: `{"rssi":-80}`})
	w.target(transmission, 0, targetSpec{unit: &a, address: 1000, function: 2, rule: &rule})
	w.target(transmission, 1, targetSpec{status: "unassigned", address: 1001})
	w.target(transmission, 2, targetSpec{status: "failed", address: 1002})
	w.appearance(message, &a, agency, base, base)
	w.appearance(message, nil, agency, base, base)
	w.correcting(message)
	caller := all()
	now := base.Add(time.Hour)
	item := w.feed(caller, now, readmodel.FeedQuery{}).Items[0]
	if item.CorrectionStatus != "correcting" || len(item.WithoutUnit) != 1 || len(item.Units) != 1 || !item.Units[0].Encrypted {
		t.Fatalf("feed item = %+v", item)
	}
	if !slices.Equal(item.UnresolvedAgencies, []uuid.UUID{agency}) {
		t.Fatalf("unresolved agencies = %v", item.UnresolvedAgencies)
	}
	detail, err := w.detail(caller, now, message)
	if err != nil || len(detail.Transmissions) != 1 {
		t.Fatalf("detail = %+v, %v", detail, err)
	}
	var resolutions []string
	for _, target := range detail.Transmissions[0].Targets {
		resolutions = append(resolutions, target.Resolution)
	}
	if !slices.Equal(resolutions, []string{"assigned", "unassigned", "failed"}) {
		t.Fatalf("resolutions = %v", resolutions)
	}
	first := detail.Transmissions[0].Targets[0]
	if !first.Encrypted || first.AddressNumber == nil || *first.AddressNumber != 1000 || first.Function == nil || *first.Function != 2 {
		t.Fatalf("assigned target = %+v", first)
	}
	if string(detail.Transmissions[0].QualityMetrics) != `{"rssi":-80}` {
		t.Fatalf("quality = %s", detail.Transmissions[0].QualityMetrics)
	}
	// The failed target needs message.read and message.detail on the Network's Agency: a Unit
	// scope cannot see it.
	unitOnly := principal(grant(scope("unit", a), nil, "message.read", "message.detail"))
	narrow, err := w.detail(unitOnly, now, message)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range narrow.Transmissions[0].Targets {
		if target.Resolution != "assigned" {
			t.Errorf("unit scope sees %s target", target.Resolution)
		}
	}
}

// TestProcessingStatusAggregatesSupportingTransmissions checks pending over failed over completed.
func TestProcessingStatusAggregatesSupportingTransmissions(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	for name, statuses := range map[string][]string{
		"pending wins": {"completed", "pending", "failed"}, "failed over completed": {"completed", "failed"},
		"completed": {"completed", "none"}, "none": {"none"},
	} {
		message := w.message(base.Add(time.Duration(len(name))*time.Minute), "text", name)
		for i, status := range statuses {
			id := w.transmission(transmissionSpec{message: message, network: network, accepted: base.Add(time.Duration(i) * time.Second), status: status})
			unit := a
			if i%2 == 1 {
				unit = b
			}
			w.target(id, 0, targetSpec{unit: &unit, address: int64(i + 1)})
		}
		w.appearance(message, &a, agency, base, base)
		w.appearance(message, &b, agency, base, base)
		got := w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{IDs: []uuid.UUID{message}}).Items[0].ProcessingStatus
		want := map[string]string{"pending wins": "pending", "failed over completed": "failed", "completed": "completed", "none": "none"}[name]
		if got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
}

// TestRepresentativeOrdersIncompleteLast checks the Representative Transmission rule.
func TestRepresentativeOrdersIncompleteLast(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	early := w.transmission(transmissionSpec{message: message, network: network, accepted: base, incomplete: true,
		payload: `{"form":"text","value":"early incomplete"}`})
	late := w.transmission(transmissionSpec{message: message, network: network, accepted: base.Add(time.Minute),
		payload: `{"form":"text","value":"late complete"}`})
	for _, id := range []uuid.UUID{early, late} {
		w.target(id, 0, targetSpec{unit: &a, address: 1})
	}
	w.appearance(message, &a, agency, base, base)
	got := w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{}).Items[0].Representative
	if got == nil || string(got.Payload) != `{"form":"text","value":"late complete"}` || got.Incomplete {
		t.Fatalf("representative = %+v", got)
	}
}
