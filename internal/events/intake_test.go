package events

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntakeFactEnvelopeUsesTypedSafeVocabularies(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	message, transmission := uuid.New(), uuid.New()
	envelope, err := (IntakeFact{ID: uuid.New(), Type: MessageMerged, MessageID: message,
		TransmissionID: transmission, Revision: 2, OccurredAt: now,
		Absorbed: []uuid.UUID{uuid.New(), uuid.New()}}).Envelope(now)
	if err != nil || envelope.AggregateRef.ID != message ||
		envelope.Type != MessageMerged || len(envelope.Data.Absorbed) != 2 {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
	if _, err := (IntakeFact{ID: uuid.New(), Type: MessageMerged, MessageID: message,
		TransmissionID: transmission, Revision: 2, OccurredAt: now,
		Absorbed: []uuid.UUID{message}}).Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("accepted self-merge: %v", err)
	}
}

func TestIntakeFactRejectsIncompleteGatewayTransitionAndAssignment(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	gateway := uuid.New()
	if _, err := (IntakeFact{ID: uuid.New(), Type: GatewayStateChanged, GatewayID: gateway,
		Revision: 1, OccurredAt: now}).Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("accepted incomplete Gateway transition: %v", err)
	}
	position := 0
	if _, err := (IntakeFact{ID: uuid.New(), Type: AssignmentChanged, MessageID: uuid.New(),
		TransmissionID: uuid.New(), TargetPosition: &position, Revision: 1, OccurredAt: now,
		After: &IntakeResolution{Status: "assigned"}}).Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("accepted assignment without Unit/rule: %v", err)
	}
}
