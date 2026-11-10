package readstore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

func TestFeedReturnsOneEligibleMessage(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit := w.unit(agency, "Engine 1", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	w.assigned(message, network, unit, agency, base, "")
	result := w.feed(all(), base.Add(time.Hour), readmodel.FeedQuery{})
	if !slices.Equal(ids(result), []uuid.UUID{message}) {
		t.Fatalf("items = %v", ids(result))
	}
	record := result.Items[0]
	if record.ProcessingStatus != "none" || record.CorrectionStatus != "current" || len(record.Units) != 1 ||
		record.Units[0].Name != "Engine 1" || record.Representative == nil || record.Representative.Protocol != "pocsag" {
		t.Fatalf("record = %+v", record)
	}
	if result.TxID < 0 || result.Seq < 0 || result.HasMore {
		t.Fatalf("result = %+v", result)
	}
}
