package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// renewal is the shortened claim renewal interval of these tests.
const renewal = 100 * time.Millisecond

// fastRunner returns a Delivery runner of the scene whose claim renewal runs every renewal.
func (s *attemptScene) fastRunner() *delivery.Runner {
	s.t.Helper()
	runtime, err := composeDelivery(s.core)
	if err != nil {
		s.t.Fatal(err)
	}
	runner, err := delivery.NewRunner(runtime.deps, s.store, s.store,
		delivery.RunnerOptions{RenewInterval: renewal, Logger: slogDiscard()})
	if err != nil {
		s.t.Fatal(err)
	}
	return runner
}

// leaseOf reads a Delivery's claim expiry.
func (s *attemptScene) leaseOf(id uuid.UUID) time.Time {
	s.t.Helper()
	var until time.Time
	s.scan([]any{&until}, `SELECT claim_expires_at FROM deliveries WHERE id = $1`, id)
	return until
}

// TestDeliveryRunnerRejectsRenewalIntervalsThatDoNotFitTheLease proves that at least three
// renewals must fit into the claim lease and that a negative interval is rejected.
func TestDeliveryRunnerRejectsRenewalIntervalsThatDoNotFitTheLease(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	runtime, err := composeDelivery(s.core)
	if err != nil {
		t.Fatal(err)
	}
	for _, interval := range []time.Duration{-time.Second, delivery.LeaseDuration/3 + time.Millisecond} {
		if _, err := delivery.NewRunner(runtime.deps, s.store, s.store, delivery.RunnerOptions{RenewInterval: interval}); err == nil {
			t.Fatalf("renewal interval %s accepted", interval)
		}
	}
	if _, err := delivery.NewRunner(runtime.deps, s.store, s.store, delivery.RunnerOptions{RenewInterval: delivery.LeaseDuration / 3}); err != nil {
		t.Fatalf("three renewals per lease rejected: %v", err)
	}
}

// TestDeliveryClaimIsRenewedDuringALongProviderCall proves that the runner keeps extending the
// claim while a provider call is still running, so a call longer than the renewal interval
// keeps its lease and its outcome is recorded.
func TestDeliveryClaimIsRenewedDuringALongProviderCall(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("long call")
	var before, after time.Time
	s.targets.setWait(func(ctx context.Context, _ delivery.AttemptInvocation) error {
		before = s.leaseOf(id)
		select {
		case <-time.After(6 * renewal):
		case <-ctx.Done():
			return ctx.Err()
		}
		after = s.leaseOf(id)
		return nil
	})
	if _, err := s.fastRunner().Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !after.After(before.Add(2 * renewal)) {
		t.Fatalf("lease not renewed during the call: before %s, after %s", before, after)
	}
	s.expectState(id, delivery.Accepted, delivery.CodeAccepted)
	if calls := len(s.targets.calls()); calls != 1 {
		t.Fatalf("%d invocations", calls)
	}
}

// TestDeliveryRevocationDuringAProviderCallCancelsItAsUncertain proves that the runner checks the
// subscriber's authority at each renewal and cancels a running call once it is revoked; the
// cancelled call may already have had its effect, so the Delivery ends uncertain and is never
// retried automatically.
func TestDeliveryRevocationDuringAProviderCallCancelsItAsUncertain(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("revoked during call")
	var cancelled bool
	var waited time.Duration
	s.targets.setWait(func(ctx context.Context, _ delivery.AttemptInvocation) error {
		s.revoke(s.subscriberID)
		started := time.Now()
		select {
		case <-ctx.Done():
			cancelled, waited = true, time.Since(started)
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return errors.New("call not cancelled")
		}
	})
	runner := s.fastRunner()
	if _, err := runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !cancelled || waited > 20*renewal {
		t.Fatalf("call cancelled=%v after %s", cancelled, waited)
	}
	view := s.expectState(id, delivery.Uncertain, delivery.CodeUncertain)
	if len(view.Attempts) != 1 || view.Attempts[0].State != delivery.Uncertain {
		t.Fatalf("attempts: %+v", view.Attempts)
	}
	s.makeDue(id)
	if _, err := runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls := len(s.targets.calls()); calls != 1 {
		t.Fatalf("an uncertain Delivery was invoked again: %d invocations", calls)
	}
}

// TestDeliveryLostClaimDuringAProviderCallCancelsIt proves that a claim taken over by another
// worker during a call is noticed at the next renewal: the call is cancelled, its outcome is not
// recorded under the lost claim, and the Delivery ends uncertain with lease-lost once the lease
// has expired.
func TestDeliveryLostClaimDuringAProviderCallCancelsIt(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("claim lost during call")
	var cancelled bool
	s.targets.setWait(func(ctx context.Context, _ delivery.AttemptInvocation) error {
		s.exec(`UPDATE deliveries SET claim_token = claim_token + 1 WHERE id = $1`, id)
		select {
		case <-ctx.Done():
			cancelled = true
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return errors.New("call not cancelled")
		}
	})
	runner := s.fastRunner()
	if _, err := runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("call kept running after the claim was lost")
	}
	if view := s.view(id); view.State != delivery.Started {
		t.Fatalf("an outcome was recorded under the lost claim: %+v", view)
	}
	s.exec(`UPDATE deliveries SET claim_expires_at = now() - interval '1 second' WHERE id = $1`, id)
	if _, err := runner.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.expectState(id, delivery.Uncertain, delivery.CodeLeaseLost)
	if calls := len(s.targets.calls()); calls != 1 {
		t.Fatalf("%d invocations", calls)
	}
}
