package main

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// lostResponse makes the controlled provider fail after the call, as a lost response does.
func (s *attemptScene) lostResponse() {
	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		return delivery.AttemptResult{}, errors.New("connection reset by peer")
	})
}

// uncertainDelivery returns a Delivery that ended uncertain after a lost response.
func (s *attemptScene) uncertainDelivery(text string) uuid.UUID {
	s.t.Helper()
	s.lostResponse()
	id := s.deliverOne(text)
	s.attempt()
	s.expectState(id, delivery.Uncertain, delivery.CodeUncertain)
	return id
}

// idOf reads the ID of a Delivery from a decoded response body's data object.
func idOf(t *testing.T, body map[string]any) uuid.UUID {
	t.Helper()
	data, _ := body["data"].(map[string]any)
	text, _ := data["id"].(string)
	id, err := uuid.Parse(text)
	if err != nil {
		t.Fatalf("response without Delivery ID: %v", body)
	}
	return id
}

// TestDeliveryRedeliveryRequiresConfirmationAndCreatesTracedDelivery proves that redelivery
// without the duplicate-risk confirmation is refused, that a confirmed one creates a new
// Delivery tracing the original and sends it once, that the original keeps its uncertain
// outcome, and that replaying the same command returns the same new Delivery.
func TestDeliveryRedeliveryRequiresConfirmationAndCreatesTracedDelivery(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	original := s.uncertainDelivery("redeliver")
	command := uuid.New()

	if status, _, body := s.redeliver(s.manager, original, command, false); status != 400 {
		t.Fatalf("redelivery without confirmation: %d %v", status, body)
	}
	if got := s.count(`SELECT count(*) FROM deliveries`); got != 1 {
		t.Fatalf("a refused redelivery created a Delivery: %d", got)
	}

	status, _, body := s.redeliver(s.manager, original, command, true)
	if status != 201 {
		t.Fatalf("confirmed redelivery: %d %v", status, body)
	}
	fresh := idOf(t, body)
	view := s.view(fresh)
	if fresh == original || view.Origin != delivery.OriginRedelivery || view.RedeliveryOf == nil ||
		*view.RedeliveryOf != original || view.State != delivery.Prepared || view.AttemptCount != 0 {
		t.Fatalf("redelivered Delivery: %+v", view)
	}

	s.useStatus(delivery.StatusAccepted)
	s.attempt()
	s.expectState(fresh, delivery.Accepted, delivery.CodeAccepted)
	s.expectState(original, delivery.Uncertain, delivery.CodeUncertain)
	if got := len(s.targets.calls()); got != 2 {
		t.Fatalf("the original once and the redelivery once, got %d calls", got)
	}

	status, _, body = s.redeliver(s.manager, original, command, true)
	if status != 201 || idOf(t, body) != fresh {
		t.Fatalf("replay of the command: %d %v", status, body)
	}
	if got := s.count(`SELECT count(*) FROM deliveries`); got != 2 {
		t.Fatalf("a replayed command created another Delivery: %d", got)
	}
	s.attempt()
	if got := len(s.targets.calls()); got != 2 {
		t.Fatalf("a replayed command sent again: %d calls", got)
	}
}

// TestDeliveryRedeliveryIsRefusedAfterTheOriginalInputCeiling proves that a Delivery created more
// than 24 hours ago cannot be redelivered and answers a dependency-unavailable conflict.
func TestDeliveryRedeliveryIsRefusedAfterTheOriginalInputCeiling(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	original := s.uncertainDelivery("old")
	s.exec(`UPDATE deliveries SET created_at = created_at - interval '25 hours' WHERE id = $1`, original)
	status, _, body := s.redeliver(s.manager, original, uuid.New(), true)
	if parameters, _ := body["parameters"].(map[string]any); status != 409 || body["code"] != "DEPENDENCY_UNAVAILABLE" ||
		parameters["dependency"] != "input-ceiling" {
		t.Fatalf("redelivery after the ceiling: %d %v", status, body)
	}
	if got := s.count(`SELECT count(*) FROM deliveries`); got != 1 {
		t.Fatalf("a refused redelivery created a Delivery: %d", got)
	}
}

// TestDeliveryRedeliveryIsRefusedWhenTheSubscriberCannotReadTheMessage proves that redelivery
// materializes under the subscriber's current authority: once the subscriber lost message.read,
// it answers a dependency-unavailable conflict and creates nothing.
func TestDeliveryRedeliveryIsRefusedWhenTheSubscriberCannotReadTheMessage(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	original := s.uncertainDelivery("hidden")
	s.revoke(s.subscriberID)
	status, _, body := s.redeliver(s.manager, original, uuid.New(), true)
	if parameters, _ := body["parameters"].(map[string]any); status != 409 || body["code"] != "DEPENDENCY_UNAVAILABLE" ||
		parameters["dependency"] != "data" {
		t.Fatalf("redelivery without data access: %d %v", status, body)
	}
	if got := s.count(`SELECT count(*) FROM deliveries`); got != 1 {
		t.Fatalf("a refused redelivery created a Delivery: %d", got)
	}
}

// TestDeliveryRevocationBeforeStartCancelsWithoutInvocation proves that a subscriber whose
// message.read grant was removed before the attempt starts has its prepared Delivery cancelled
// with authority-revoked, its input removed and the provider never called.
func TestDeliveryRevocationBeforeStartCancelsWithoutInvocation(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("revoked")
	s.revoke(s.subscriberID)
	s.attempt()
	view := s.expectState(id, delivery.Cancelled, delivery.CodeAuthorityRevoked)
	if view.AttemptCount != 0 || len(view.Attempts) != 0 || len(s.targets.calls()) != 0 {
		t.Fatalf("revoked Delivery: %+v, %d calls", view, len(s.targets.calls()))
	}
	if got := s.count(`SELECT count(*) FROM deliveries WHERE id = $1 AND input IS NULL`, id); got != 1 {
		t.Fatal("the input of a cancelled Delivery was kept")
	}
}

// TestDeliveryRevocationAfterStartRecordsTheStartedEffectHonestly proves that a revocation
// committed while the provider call runs does not turn the accepted answer into a cancellation
// (the effect cannot be recalled), and that a later fact creates no Delivery for the revoked
// subscriber.
func TestDeliveryRevocationAfterStartRecordsTheStartedEffectHonestly(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	id := s.deliverOne("started")
	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		s.revoke(s.subscriberID)
		return delivery.AttemptResult{Status: delivery.StatusAccepted}, nil
	})
	s.attempt()
	view := s.expectState(id, delivery.Accepted, delivery.CodeAccepted)
	if view.AttemptCount != 1 || len(view.Attempts) != 1 || view.Attempts[0].State != "accepted" {
		t.Fatalf("started effect: %+v", view)
	}

	s.messageFact("message.created", s.newMessage("after"), map[string]any{"messageTime": s.now().Add(-time.Second).Format(time.RFC3339Nano)})
	s.fanOutAll()
	s.attempt()
	if got := s.count(`SELECT count(*) FROM deliveries`); got != 1 || len(s.targets.calls()) != 1 {
		t.Fatalf("a revoked subscriber got another Delivery: %d rows, %d calls", got, len(s.targets.calls()))
	}
}
