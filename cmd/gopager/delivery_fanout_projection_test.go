package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

// countingProjector counts the projections requested per subscriber authority.
type countingProjector struct {
	delivery.Projector
	mu    sync.Mutex
	calls map[string]int
}

func (c *countingProjector) ProjectForDelivery(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, now time.Time, message, result uuid.UUID) (readmodel.DeliveryProjection, bool, error) {
	c.mu.Lock()
	c.calls[authority.PrincipalID]++
	c.mu.Unlock()
	return c.Projector.ProjectForDelivery(ctx, tx, authority, now, message, result)
}

// TestFanOutProjectsOncePerSubscriberAndEvent proves that the Subscriptions of one subscriber
// share one projection of the Message per event, that every subscriber with a different
// authority gets their own, and that the next event projects again.
func TestFanOutProjectsOncePerSubscriberAndEvent(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	instance := f.instance(true)
	runtime, err := composeDelivery(f.core)
	if err != nil {
		t.Fatal(err)
	}
	projector := &countingProjector{Projector: runtime.deps.Projector, calls: map[string]int{}}
	deps := runtime.deps
	deps.Projector = projector
	fanOut, err := delivery.NewFanOut(deps, f.core.store)
	if err != nil {
		t.Fatal(err)
	}
	principals := map[uuid.UUID]bool{}
	for range 2 {
		bearer, principal := f.subscriber(fanOutGroups...)
		principals[principal] = true
		f.subCreated(bearer, subInput(instance, nil))
		f.subCreated(bearer, subInput(instance, map[string]any{"splitByUnit": true}))
	}
	message := f.message(f.minuteAgo(), "SHARED", f.unit("Engine 1", nil), f.unit("Engine 2", nil))

	for event := 1; event <= 2; event++ {
		record := f.eventRecord(message)
		f.withTx(func(tx platform.TransactionContext) error { return fanOut.Handle(tx.Context(), tx, record) })
		if len(projector.calls) != len(principals) {
			t.Fatalf("event %d projected for %d authorities, want %d", event, len(projector.calls), len(principals))
		}
		for principal, calls := range projector.calls {
			if calls != event {
				t.Fatalf("event %d: %d projections for %s, want %d (one per event)", event, calls, principal, event)
			}
		}
	}
	// Two unsplit and four split Deliveries per event prove that both Subscriptions of a subscriber
	// still materialized from the shared projection.
	if got := f.subStrings(`SELECT count(*)::text FROM deliveries`); got[0] != "12" {
		t.Fatalf("Deliveries = %s, want 12", got[0])
	}
}
