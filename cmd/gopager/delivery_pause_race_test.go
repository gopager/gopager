package main

import (
	"context"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
)

// TestDeliveryStartWaitsForAnInFlightPause proves that a pause committing while a Delivery's start
// is being checked cannot be overtaken: the start waits for the pause, sees the paused
// Subscription and cancels the Delivery without invoking the provider.
func TestDeliveryStartWaitsForAnInFlightPause(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("paused during start")
	locked, commit, paused := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		paused <- s.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
			executor, err := s.store.Resolve(tx)
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `UPDATE subscriptions SET state = 'paused',
				revision = revision + 1 WHERE id = $1`, s.subscription); err != nil {
				return err
			}
			close(locked)
			<-commit
			return nil
		})
	}()
	<-locked
	ticked := make(chan error, 1)
	go func() {
		_, err := s.runner.Tick(context.Background())
		ticked <- err
	}()
	select {
	case err := <-ticked:
		t.Fatalf("the start did not wait for the in-flight pause: %v, %d invocations", err, len(s.targets.calls()))
	case <-time.After(300 * time.Millisecond):
	}
	close(commit)
	if err := <-paused; err != nil {
		t.Fatal(err)
	}
	if err := <-ticked; err != nil {
		t.Fatal(err)
	}
	s.expectState(id, delivery.Cancelled, delivery.CodeSubscriptionPaused)
	if calls := len(s.targets.calls()); calls != 0 {
		t.Fatalf("a paused Subscription's Delivery was invoked %d times", calls)
	}
}
