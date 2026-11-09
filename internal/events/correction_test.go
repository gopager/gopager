package events

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCorrectionAssignmentChangedCarriesChangedTargetsBeforeAndAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	unit, rule := uuid.New(), uuid.New()
	message := uuid.New()
	targets := []CorrectionTarget{
		{Position: 0, Before: IntakeResolution{Status: "unassigned"},
			After: IntakeResolution{Status: "assigned", UnitID: &unit, AddressRuleID: &rule}},
		{Position: 2, Before: IntakeResolution{Status: "assigned", UnitID: &unit, AddressRuleID: &rule},
			After: IntakeResolution{Status: "unassigned"}},
	}
	fact := CorrectionFact{ID: uuid.New(), Type: "assignment.changed",
		Aggregate: EntityRef{Type: "message", ID: message}, Revision: 3, OccurredAt: now,
		TransmissionID: uuid.New(), MessageID: message, NetworkID: uuid.New(), Targets: targets, Reason: "corrected", MessageTime: now}
	envelope, err := fact.Envelope(now)
	if err != nil || len(envelope.Data.Targets) != 2 || envelope.Data.Targets[1].Position != 2 {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
	for name, mutate := range map[string]func(*CorrectionFact){
		"no targets": func(f *CorrectionFact) { f.Targets = nil },
		"unordered":  func(f *CorrectionFact) { f.Targets = []CorrectionTarget{targets[1], targets[0]} },
		"unchanged": func(f *CorrectionFact) {
			f.Targets = []CorrectionTarget{{Position: 0, Before: targets[0].Before, After: targets[0].Before}}
		},
		"invalid after": func(f *CorrectionFact) {
			f.Targets = []CorrectionTarget{{Position: 0, Before: targets[0].Before, After: IntakeResolution{Status: "assigned"}}}
		},
		"wrong reason": func(f *CorrectionFact) { f.Reason = "created" },
	} {
		invalid := fact
		mutate(&invalid)
		if _, err := invalid.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
}

func TestCorrectionAssignmentChangedRecordsNetworkMove(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	message, previous, current := uuid.New(), uuid.New(), uuid.New()
	fact := CorrectionFact{ID: uuid.New(), Type: "assignment.changed",
		Aggregate: EntityRef{Type: "message", ID: message}, Revision: 2, OccurredAt: now,
		TransmissionID: uuid.New(), MessageID: message, NetworkID: current, PreviousNetworkID: &previous,
		Reason: "corrected", MessageTime: now}
	envelope, err := fact.Envelope(now)
	if err != nil || envelope.Data.PreviousNetworkID == nil || *envelope.Data.PreviousNetworkID != previous ||
		envelope.Data.NetworkID != current || len(envelope.Data.Targets) != 0 {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
	nilNetwork := uuid.Nil
	for name, mutate := range map[string]func(*CorrectionFact){
		"same network": func(f *CorrectionFact) { f.PreviousNetworkID = &current },
		"nil previous": func(f *CorrectionFact) { f.PreviousNetworkID = &nilNetwork },
	} {
		invalid := fact
		mutate(&invalid)
		if _, err := invalid.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
}

func TestCorrectionFailedAcceptsGatewayMappingReason(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	fact := CorrectionFact{ID: uuid.New(), Type: "correction.failed",
		Aggregate: EntityRef{Type: "correction", ID: uuid.New()}, Revision: 5, OccurredAt: now,
		Reason: "gateway-mapping-change", FailureCode: "RETRY_LIMIT"}
	if envelope, err := fact.Envelope(now); err != nil || envelope.Data.Reason != "gateway-mapping-change" {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
}
