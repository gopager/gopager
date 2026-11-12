package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
)

// attemptScene is a Delivery fixture with a subscriber, a Subscription to one controlled target
// instance, and a manager who may read, retry and redeliver every Delivery.
type attemptScene struct {
	*deliveryFixture
	subscriberBearer string
	subscriberID     uuid.UUID
	manager          string
	instanceID       uuid.UUID
	subscription     uuid.UUID
}

// newBareScene returns a scene without a Subscription. Scenes with admission limits wait until
// the current one-minute admission window has room, so a window change cannot fail a test.
func newBareScene(t *testing.T, limits delivery.AdmissionLimits) *attemptScene {
	t.Helper()
	f := newDeliveryFixture(t, limits)
	s := &attemptScene{deliveryFixture: f}
	s.subscriberBearer, s.subscriberID = f.subscriber("payload", "unit")
	s.manager, _ = f.caller([]string{"delivery.read", "delivery.retry", "delivery.redeliver"}, authz.Scope{Kind: "all"},
		[]string{"payload"})
	s.instanceID = f.instance(true)
	return s
}

// newAttemptScene returns a scene whose subscriber has an active message.created Subscription.
func newAttemptScene(t *testing.T, limits delivery.AdmissionLimits) *attemptScene {
	t.Helper()
	s := newBareScene(t, limits)
	s.subscription = s.subscribeTo(s.subscriberBearer)
	return s
}

// subscribeTo creates a message.created Subscription of the bearer's subscriber to the target.
func (s *attemptScene) subscribeTo(bearer string) uuid.UUID {
	return s.subscribe(bearer, map[string]any{"eventTypes": []string{"message.created"},
		"targets": []any{target(s.instanceID)}})
}

// scan reads one row of a statement into dest.
func (s *attemptScene) scan(dest []any, statement string, args ...any) {
	s.t.Helper()
	err := s.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := s.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), statement, args...).Scan(dest...)
	})
	if err != nil {
		s.t.Fatalf("%s: %v", statement, err)
	}
}

// count returns the first column of a counting statement.
func (s *attemptScene) count(statement string, args ...any) int {
	s.t.Helper()
	var n int
	s.scan([]any{&n}, statement, args...)
	return n
}

// newMessage creates a Message of one new Unit and returns its ID.
func (s *attemptScene) newMessage(text string) uuid.UUID {
	unit := s.unit("Engine "+text, nil)
	return s.message(s.now().Add(-time.Minute), text, unit)
}

// fanOutAll runs the fan-out consumer until its offset covers every recorded fact.
func (s *attemptScene) fanOutAll() {
	s.t.Helper()
	s.fanOut()
}

// deliverMessage records a message.created fact for a new Message, runs fan-out and returns the
// ID of the single prepared Delivery and the Message.
func (s *attemptScene) deliverMessage(text string) (uuid.UUID, uuid.UUID) {
	s.t.Helper()
	message := s.newMessage(text)
	s.messageFact("message.created", message, nil)
	s.fanOutAll()
	var id uuid.UUID
	s.scan([]any{&id}, `SELECT id FROM deliveries WHERE message_id = $1 AND subscriber_id = $2`, message, s.subscriberID)
	return id, message
}

// deliverOne returns the ID of a prepared Delivery of a new Message.
func (s *attemptScene) deliverOne(text string) uuid.UUID {
	s.t.Helper()
	id, _ := s.deliverMessage(text)
	return id
}

// view reads a Delivery with its attempts over HTTP as the manager.
func (s *attemptScene) view(id uuid.UUID) delivery.Delivery {
	s.t.Helper()
	response := managementRequest(s.t, s.client, s.origin, "GET", "/api/v1/deliveries/"+id.String(), "", "", s.manager, 200)
	var value delivery.Delivery
	if err := json.Unmarshal(response.Data, &value); err != nil {
		s.t.Fatal(err)
	}
	return value
}

// makeDue moves the next attempt of an open Delivery to the past.
func (s *attemptScene) makeDue(id uuid.UUID) {
	s.t.Helper()
	s.exec(`UPDATE deliveries SET next_attempt_at = $2 WHERE id = $1`, id, s.now().Add(-time.Minute))
}

// post sends an authenticated JSON POST and returns the status, headers and decoded body.
func (s *attemptScene) post(bearer, path, body string) (int, http.Header, map[string]any) {
	s.t.Helper()
	request, err := http.NewRequestWithContext(s.t.Context(), http.MethodPost, s.origin+path, strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", s.origin)
	request.Header.Set("Authorization", bearer)
	response, err := s.client.Do(request)
	if err != nil {
		s.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		s.t.Fatal(err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		s.t.Fatalf("%s %s: %s", path, body, raw)
	}
	return response.StatusCode, response.Header, decoded
}

// retry requests a manual retry with the expected revision and returns status and error code.
func (s *attemptScene) retry(bearer string, id uuid.UUID, revision int64) (int, string) {
	s.t.Helper()
	status, _, body := s.post(bearer, "/api/v1/deliveries/"+id.String()+"/retry",
		fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":"%d"}`, revision))
	code, _ := body["code"].(string)
	return status, code
}

// redeliver requests a redelivery and returns status, headers and the decoded body.
func (s *attemptScene) redeliver(bearer string, id, command uuid.UUID, confirm bool) (int, http.Header, map[string]any) {
	s.t.Helper()
	return s.post(bearer, "/api/v1/deliveries/"+id.String()+"/redeliver",
		fmt.Sprintf(`{"schemaVersion":1,"commandId":%q,"confirmDuplicateRisk":%t}`, command, confirm))
}

// revoke deletes the principal's grants and bumps the access revision, as removing a role does.
func (s *attemptScene) revoke(principal uuid.UUID) {
	s.t.Helper()
	s.exec(`DELETE FROM identity_grants WHERE subject_id = $1`, principal)
	s.exec(`UPDATE identity_control SET access_revision = access_revision + 1`)
}

// stringOf dereferences an optional string.
func stringOf(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

// useStatus makes the controlled provider answer every invocation with the status.
func (s *attemptScene) useStatus(status delivery.AttemptStatus) {
	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		return delivery.AttemptResult{Status: status, Code: "test"}, nil
	})
}

// expectState asserts the state and outcome code of a Delivery read over HTTP.
func (s *attemptScene) expectState(id uuid.UUID, state, code string) delivery.Delivery {
	s.t.Helper()
	view := s.view(id)
	if view.State != state || stringOf(view.OutcomeCode) != code {
		s.t.Fatalf("delivery is %s/%s, want %s/%s: %+v", view.State, stringOf(view.OutcomeCode), state, code, view)
	}
	return view
}

// TestDeliveryTransientFailuresBackOffUntilRetriesAreExhausted proves that a provider answering
// transient makes the Delivery wait with the documented jittered backoff after each attempt and
// end failed with retry-exhausted after exactly five invocations, each with its own attempt row.
func TestDeliveryTransientFailuresBackOffUntilRetriesAreExhausted(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	s.useStatus(delivery.StatusTransient)
	id := s.deliverOne("backoff")
	nominal := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}

	for attempt := 1; attempt <= 4; attempt++ {
		before := s.now()
		s.attempt()
		after := s.now()
		view := s.expectState(id, delivery.RetryPending, delivery.CodeTransient)
		if view.AttemptCount != attempt || len(view.Attempts) != attempt || view.NextAttemptAt == nil {
			t.Fatalf("after attempt %d: %+v", attempt, view)
		}
		low, high := nominal[attempt-1]*8/10, nominal[attempt-1]*12/10
		if view.NextAttemptAt.Before(before.Add(low)) || view.NextAttemptAt.After(after.Add(high)) {
			t.Fatalf("attempt %d: next attempt %s outside %s..%s after the failure", attempt, view.NextAttemptAt, low, high)
		}
		// Not yet due: another tick must not invoke the provider.
		s.attempt()
		if got := len(s.targets.calls()); got != attempt {
			t.Fatalf("a Delivery that is not due was invoked: %d calls after attempt %d", got, attempt)
		}
		s.makeDue(id)
	}
	s.attempt()
	view := s.expectState(id, delivery.Failed, delivery.CodeRetryExhausted)
	if view.AttemptCount != 5 || len(view.Attempts) != 5 || view.NextAttemptAt != nil || len(s.targets.calls()) != 5 {
		t.Fatalf("after exhaustion: %+v, %d calls", view, len(s.targets.calls()))
	}
	for index, attempt := range view.Attempts {
		if attempt.Number != index+1 || attempt.State != "transient" || attempt.FinishedAt == nil {
			t.Fatalf("attempt row %d: %+v", index, attempt)
		}
	}
	s.exec(`UPDATE deliveries SET next_attempt_at = $2 WHERE id = $1`, id, s.now().Add(-time.Minute))
	s.attempt()
	if got := len(s.targets.calls()); got != 5 {
		t.Fatalf("an exhausted Delivery was invoked again: %d calls", got)
	}
}

// TestDeliveryPermanentFailureEndsWithoutRetry proves that a permanent provider answer ends the
// Delivery failed after one attempt, with no retry even when it is made due again.
func TestDeliveryPermanentFailureEndsWithoutRetry(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	s.useStatus(delivery.StatusPermanent)
	id := s.deliverOne("permanent")
	s.attempt()
	view := s.expectState(id, delivery.Failed, delivery.CodePermanent)
	if view.AttemptCount != 1 || len(view.Attempts) != 1 || view.Attempts[0].State != "permanent" {
		t.Fatalf("after permanent failure: %+v", view)
	}
	s.makeDue(id)
	s.attempt()
	if got := len(s.targets.calls()); got != 1 {
		t.Fatalf("a permanently failed Delivery was invoked again: %d calls", got)
	}
}

// TestDeliveryProviderRetryAfterWinsOnlyWhenLongerThanBackoff proves that a provider retryAfter
// beyond the backoff schedules the next attempt at least that late, and a shorter one never
// brings the next attempt before the backoff minimum.
func TestDeliveryProviderRetryAfterWinsOnlyWhenLongerThanBackoff(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	retryAfter := 10 * time.Minute
	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		return delivery.AttemptResult{Status: delivery.StatusTransient, RetryAfter: retryAfter}, nil
	})
	long := s.deliverOne("long")
	before := s.now()
	s.attempt()
	after := s.now()
	view := s.expectState(long, delivery.RetryPending, delivery.CodeTransient)
	if view.NextAttemptAt.Before(before.Add(retryAfter)) || view.NextAttemptAt.After(after.Add(retryAfter)) {
		t.Fatalf("next attempt %s does not follow the provider retryAfter %s (%s..%s)", view.NextAttemptAt, retryAfter, before, after)
	}

	retryAfter = time.Second
	short := s.deliverOne("short")
	before = s.now()
	s.attempt()
	view = s.expectState(short, delivery.RetryPending, delivery.CodeTransient)
	if view.NextAttemptAt.Before(before.Add(24 * time.Second)) {
		t.Fatalf("a short retryAfter brought the next attempt to %s, before the backoff minimum", view.NextAttemptAt)
	}
}

// exhaust makes the provider answer transient and runs the Delivery to retry-exhausted.
func (s *attemptScene) exhaust(id uuid.UUID) {
	s.t.Helper()
	s.useStatus(delivery.StatusTransient)
	for range 5 {
		s.makeDue(id)
		s.attempt()
	}
	s.expectState(id, delivery.Failed, delivery.CodeRetryExhausted)
}

// TestDeliveryManualRetryRestartsOnlyKnownTransientFailures proves that a retry with the current
// revision makes a retry-exhausted Delivery due now and the next tick attempts it again, and that
// a stale revision, an accepted Delivery, an uncertain Delivery and missing rights are refused.
func TestDeliveryManualRetryRestartsOnlyKnownTransientFailures(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	readOnly, _ := s.caller([]string{"delivery.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	outsider, _ := s.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	exhausted := s.deliverOne("exhausted")
	s.exhaust(exhausted)
	revision := s.view(exhausted).Revision

	if status, code := s.retry(s.manager, exhausted, revision-1); status != 409 || code != "REVISION_CONFLICT" {
		t.Fatalf("stale revision: %d %s", status, code)
	}
	if status, code := s.retry(readOnly, exhausted, revision); status != 403 || code != "FORBIDDEN" {
		t.Fatalf("reader without delivery.retry: %d %s", status, code)
	}
	if status, code := s.retry(outsider, exhausted, revision); status != 404 || code != "NOT_FOUND" {
		t.Fatalf("caller without any delivery right: %d %s", status, code)
	}
	if view := s.expectState(exhausted, delivery.Failed, delivery.CodeRetryExhausted); view.Revision != revision {
		t.Fatalf("a refused retry changed the Delivery: %+v", view)
	}

	s.useStatus(delivery.StatusAccepted)
	before := s.now()
	if status, code := s.retry(s.manager, exhausted, revision); status != 200 {
		t.Fatalf("retry: %d %s", status, code)
	}
	view := s.view(exhausted)
	if view.State != delivery.RetryPending || view.NextAttemptAt == nil || view.NextAttemptAt.After(s.now()) ||
		view.NextAttemptAt.Before(before.Add(-time.Second)) || view.AttemptCount != 5 {
		t.Fatalf("after retry: %+v", view)
	}
	calls := len(s.targets.calls())
	s.attempt()
	view = s.expectState(exhausted, delivery.Accepted, delivery.CodeAccepted)
	if view.AttemptCount != 6 || len(view.Attempts) != 6 || len(s.targets.calls()) != calls+1 {
		t.Fatalf("after the retried attempt: %+v", view)
	}

	if status, code := s.retry(s.manager, exhausted, view.Revision); status != 409 || code != "REVISION_CONFLICT" {
		t.Fatalf("retry of an accepted Delivery: %d %s", status, code)
	}

	s.targets.setOutcome(func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		return delivery.AttemptResult{}, fmt.Errorf("connection reset")
	})
	uncertain := s.deliverOne("uncertain")
	s.attempt()
	view = s.expectState(uncertain, delivery.Uncertain, delivery.CodeUncertain)
	if status, code := s.retry(s.manager, uncertain, view.Revision); status != 409 || code != "EXTERNAL_UNCERTAIN" {
		t.Fatalf("retry of an uncertain Delivery: %d %s", status, code)
	}
}

// TestDeliveryUncertainOutcomeIsNeverRetriedAutomatically proves that a provider error after the
// call started, and a provider answer of uncertain, both end the Delivery and its attempt as
// uncertain and that no later tick invokes the provider again, even when the Delivery is made due.
func TestDeliveryUncertainOutcomeIsNeverRetriedAutomatically(t *testing.T) {
	t.Parallel()
	cases := map[string]func(delivery.AttemptInvocation) (delivery.AttemptResult, error){
		"lost response": func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
			return delivery.AttemptResult{Receipt: json.RawMessage(`{"id":"lost"}`)}, fmt.Errorf("connection reset by peer")
		},
		"provider status uncertain": func(delivery.AttemptInvocation) (delivery.AttemptResult, error) {
			return delivery.AttemptResult{Status: delivery.StatusUncertain, Code: "timeout"}, nil
		},
	}
	for name, outcome := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newAttemptScene(t, delivery.AdmissionLimits{})
			s.targets.setOutcome(outcome)
			id := s.deliverOne("uncertain")
			s.attempt()
			view := s.expectState(id, delivery.Uncertain, delivery.CodeUncertain)
			if view.AttemptCount != 1 || len(view.Attempts) != 1 || view.Attempts[0].State != "uncertain" ||
				view.Attempts[0].FinishedAt == nil || view.FinishedAt == nil {
				t.Fatalf("uncertain Delivery: %+v", view)
			}
			if len(view.Attempts[0].Receipt) != 0 && name == "lost response" {
				t.Fatalf("a receipt of a failed call was kept: %s", view.Attempts[0].Receipt)
			}
			for range 3 {
				s.makeDue(id)
				s.attempt()
			}
			if got := len(s.targets.calls()); got != 1 {
				t.Fatalf("an uncertain Delivery was invoked again: %d calls", got)
			}
			s.expectState(id, delivery.Uncertain, delivery.CodeUncertain)
		})
	}
}
