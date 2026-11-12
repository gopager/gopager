package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// placedRunner returns a Delivery runner of the scene that asks placement which instances it can serve.
func (s *attemptScene) placedRunner(placement func(context.Context) ([]uuid.UUID, error)) *delivery.Runner {
	s.t.Helper()
	runtime, err := composeDelivery(s.core)
	if err != nil {
		s.t.Fatal(err)
	}
	runner, err := delivery.NewRunner(runtime.deps, s.store, s.store,
		delivery.RunnerOptions{Placement: placement, Logger: slogDiscard()})
	if err != nil {
		s.t.Fatal(err)
	}
	return runner
}

// TestDeliveryInvocationCarriesTheSubscriber proves that the provider learns the Delivery's subscriber.
func TestDeliveryInvocationCarriesTheSubscriber(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	s.deliverOne("who")
	s.attempt()
	calls := s.targets.calls()
	if len(calls) != 1 || calls[0].SubscriberID != s.subscriberID {
		t.Fatalf("invocations: %+v, want subscriber %s", calls, s.subscriberID)
	}
}

// TestDeliveryPlacementLimitsClaimsUntilOverdueThenDefers proves that a node whose placement lists
// no instance leaves a fresh Delivery alone, and that once the Delivery is overdue the node claims it
// and, because its provider cannot describe the instance, defers it with
// dependency-unavailable, keeping its state and consuming no attempt.
func TestDeliveryPlacementLimitsClaimsUntilOverdueThenDefers(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("placed")
	var listed []uuid.UUID
	runner := s.placedRunner(func(context.Context) ([]uuid.UUID, error) { return listed, nil })

	if _, err := runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.expectState(id, delivery.Prepared, "<nil>")

	s.targets.setUnavailable(s.instanceID, true)
	s.exec(`UPDATE deliveries SET next_attempt_at = $2 WHERE id = $1`, id,
		s.now().Add(-delivery.PlacementGrace-time.Second))
	if _, err := runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	view := s.expectState(id, delivery.Prepared, delivery.CodeDependency)
	if view.AttemptCount != 0 || len(view.Attempts) != 0 || len(s.targets.calls()) != 0 {
		t.Fatalf("deferred Delivery consumed an attempt: %+v", view)
	}

	s.targets.setUnavailable(s.instanceID, false)
	s.makeDue(id)
	listed = []uuid.UUID{s.instanceID}
	if _, err := runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.expectState(id, delivery.Accepted, delivery.Accepted)
}

// TestDeliveryPlacementErrorFailsTheTick proves that a placement failure surfaces from Tick and
// claims nothing.
func TestDeliveryPlacementErrorFailsTheTick(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("failing placement")
	boom := errors.New("placement unavailable")
	runner := s.placedRunner(func(context.Context) ([]uuid.UUID, error) { return nil, boom })
	if _, err := runner.Tick(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("tick error: %v", err)
	}
	s.expectState(id, delivery.Prepared, "<nil>")
}
