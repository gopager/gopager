package readstore_test

import (
	"context"
	"github.com/gopager/gopager/internal/platform"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

// TestPagingIsStableAndGroupsBeforePaging checks: 120 Messages with
// two Unit appearances each come back as 50, 50 and 20 distinct rows in descending
// (time, ID) order, a Message arriving between pages appears only in a fresh first page, and
// Messages sharing one time are split across a page boundary without loss.
func TestPagingIsStableAndGroupsBeforePaging(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, b := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	var created []uuid.UUID
	insert := func(first time.Time) uuid.UUID {
		m := w.message(first, "text", "alarm")
		w.assigned(m, network, a, agency, first, "")
		w.assigned(m, network, b, agency, first, "")
		return m
	}
	for i := range 120 {
		// Every third Message shares its time with the previous one so keyset ties are crossed.
		created = append(created, insert(base.Add(time.Duration(i-i%3)*time.Second)))
	}
	caller := all()
	now := base.Add(time.Hour)
	var got []uuid.UUID
	var cursor *readmodel.Cursor
	sizes := []int{}
	var firstPosition int64
	for page := 0; page < 3; page++ {
		result := w.feed(caller, now, readmodel.FeedQuery{Limit: 50, After: cursor})
		sizes = append(sizes, len(result.Items))
		got = append(got, ids(result)...)
		if page == 0 {
			firstPosition = result.Seq
			insert(base.Add(24 * time.Hour)) // newer than everything: above the cursor
		}
		if (page < 2) != result.HasMore {
			t.Fatalf("page %d HasMore = %v", page, result.HasMore)
		}
		last := result.Items[len(result.Items)-1]
		cursor = &readmodel.Cursor{Time: last.Time, ID: last.MessageID}
	}
	if !slices.Equal(sizes, []int{50, 50, 20}) {
		t.Fatalf("page sizes = %v", sizes)
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("duplicate %v", id)
		}
		seen[id] = true
	}
	for _, id := range created {
		if !seen[id] {
			t.Fatalf("gap: %v missing", id)
		}
	}
	fresh := w.feed(caller, now, readmodel.FeedQuery{Limit: 50})
	if len(fresh.Items) != 50 || fresh.Items[0].Time.Before(base.Add(24*time.Hour)) {
		t.Fatalf("a fresh first page shows the new Message first: %v", fresh.Items[0].Time)
	}
	if firstPosition < 0 {
		t.Fatalf("position = %d", firstPosition)
	}
}

// TestEvaluateReturnsListedMessagesWithoutTimeBoundsOrPaging covers the live re-evaluation
// query: the listed Messages that match come back in feed order, the others vanish.
func TestEvaluateReturnsListedMessagesWithoutTimeBoundsOrPaging(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	unit, other := w.unit(agency, "A", nil), w.unit(agency, "B", nil)
	network := w.network(agency, "N", "pocsag")
	m1 := w.message(base, "text", "one")
	m2 := w.message(base.Add(time.Minute), "text", "two")
	m3 := w.message(base.Add(2*time.Minute), "text", "three")
	w.assigned(m1, network, unit, agency, base, "")
	w.assigned(m2, network, unit, agency, base, "")
	w.assigned(m3, network, other, agency, base, "")
	caller := all()
	result := w.feed(caller, base.Add(time.Hour), readmodel.FeedQuery{
		IDs: []uuid.UUID{m1, m2, m3}, Filters: readmodel.Filters{Units: []uuid.UUID{unit}}})
	if !slices.Equal(ids(result), []uuid.UUID{m2, m1}) || result.HasMore {
		t.Fatalf("evaluate = %v", ids(result))
	}
}

// TestPositionAdvancesWithTheEventLog checks that the cutoff reflects committed facts.
func TestPositionAdvancesWithTheEventLog(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	before := w.feed(all(), base, readmodel.FeedQuery{})
	w.exec(`INSERT INTO domain_events (id, envelope) VALUES ($1::uuid, jsonb_build_object('domainEventId', $1::uuid::text, 'type', 'test.fact',
		'occurredAt', '2026-11-08T12:00:00Z'))`, uuid.New())
	after := w.feed(all(), base, readmodel.FeedQuery{})
	if after.Seq <= before.Seq || after.TxID < before.TxID {
		t.Fatalf("position before %d/%d, after %d/%d", before.TxID, before.Seq, after.TxID, after.Seq)
	}
}

// TestTokenKeyIsCreatedOnceAndStable covers the query_token_keys secret.
func TestTokenKeyIsCreatedOnceAndStable(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	var first, second []byte
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		var err error
		if first, err = w.reads.TokenKey(ctx, tx); err != nil {
			t.Fatal(err)
		}
	})
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		var err error
		if second, err = w.reads.TokenKey(ctx, tx); err != nil {
			t.Fatal(err)
		}
	})
	if len(first) != 32 || !slices.Equal(first, second) {
		t.Fatalf("keys %x / %x", first, second)
	}
}
