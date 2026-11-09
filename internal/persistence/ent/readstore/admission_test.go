package readstore_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

func TestTakeSearchAttemptCountsOnlyBelowTheLimitAndPrunesOldWindows(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	bucket := [32]byte{1}
	other := [32]byte{2}
	window := base.Truncate(time.Minute)
	take := func(bucket [32]byte, start time.Time, limit int) bool {
		t.Helper()
		var admitted bool
		w.within(func(ctx context.Context, tx platform.TransactionContext) {
			var err error
			if admitted, err = w.reads.TakeSearchAttempt(ctx, tx, bucket, start, limit); err != nil {
				t.Fatal(err)
			}
		})
		return admitted
	}
	count := func(bucket [32]byte) int {
		var rows int
		w.within(func(ctx context.Context, tx platform.TransactionContext) {
			executor, err := w.store.Resolve(tx)
			if err != nil {
				t.Fatal(err)
			}
			if err := executor.QueryRowContext(ctx, `SELECT coalesce(sum(count), 0) FROM search_admissions WHERE bucket = $1`, bucket[:]).Scan(&rows); err != nil {
				t.Fatal(err)
			}
		})
		return rows
	}
	for i := 0; i < 2; i++ {
		if !take(bucket, window, 2) {
			t.Fatalf("attempt %d was rejected", i+1)
		}
	}
	for i := 0; i < 3; i++ {
		if take(bucket, window, 2) {
			t.Fatal("an attempt above the limit was admitted")
		}
	}
	if got := count(bucket); got != 2 {
		t.Fatalf("count = %d, rejected attempts must not be counted", got)
	}
	if !take(other, window, 2) {
		t.Fatal("another bucket shares the count")
	}
	if !take(bucket, window.Add(time.Minute), 2) {
		t.Fatal("the next window starts empty")
	}
	if got := count(bucket); got != 1 {
		t.Fatalf("older windows of the bucket were kept: sum = %d", got)
	}
	if got := count(other); got != 1 {
		t.Fatalf("another bucket was pruned: %d", got)
	}
}

func TestTakeSearchAttemptIsAtomicUnderConcurrency(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	bucket := [32]byte{9}
	window := base.Truncate(time.Minute)
	const limit, requests = 5, 20
	var mu sync.Mutex
	admitted := 0
	var group sync.WaitGroup
	for i := 0; i < requests; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
				ok, err := w.reads.TakeSearchAttempt(tx.Context(), tx, bucket, window, limit)
				if ok {
					mu.Lock()
					admitted++
					mu.Unlock()
				}
				return err
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if admitted != limit {
		t.Fatalf("admitted = %d, want %d", admitted, limit)
	}
}

func TestSavedViewStoreRoundTrip(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	owner := uuid.New()
	record := readmodel.SavedViewRecord{ID: uuid.New(), Revision: 1, Ownership: "personal", OwnerPrincipalID: &owner,
		Name: "View", Definition: []byte(`{"schemaVersion":1,"filters":{}}`), CreatedAt: base, UpdatedAt: base}
	global := readmodel.SavedViewRecord{ID: uuid.New(), Revision: 1, Ownership: "global", Name: "Global",
		Definition: []byte(`{"schemaVersion":1,"filters":{}}`), CreatedAt: base, UpdatedAt: base}
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		for _, stored := range []readmodel.SavedViewRecord{record, global} {
			if err := w.reads.InsertSavedView(ctx, tx, stored); err != nil {
				t.Fatal(err)
			}
		}
		got, err := w.reads.SavedView(ctx, tx, record.ID)
		if err != nil || got.OwnerPrincipalID == nil || *got.OwnerPrincipalID != owner || got.Revision != 1 {
			t.Fatalf("read = %+v, %v", got, err)
		}
		if _, err := w.reads.SavedView(ctx, tx, uuid.New()); err != authz.ErrNotFound {
			t.Fatalf("absent view: %v", err)
		}
		records, total, err := w.reads.ListSavedViews(ctx, tx, readmodel.SavedViewListQuery{Owner: owner, Global: true, Limit: 10})
		if err != nil || total != 2 || len(records) != 2 || records[0].Name != "Global" {
			t.Fatalf("list = %v, %d, %v", records, total, err)
		}
		records, total, err = w.reads.ListSavedViews(ctx, tx, readmodel.SavedViewListQuery{Owner: uuid.New(), Limit: 10})
		if err != nil || total != 0 || len(records) != 0 {
			t.Fatalf("foreign owner list = %v, %d, %v", records, total, err)
		}
		if err := w.reads.UpdateSavedView(ctx, tx, record.ID, 2, "X", record.Definition, base); err != authz.ErrRevisionConflict {
			t.Fatalf("stale update: %v", err)
		}
		if err := w.reads.UpdateSavedView(ctx, tx, record.ID, 1, "Renamed", record.Definition, base.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := w.reads.DeleteSavedView(ctx, tx, record.ID, 1); err != authz.ErrRevisionConflict {
			t.Fatalf("stale delete: %v", err)
		}
		if err := w.reads.DeleteSavedView(ctx, tx, record.ID, 2); err != nil {
			t.Fatal(err)
		}
	})
}
