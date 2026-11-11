package delivery

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
)

// TestParseEventReadsRealProducerEnvelopes proves that the identities Delivery needs are read from
// the envelopes the producers actually write: the Gateway of a gateway.state-changed fact from its
// aggregate reference, the Message and Transmission of a Message fact from its data.
func TestParseEventReadsRealProducerEnvelopes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	gateway, message, transmission := uuid.New(), uuid.New(), uuid.New()
	for _, test := range []struct {
		fact         events.IntakeFact
		gateway      uuid.UUID
		message      uuid.UUID
		transmission uuid.UUID
	}{
		{fact: events.IntakeFact{ID: uuid.New(), Type: events.GatewayStateChanged, GatewayID: gateway, Revision: 2,
			OccurredAt: now, OldState: "active", NewState: "contact-lost", ContactTime: &now}, gateway: gateway},
		{fact: events.IntakeFact{ID: uuid.New(), Type: events.MessageCreated, MessageID: message, TransmissionID: transmission,
			Revision: 1, OccurredAt: now, MessageTime: now}, message: message, transmission: transmission},
	} {
		envelope, err := test.fact.Envelope(now)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		event, err := parseEvent(test.fact.ID, encoded)
		if err != nil {
			t.Fatal(err)
		}
		if event.Type != string(test.fact.Type) || event.Data.GatewayID != test.gateway ||
			event.Data.MessageID != test.message || event.Data.TransmissionID != test.transmission {
			t.Fatalf("%s parsed as %+v", test.fact.Type, event)
		}
	}
}
