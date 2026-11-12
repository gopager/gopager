package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

func TestMessageFieldReaderReadsTheMessageWithoutEventFields(t *testing.T) {
	unit := readmodel.UnitRecord{UnitID: uuid.New(), Name: "Engine 1"}
	text := `{"form":"text","text":"FIRE"}`
	projection := readmodel.DeliveryProjection{
		Item:   readmodel.FeedItem{MessageID: uuid.New(), MessageTime: time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC), Payload: []byte(text)},
		Units:  []readmodel.UnitRecord{unit},
		Groups: []string{"payload", "unit"},
	}
	reader := MessageFieldReader(projection, incidentDeclarations())
	ctx := context.Background()
	for _, tc := range []struct {
		ref  processing.FieldRef
		want processing.FieldState
	}{
		{*field("message", "text"), processing.FieldPresent},
		{*field("message", "unitNames"), processing.FieldPresent},
		{*field("event", "type"), processing.FieldAbsent},
		{*field("event", "occurredAt"), processing.FieldAbsent},
	} {
		got, err := reader.LookupField(ctx, tc.ref)
		if err != nil {
			t.Fatalf("LookupField(%v): %v", tc.ref, err)
		}
		if got.State != tc.want {
			t.Fatalf("LookupField(%v) state = %v, want %v", tc.ref, got.State, tc.want)
		}
	}
}
