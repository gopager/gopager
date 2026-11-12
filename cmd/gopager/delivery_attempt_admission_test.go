package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/processing"
)

// limitedDescriptor returns a target descriptor like the default one that lowers the per-minute
// sends of the instance.
func limitedDescriptor(perMinute int) func(processing.InstanceConfiguration) delivery.TargetDescriptor {
	return func(instance processing.InstanceConfiguration) delivery.TargetDescriptor {
		return delivery.TargetDescriptor{Version: delivery.DescriptorVersion, InstanceID: instance.ID,
			InstanceRevision: instance.Revision, CapabilityKind: delivery.CapabilityDelivery, ConfigurationAuthority: "admin",
			OutputKinds:    []string{processing.LayoutText, processing.LayoutMarkdown, processing.LayoutJSON},
			Inputs:         []delivery.InputField{{Name: "recipient", Kind: processing.StringValue, Required: true, MaxBytes: 256}},
			SendsPerMinute: perMinute}
	}
}

// TestDeliverySubscriptionCreateConsumesTheSubscribersSendingUnit proves that creating a
// Subscription takes one unit of the subscriber's sending admission and that the next create in
// the same minute answers 429 RATE_LIMITED with a Retry-After header and retryAfterSeconds.
func TestDeliverySubscriptionCreateConsumesTheSubscribersSendingUnit(t *testing.T) {
	t.Parallel()
	withinOneAdmissionWindow(t, newAdmissionClockStore(t), func(int) error {
		s := newBareScene(t, delivery.AdmissionLimits{Subscriber: 1})
		s.subscribeTo(s.subscriberBearer)
		body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "kind": "personal", "state": "active",
			"eventTypes": []string{"message.created"}, "targets": []any{target(s.instanceID)}})
		status, header, decoded := s.post(s.subscriberBearer, "/api/v1/subscriptions", string(body))
		seconds, _ := decoded["retryAfterSeconds"].(float64)
		if status != 429 || decoded["code"] != "RATE_LIMITED" || seconds < 1 || seconds > 60 ||
			header.Get("Retry-After") != strconv.Itoa(int(seconds)) {
			return fmt.Errorf("second create: %d %v retry-after=%q", status, decoded, header.Get("Retry-After"))
		}
		if got := s.count(`SELECT count(*) FROM subscriptions`); got != 1 {
			return fmt.Errorf("a rate-limited create stored a Subscription: %d", got)
		}
		return nil
	})
}

// TestDeliverySubscriberLimitDefersBackgroundSendsAndRefusesRedelivery proves that when the
// subscriber's sending units of the minute are used up, the next Delivery waits as retry-pending
// with rate-limited, no attempt consumed and the next minute boundary as due time, and that an
// explicit redelivery answers 429 with Retry-After.
func TestDeliverySubscriberLimitDefersBackgroundSendsAndRefusesRedelivery(t *testing.T) {
	t.Parallel()
	withinOneAdmissionWindow(t, newAdmissionClockStore(t), func(int) error {
		s := newAttemptScene(t, delivery.AdmissionLimits{Subscriber: 2})
		first, second := s.deliverOne("one"), s.deliverOne("two")
		s.attempt()
		boundary := s.now().Truncate(time.Minute).Add(time.Minute)

		states := map[string]uuid.UUID{}
		for _, id := range []uuid.UUID{first, second} {
			states[s.view(id).State] = id
		}
		sent, deferred := states[delivery.Accepted], states[delivery.RetryPending]
		if sent == uuid.Nil || deferred == uuid.Nil || len(s.targets.calls()) != 1 {
			return fmt.Errorf("expected one accepted and one deferred Delivery: %v, %d calls", states, len(s.targets.calls()))
		}
		view := s.view(deferred)
		if view.State != delivery.RetryPending || stringOf(view.OutcomeCode) != delivery.CodeRateLimited {
			return fmt.Errorf("deferred Delivery %s: state %s, outcome %q", deferred, view.State, stringOf(view.OutcomeCode))
		}
		if view.AttemptCount != 0 || len(view.Attempts) != 0 || view.NextAttemptAt == nil || !view.NextAttemptAt.Equal(boundary) {
			return fmt.Errorf("deferred Delivery: %+v, want next attempt %s", view, boundary)
		}

		status, header, decoded := s.redeliver(s.manager, sent, uuid.New(), true)
		if status != 429 || decoded["code"] != "RATE_LIMITED" || header.Get("Retry-After") == "" || decoded["retryAfterSeconds"] == nil {
			return fmt.Errorf("redelivery over the limit: %d %v retry-after=%q", status, decoded, header.Get("Retry-After"))
		}
		if got := s.count(`SELECT count(*) FROM deliveries`); got != 2 {
			return fmt.Errorf("a rate-limited redelivery created a Delivery: %d", got)
		}
		return nil
	})
}

// TestDeliveryTargetLimitCapsSendsAcrossSubscribers proves that a descriptor limit of one send per
// minute lets only one Delivery reach the instance in that minute, even for different
// subscribers, and defers the other without consuming an attempt.
func TestDeliveryTargetLimitCapsSendsAcrossSubscribers(t *testing.T) {
	t.Parallel()
	withinOneAdmissionWindow(t, newAdmissionClockStore(t), func(int) error {
		s := newBareScene(t, delivery.AdmissionLimits{Subscriber: 100, Target: 100})
		s.targets.descriptor = limitedDescriptor(1)
		other, _ := s.subscriber("payload", "unit")
		s.subscription = s.subscribeTo(s.subscriberBearer)
		s.subscribeTo(other)
		s.messageFact("message.created", s.newMessage("shared"), nil)
		s.fanOutAll()
		if got := s.count(`SELECT count(*) FROM deliveries WHERE state = 'prepared'`); got != 2 {
			t.Fatalf("expected two prepared Deliveries, got %d", got)
		}
		s.attempt()
		if got := s.count(`SELECT count(*) FROM deliveries WHERE state = 'accepted'`); got != 1 || len(s.targets.calls()) != 1 {
			return fmt.Errorf("expected one send, got %d accepted, %d calls", got, len(s.targets.calls()))
		}
		if got := s.count(`SELECT count(*) FROM deliveries WHERE state = 'retry-pending' AND outcome_code = 'rate-limited'
			AND attempts = 0`); got != 1 {
			return fmt.Errorf("expected one rate-limited Delivery without attempt, got %d", got)
		}
		return nil
	})
}

// TestDeliveryInvocationCarriesVersionIdentityDeadlineAndTrace proves the provider contract: the
// invocation has version 1, the stable Delivery ID, a fresh attempt ID and increasing number per
// attempt, the instance revision and a deadline 30 seconds after the start; the causation is the
// fact's own ID unless it records one; the correlation comes from the fact; and the attempt rows
// match the invocations.
func TestDeliveryInvocationCarriesVersionIdentityDeadlineAndTrace(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	plainFact := s.messageFact("message.created", s.newMessage("plain"), nil)
	causation, correlation := uuid.New(), uuid.New()
	tracedFact := s.envelopeFact("message.created", s.newMessage("traced"), map[string]any{
		"causationId": causation, "correlationId": correlation})
	s.fanOutAll()
	var plain, traced uuid.UUID
	s.scan([]any{&plain}, `SELECT id FROM deliveries WHERE domain_event_id = $1`, plainFact)
	s.scan([]any{&traced}, `SELECT id FROM deliveries WHERE domain_event_id = $1`, tracedFact)

	s.useStatus(delivery.StatusTransient)
	var starts []time.Time
	for range 3 {
		starts = append(starts, s.now())
		s.attempt()
		s.makeDue(plain)
		s.makeDue(traced)
	}
	finished := s.now()
	calls := map[uuid.UUID][]delivery.AttemptInvocation{}
	for _, call := range s.targets.calls() {
		calls[call.DeliveryID] = append(calls[call.DeliveryID], call)
	}
	for id, wantCausation := range map[uuid.UUID]uuid.UUID{plain: plainFact, traced: causation} {
		view := s.view(id)
		invocations := calls[id]
		if len(invocations) != 3 || len(view.Attempts) != 3 {
			t.Fatalf("delivery %s: %d invocations, %d attempt rows", id, len(invocations), len(view.Attempts))
		}
		seen := map[uuid.UUID]bool{}
		for index, call := range invocations {
			attempt := view.Attempts[index]
			if call.Version != delivery.InvocationVersion || call.AttemptNumber != index+1 || attempt.Number != index+1 ||
				call.InstanceID != s.instanceID || call.InstanceRevision != 1 || attempt.InstanceRevision != 1 ||
				call.AttemptID != attempt.ID || seen[call.AttemptID] || call.CausationID != wantCausation {
				t.Fatalf("delivery %s invocation %d: %+v attempt %+v", id, index, call, attempt)
			}
			seen[call.AttemptID] = true
			if call.Deadline.Before(starts[index].Add(delivery.AttemptDeadline)) ||
				call.Deadline.After(finished.Add(delivery.AttemptDeadline)) {
				t.Fatalf("deadline %s is not 30 s after the start (%s..%s)", call.Deadline, starts[index], finished)
			}
		}
	}
	if calls[plain][0].CorrelationID != nil {
		t.Fatalf("a fact without correlation produced %v", calls[plain][0].CorrelationID)
	}
	if got := calls[traced][0].CorrelationID; got == nil || *got != correlation {
		t.Fatalf("correlation %v, want %s", got, correlation)
	}
}

// envelopeFact records a fact for the Message whose envelope carries extra top-level fields, such
// as causation and correlation, and returns its ID.
func (s *attemptScene) envelopeFact(eventType string, message uuid.UUID, extra map[string]any) uuid.UUID {
	s.t.Helper()
	id, recorded := uuid.New(), s.now()
	entry, ok := events.LookupType(eventType)
	if !ok {
		s.t.Fatalf("unknown type %s", eventType)
	}
	envelope := map[string]any{"schemaVersion": 1, "domainEventId": id, "type": eventType,
		"producer":     map[string]string{"domain": entry.Producer, "version": "test1.0"},
		"aggregateRef": map[string]any{"type": "message", "id": uuid.New()}, "aggregateRevision": "1",
		"occurredAt": recorded.Format(time.RFC3339Nano), "recordedAt": recorded.Format(time.RFC3339Nano),
		"data":           map[string]any{"messageId": message, "messageTime": recorded.Format(time.RFC3339Nano)},
		"classification": "operational", "replayMode": "normal"}
	for key, value := range extra {
		envelope[key] = value
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		s.t.Fatal(err)
	}
	s.exec(`INSERT INTO domain_events (id, envelope) VALUES ($1, $2::jsonb)`, id, string(encoded))
	return id
}

// TestDeliveryReceiptIsShownWhenSmallAndValidAndDroppedOtherwise proves that a provider receipt is
// stored with its attempt and visible in the Delivery, while a receipt over 4 KiB or one that is
// not valid JSON is dropped without failing the attempt.
func TestDeliveryReceiptIsShownWhenSmallAndValidAndDroppedOtherwise(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	big := json.RawMessage(`{"blob":"` + stringOfLength(5000) + `"}`)
	receipts := []struct {
		raw  json.RawMessage
		keep bool
	}{{json.RawMessage(`{"id":"r-1"}`), true}, {big, false}, {json.RawMessage(`{not json`), false}}
	s.targets.setOutcome(func(invocation delivery.AttemptInvocation) (delivery.AttemptResult, error) {
		if invocation.AttemptNumber <= len(receipts) {
			return delivery.AttemptResult{Status: delivery.StatusTransient, Receipt: receipts[invocation.AttemptNumber-1].raw}, nil
		}
		return delivery.AttemptResult{Status: delivery.StatusAccepted, Receipt: json.RawMessage(`{"id":"r-4"}`)}, nil
	})
	id := s.deliverOne("receipts")
	for range 4 {
		s.attempt()
		s.makeDue(id)
	}
	view := s.expectState(id, delivery.Accepted, delivery.CodeAccepted)
	if len(view.Attempts) != 4 {
		t.Fatalf("attempts: %+v", view.Attempts)
	}
	for index, want := range receipts {
		got := view.Attempts[index].Receipt
		if want.keep != (len(got) > 0) {
			t.Fatalf("attempt %d receipt kept=%t, want %t (%d bytes)", index+1, len(got) > 0, want.keep, len(got))
		}
	}
	var last struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(view.Attempts[3].Receipt, &last); err != nil || last.ID != "r-4" {
		t.Fatalf("accepted attempt receipt %s: %v", view.Attempts[3].Receipt, err)
	}
	var first struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(view.Attempts[0].Receipt, &first); err != nil || first.ID != "r-1" {
		t.Fatalf("first attempt receipt %s: %v", view.Attempts[0].Receipt, err)
	}
}

// stringOfLength returns a string of n letters.
func stringOfLength(n int) string {
	buffer := make([]byte, n)
	for i := range buffer {
		buffer[i] = 'x'
	}
	return string(buffer)
}
