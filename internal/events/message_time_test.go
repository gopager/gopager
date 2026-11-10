package events

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntakeMessageTimeRules(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)
	unit, rule := uuid.New(), uuid.New()
	base := func(factType IntakeType) IntakeFact {
		fact := IntakeFact{ID: uuid.New(), Type: factType, MessageID: uuid.New(), TransmissionID: uuid.New(),
			Revision: 1, OccurredAt: now, MessageTime: now}
		switch factType {
		case MessageMerged:
			fact.Absorbed = []uuid.UUID{uuid.New()}
		case AssignmentChanged:
			fact.Reason = "created"
			fact.Targets = []IntakeTarget{{Position: 0, IntakeResolution: IntakeResolution{
				Status: "assigned", UnitID: &unit, AddressRuleID: &rule}}}
		}
		return fact
	}
	for _, factType := range []IntakeType{MessageCreated, MessageTransmissionJoined, MessageMerged, AssignmentChanged} {
		fact := base(factType)
		envelope, err := fact.Envelope(now)
		if err != nil || envelope.Data.MessageTime == nil || !envelope.Data.MessageTime.Equal(now) ||
			envelope.Data.PreviousMessageTime != nil {
			t.Fatalf("%s envelope=%+v err=%v", factType, envelope.Data, err)
		}
		fact.MessageTime = time.Time{}
		if _, err := fact.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%s accepted without Message time: %v", factType, err)
		}
	}
	for _, factType := range []IntakeType{MessageTransmissionJoined, MessageMerged} {
		fact := base(factType)
		fact.PreviousMessageTime = &later
		envelope, err := fact.Envelope(now)
		if err != nil || envelope.Data.PreviousMessageTime == nil || !envelope.Data.PreviousMessageTime.Equal(later) {
			t.Fatalf("%s lost previous Message time: %+v err=%v", factType, envelope.Data, err)
		}
		for name, previous := range map[string]time.Time{"equal": now, "earlier": earlier} {
			fact.PreviousMessageTime = &previous
			if _, err := fact.Envelope(now); !errors.Is(err, ErrInvalidFact) {
				t.Fatalf("%s accepted %s previous Message time: %v", factType, name, err)
			}
		}
	}
	for _, factType := range []IntakeType{MessageCreated, AssignmentChanged} {
		fact := base(factType)
		fact.PreviousMessageTime = &later
		if _, err := fact.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%s accepted a previous Message time: %v", factType, err)
		}
	}
}

func TestIntakeMessageTimeIsEncodedAsRFC3339UTC(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	zone := time.FixedZone("test", 3600)
	fact := IntakeFact{ID: uuid.New(), Type: MessageCreated, MessageID: uuid.New(), TransmissionID: uuid.New(),
		Revision: 1, OccurredAt: now, MessageTime: now.In(zone)}
	envelope, err := fact.Envelope(now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(encoded, &data); err != nil || data["messageTime"] != "2026-11-08T10:00:00Z" {
		t.Fatalf("data=%s err=%v", encoded, err)
	}
}

func TestCorrectionMessageTimeRules(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	message, current, previous := uuid.New(), uuid.New(), uuid.New()
	position := 0
	assignment := CorrectionFact{ID: uuid.New(), Type: "assignment.changed",
		Aggregate: EntityRef{Type: "message", ID: message}, Revision: 2, OccurredAt: now,
		TransmissionID: uuid.New(), MessageID: message, NetworkID: current, PreviousNetworkID: &previous,
		Reason: "corrected", MessageTime: now}
	suppression := CorrectionFact{ID: uuid.New(), Type: "suppression.changed",
		Aggregate: EntityRef{Type: "suppression", ID: uuid.New()}, Revision: 1, OccurredAt: now,
		TargetPosition: &position, TransmissionID: uuid.New(), MessageID: message, NetworkID: current,
		ChangeKind: "created", MessageTime: now}
	for _, fact := range []CorrectionFact{assignment, suppression} {
		envelope, err := fact.Envelope(now)
		if err != nil || envelope.Data.MessageTime == nil || !envelope.Data.MessageTime.Equal(now) {
			t.Fatalf("%s envelope=%+v err=%v", fact.Type, envelope.Data, err)
		}
		fact.MessageTime = time.Time{}
		if _, err := fact.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatalf("%s accepted without Message time: %v", fact.Type, err)
		}
	}
	envelope, err := suppression.Envelope(now)
	if err != nil || envelope.Data.MessageID != message {
		t.Fatalf("suppression lost Message: %+v err=%v", envelope.Data, err)
	}
	suppression.MessageID = uuid.Nil
	if _, err := suppression.Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("suppression accepted without Message: %v", err)
	}
}

func TestProcessingCompletionRequiresMessageTime(t *testing.T) {
	t.Parallel()
	fact := completionFixture(t)
	envelope, err := fact.Envelope(time.Unix(101, 0))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		MessageTime time.Time `json:"messageTime"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil || !data.MessageTime.Equal(fact.MessageTime) {
		t.Fatalf("data=%s err=%v", envelope.Data, err)
	}
	fact.MessageTime = time.Time{}
	if _, err := fact.Envelope(time.Unix(101, 0)); err == nil {
		t.Fatal("completion accepted without Message time")
	}
}
