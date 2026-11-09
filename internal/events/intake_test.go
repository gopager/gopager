package events

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntakeFactEnvelopeUsesTypedSafeVocabularies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	message, transmission := uuid.New(), uuid.New()
	envelope, err := (IntakeFact{ID: uuid.New(), Type: MessageMerged, MessageID: message,
		TransmissionID: transmission, Revision: 2, OccurredAt: now, MessageTime: now,
		Absorbed: []uuid.UUID{uuid.New(), uuid.New()}}).Envelope(now)
	if err != nil || envelope.AggregateRef.ID != message ||
		envelope.Type != MessageMerged || len(envelope.Data.Absorbed) != 2 {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
	if _, err := (IntakeFact{ID: uuid.New(), Type: MessageMerged, MessageID: message,
		TransmissionID: transmission, Revision: 2, OccurredAt: now, MessageTime: now,
		Absorbed: []uuid.UUID{message}}).Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("accepted self-merge: %v", err)
	}
}

func TestIntakeFactRejectsIncompleteGatewayTransitionAndAssignment(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	gateway := uuid.New()
	if _, err := (IntakeFact{ID: uuid.New(), Type: GatewayStateChanged, GatewayID: gateway,
		Revision: 1, OccurredAt: now}).Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("accepted incomplete Gateway transition: %v", err)
	}
	assigned := IntakeResolution{Status: "assigned"}
	if _, err := (IntakeFact{ID: uuid.New(), Type: AssignmentChanged, MessageID: uuid.New(),
		TransmissionID: uuid.New(), Targets: []IntakeTarget{{Position: 0, IntakeResolution: assigned}},
		Reason: "created", Revision: 1, OccurredAt: now, MessageTime: now}).Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("accepted assignment without Unit/rule: %v", err)
	}
}

func TestAssignmentChangedCarriesAllTargetsOfOneTransmission(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	unit, rule := uuid.New(), uuid.New()
	targets := []IntakeTarget{
		{Position: 0, IntakeResolution: IntakeResolution{Status: "assigned", UnitID: &unit, AddressRuleID: &rule}},
		{Position: 1, IntakeResolution: IntakeResolution{Status: "unassigned"}},
	}
	fact := IntakeFact{ID: uuid.New(), Type: AssignmentChanged, MessageID: uuid.New(),
		TransmissionID: uuid.New(), Targets: targets, Reason: "created", Revision: 1, OccurredAt: now, MessageTime: now}
	envelope, err := fact.Envelope(now)
	if err != nil || len(envelope.Data.Targets) != 2 || envelope.Data.Targets[1].Position != 1 {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
	for name, mutate := range map[string]func(*IntakeFact){
		"no targets":     func(f *IntakeFact) { f.Targets = nil },
		"unordered":      func(f *IntakeFact) { f.Targets = []IntakeTarget{targets[1], targets[0]} },
		"duplicate":      func(f *IntakeFact) { f.Targets = []IntakeTarget{targets[0], targets[0]} },
		"unknown reason": func(f *IntakeFact) { f.Reason = "other" },
		"negative position": func(f *IntakeFact) {
			f.Targets = []IntakeTarget{{Position: -1, IntakeResolution: targets[1].IntakeResolution}}
		},
	} {
		invalid := fact
		mutate(&invalid)
		if _, err := invalid.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
}
