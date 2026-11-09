package readstore_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// TestLiveContextsAreCurrent checks that a Unit's Agency and collections and a Network's Agency
// are read as they are now, and that the transaction bound covers positions of earlier pages.
func TestLiveContextsAreCurrent(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency, other := w.agency("county"), w.agency("city")
	unit := w.unit(agency, "A", nil)
	bare := w.unit(other, "B", nil)
	network := w.network(other, "N", "pocsag")
	collection := uuid.New()
	w.exec(`INSERT INTO unit_collections (id, name) VALUES ($1, 'C')`, collection)
	w.exec(`INSERT INTO unit_collection_memberships (unit_id, collection_id) VALUES ($1, $2)`, unit, collection)
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		contexts, err := w.reads.UnitContexts(ctx, tx, []uuid.UUID{unit, bare, uuid.New()})
		if err != nil {
			t.Fatal(err)
		}
		if len(contexts) != 2 || contexts[unit].AgencyID != agency || !slices.Equal(contexts[unit].Collections, []uuid.UUID{collection}) ||
			contexts[bare].AgencyID != other || len(contexts[bare].Collections) != 0 {
			t.Errorf("unit contexts = %+v", contexts)
		}
		agencies, err := w.reads.NetworkAgencies(ctx, tx, []uuid.UUID{network, uuid.New()})
		if err != nil || len(agencies) != 1 || agencies[network] != other {
			t.Errorf("network agencies = %v, %v", agencies, err)
		}
		head, err := w.reads.CurrentTxID(ctx, tx)
		if err != nil || head <= 0 {
			t.Errorf("current transaction ID = %d, %v", head, err)
		}
	})
}

// TestTransmissionMessagesFollowMerges checks that a Transmission resolves to the Message that
// holds it now, which after merges is the final survivor.
func TestTransmissionMessagesFollowMerges(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	network := w.network(w.agency("county"), "N", "pocsag")
	at := time.Now().UTC().Truncate(time.Microsecond)
	first, final := w.message(at, "text", "x"), w.message(at, "text", "x")
	transmission := w.transmission(transmissionSpec{network: network, message: first, accepted: at})
	w.exec(`UPDATE transmissions SET message_id = $1 WHERE message_id = $2`, final, first)
	w.exec(`DELETE FROM messages WHERE id = $1`, first)
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		messages, err := w.reads.TransmissionMessages(ctx, tx, []uuid.UUID{transmission, uuid.New()})
		if err != nil || len(messages) != 1 || messages[transmission] != final {
			t.Errorf("transmission messages = %v, %v", messages, err)
		}
	})
}
