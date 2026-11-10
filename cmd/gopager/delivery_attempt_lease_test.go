package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// TestDeliverySupersededClaimCannotRecordTheOutcome proves fencing: when another worker took
// over the claim during the provider call, the late accepted answer is rejected, the Delivery is
// not recorded as accepted, and the next tick turns it into uncertain with lease-lost without
// invoking the provider again.
func TestDeliverySupersededClaimCannotRecordTheOutcome(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("fenced")
	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		s.exec(`UPDATE deliveries SET claim_token = claim_token + 1, claim_expires_at = $2 WHERE id = $1`,
			id, s.now().Add(-time.Second))
		return delivery.AttemptResult{Status: delivery.StatusAccepted}, nil
	})
	s.attempt()
	view := s.view(id)
	if view.State == delivery.Accepted || view.FinishedAt != nil {
		t.Fatalf("a superseded claim recorded its outcome: %+v", view)
	}
	if len(view.Attempts) != 1 || view.Attempts[0].State != "started" {
		t.Fatalf("the rejected write changed the attempt: %+v", view.Attempts)
	}

	s.attempt()
	view = s.expectState(id, delivery.Uncertain, delivery.CodeLeaseLost)
	if len(view.Attempts) != 1 || view.Attempts[0].State != "uncertain" || stringOf(view.Attempts[0].OutcomeCode) != delivery.CodeLeaseLost {
		t.Fatalf("attempt after lease loss: %+v", view.Attempts)
	}
	s.makeDue(id)
	s.attempt()
	if got := len(s.targets.calls()); got != 1 {
		t.Fatalf("a lease-lost Delivery was invoked again: %d calls", got)
	}
}

// TestDeliveryCrashedWorkerLeavesUncertainAndLateAnswerIsRejected proves recovery: while one
// worker is inside the provider call and its lease expires, another worker's tick records the
// started Delivery as uncertain (lease-lost), and the first worker's late answer is rejected and
// leaves it uncertain, with the provider invoked exactly once.
func TestDeliveryCrashedWorkerLeavesUncertainAndLateAnswerIsRejected(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("crash")
	entered, release := make(chan struct{}), make(chan struct{})
	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		close(entered)
		<-release
		return delivery.AttemptResult{Status: delivery.StatusAccepted}, nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := s.runner.Tick(t.Context())
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the provider was not invoked")
	}
	if state := s.view(id).State; state != delivery.Started {
		t.Fatalf("a Delivery in a provider call is %s", state)
	}
	s.exec(`UPDATE deliveries SET claim_expires_at = $2 WHERE id = $1`, id, s.now().Add(-time.Second))

	other, err := composeDeliveryRunner(s.core, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	s.expectState(id, delivery.Uncertain, delivery.CodeLeaseLost)

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	view := s.expectState(id, delivery.Uncertain, delivery.CodeLeaseLost)
	if len(view.Attempts) != 1 || view.Attempts[0].State != "uncertain" {
		t.Fatalf("late answer changed the attempt: %+v", view.Attempts)
	}
	if got := len(s.targets.calls()); got != 1 {
		t.Fatalf("expected one invocation, got %d", got)
	}
}

// TestDeliveryConcurrentWorkersInvokeEachDeliveryOnce proves that two runners ticking at the same
// time share the due Deliveries without sending any of them twice.
func TestDeliveryConcurrentWorkersInvokeEachDeliveryOnce(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{Subscriber: 1000, Target: 1000})
	const total = 30
	for i := range total {
		s.messageFact("message.created", s.newMessage(fmt.Sprintf("bulk %d", i)), nil)
	}
	s.fanOutAll()
	if got := s.count(`SELECT count(*) FROM deliveries WHERE state = 'prepared'`); got != total {
		t.Fatalf("prepared %d of %d", got, total)
	}
	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		time.Sleep(5 * time.Millisecond)
		return delivery.AttemptResult{Status: delivery.StatusAccepted}, nil
	})
	second, err := composeDeliveryRunner(s.core, nil)
	if err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	failures := make(chan error, 2)
	var group sync.WaitGroup
	for _, runner := range []*delivery.Runner{s.runner, second} {
		group.Add(1)
		go func() {
			defer group.Done()
			for !stop.Load() {
				if _, err := runner.Tick(t.Context()); err != nil {
					failures <- err
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}
	deadline := time.Now().Add(60 * time.Second)
	for s.count(`SELECT count(*) FROM deliveries WHERE state IN ('prepared', 'retry-pending', 'started')`) > 0 &&
		time.Now().Before(deadline) && len(failures) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	stop.Store(true)
	group.Wait()
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}

	perDelivery := map[uuid.UUID]int{}
	for _, call := range s.targets.calls() {
		perDelivery[call.DeliveryID]++
	}
	if len(perDelivery) != total {
		t.Fatalf("%d Deliveries invoked, want %d", len(perDelivery), total)
	}
	for id, n := range perDelivery {
		if n != 1 {
			t.Fatalf("Delivery %s invoked %d times", id, n)
		}
	}
	if got := s.count(`SELECT count(*) FROM deliveries WHERE state = 'accepted' AND attempts = 1`); got != total {
		t.Fatalf("%d accepted, want %d", got, total)
	}
	if got := s.count(`SELECT count(*) FROM delivery_attempts`); got != total {
		t.Fatalf("%d attempt rows, want %d", got, total)
	}
}

// TestDeliveryUnavailableTargetDefersWithoutConsumingAnAttempt proves that a target provider that
// is unavailable at start keeps the Delivery open with dependency-unavailable, no attempt and a
// 60 second delay, and that it is sent once when the provider returns.
func TestDeliveryUnavailableTargetDefersWithoutConsumingAnAttempt(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("unavailable")
	s.targets.setUnavailable(s.instanceID, true)
	before := s.now()
	s.attempt()
	after := s.now()
	view := s.expectState(id, delivery.Prepared, delivery.CodeDependency)
	if view.AttemptCount != 0 || len(view.Attempts) != 0 || len(s.targets.calls()) != 0 || view.NextAttemptAt == nil ||
		view.NextAttemptAt.Before(before.Add(delivery.UnavailableDelay)) || view.NextAttemptAt.After(after.Add(delivery.UnavailableDelay)) {
		t.Fatalf("deferred Delivery: %+v", view)
	}
	s.attempt()
	if len(s.targets.calls()) != 0 {
		t.Fatal("a deferred Delivery was invoked before it was due")
	}

	s.targets.setUnavailable(s.instanceID, false)
	s.makeDue(id)
	s.attempt()
	view = s.expectState(id, delivery.Accepted, delivery.CodeAccepted)
	if view.AttemptCount != 1 || len(s.targets.calls()) != 1 {
		t.Fatalf("after the provider returned: %+v, %d calls", view, len(s.targets.calls()))
	}
}

// TestDeliveryEveryTerminalStateRemovesTheInput proves that expiry before start ends the Delivery
// expired without an invocation, and that accepted, failed, uncertain, expired and cancelled
// Deliveries all have their input removed.
func TestDeliveryEveryTerminalStateRemovesTheInput(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	s.targets.setOutcome(func(invocation delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		switch {
		case bodyHas(invocation.Input.Body, "permanent"):
			return delivery.AttemptResult{Status: delivery.StatusPermanent}, nil
		case bodyHas(invocation.Input.Body, "uncertain"):
			return delivery.AttemptResult{Status: delivery.StatusUncertain}, nil
		}
		return delivery.AttemptResult{Status: delivery.StatusAccepted}, nil
	})
	accepted, failed := s.deliverOne("fine"), s.deliverOne("permanent")
	uncertain, expired := s.deliverOne("uncertain"), s.deliverOne("late")
	for _, id := range []uuid.UUID{accepted, failed, uncertain, expired} {
		if got := s.count(`SELECT count(*) FROM deliveries WHERE id = $1 AND input IS NOT NULL`, id); got != 1 {
			t.Fatalf("prepared Delivery %s has no input", id)
		}
	}
	s.exec(`UPDATE deliveries SET expires_at = $2 WHERE id = $1`, expired, s.now().Add(-time.Second))
	s.attempt()
	s.expectState(accepted, delivery.Accepted, delivery.CodeAccepted)
	s.expectState(failed, delivery.Failed, delivery.CodePermanent)
	s.expectState(uncertain, delivery.Uncertain, delivery.CodeUncertain)
	view := s.expectState(expired, delivery.Expired, delivery.CodeExpired)
	if view.AttemptCount != 0 || len(s.targets.calls()) != 3 {
		t.Fatalf("expired Delivery %+v, %d calls", view, len(s.targets.calls()))
	}

	cancelled := s.deliverOne("cancel")
	s.exec(`UPDATE subscriptions SET state = 'paused' WHERE id = $1`, s.subscription)
	s.attempt()
	s.expectState(cancelled, delivery.Cancelled, delivery.CodeSubscriptionPaused)
	if got := s.count(`SELECT count(*) FROM deliveries WHERE finished_at IS NOT NULL`); got != 5 {
		t.Fatalf("%d terminal Deliveries, want 5", got)
	}
	if got := s.count(`SELECT count(*) FROM deliveries WHERE finished_at IS NOT NULL AND input IS NOT NULL`); got != 0 {
		t.Fatalf("%d terminal Deliveries kept their input", got)
	}
}

// TestDeliverySendsStoredInputAfterTheMessageDataWasRemoved proves that a prepared Delivery is
// self-contained: when the Message it came from is deleted while the subscriber's access
// revision is unchanged, the stored body is still sent.
func TestDeliverySendsStoredInputAfterTheMessageDataWasRemoved(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id, message := s.deliverMessage("STORED BODY")
	s.deleteMessage(message)
	s.attempt()
	view := s.expectState(id, delivery.Accepted, delivery.CodeAccepted)
	calls := s.targets.calls()
	if view.AttemptCount != 1 || len(calls) != 1 || !bodyHas(calls[0].Input.Body, "STORED BODY") {
		t.Fatalf("stored input not sent: %+v, %+v", view, calls)
	}
}

// TestDeliveryMessageRemovalWithChangedAccessCancelsAsRevoked proves that when the Message data
// is gone and the subscriber's access revision changed too, the stored input is not trusted and
// the Delivery is cancelled with authority-revoked.
func TestDeliveryMessageRemovalWithChangedAccessCancelsAsRevoked(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id, message := s.deliverMessage("STALE BODY")
	s.deleteMessage(message)
	s.exec(`UPDATE identity_control SET access_revision = access_revision + 1`)
	s.attempt()
	view := s.expectState(id, delivery.Cancelled, delivery.CodeAuthorityRevoked)
	if view.AttemptCount != 0 || len(s.targets.calls()) != 0 {
		t.Fatalf("cancelled Delivery %+v, %d calls", view, len(s.targets.calls()))
	}
}

// deleteMessage removes a Message with its Units, Transmissions and their targets.
func (s *attemptScene) deleteMessage(message uuid.UUID) {
	s.t.Helper()
	s.exec(`DELETE FROM message_units WHERE message_id = $1`, message)
	s.exec(`DELETE FROM transmission_targets WHERE transmission_id IN (SELECT id FROM transmissions WHERE message_id = $1)`, message)
	s.exec(`DELETE FROM transmissions WHERE message_id = $1`, message)
	s.exec(`DELETE FROM messages WHERE id = $1`, message)
}

// contains reports whether text contains part.
func bodyHas(text, part string) bool { return strings.Contains(text, part) }
