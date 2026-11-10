package main

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// TestSubscriptionCreateReplayNeedsCurrentVisibility proves that replaying a Subscription create
// command returns the stored Subscription only while the caller may still see it.
func TestSubscriptionCreateReplayNeedsCurrentVisibility(t *testing.T) {
	t.Parallel()
	s := newBareScene(t, delivery.AdmissionLimits{})
	body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "kind": "personal",
		"state": "active", "eventTypes": []string{"message.created"}, "targets": []any{target(s.instanceID)}})
	managementRequest(t, s.client, s.origin, "POST", "/api/v1/subscriptions", string(body), "", s.subscriberBearer, 201)
	replayed := managementRequest(t, s.client, s.origin, "POST", "/api/v1/subscriptions", string(body), "", s.subscriberBearer, 201)
	if len(replayed.Data) == 0 {
		t.Fatal("replay returned no Subscription")
	}
	s.revoke(s.subscriberID)
	managementRequest(t, s.client, s.origin, "POST", "/api/v1/subscriptions", string(body), "", s.subscriberBearer, 404)
}

// TestRedeliveryReplayNeedsCurrentReadRight proves that replaying a redelivery command returns the
// new Delivery only while the caller may still read it.
func TestRedeliveryReplayNeedsCurrentReadRight(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	original := s.uncertainDelivery("replay")
	command := uuid.New()
	if status, _, body := s.redeliver(s.manager, original, command, true); status != 201 {
		t.Fatalf("redelivery: %d %v", status, body)
	}
	if status, _, body := s.redeliver(s.manager, original, command, true); status != 201 {
		t.Fatalf("replay: %d %v", status, body)
	}
	var manager uuid.UUID
	s.scan([]any{&manager}, `SELECT c.account_id FROM identity_credentials c JOIN identity_grants g ON g.subject_id = c.account_id
		WHERE g.actions ? 'delivery.redeliver' AND NOT g.actions ? 'subscription.manage-own'`)
	s.revoke(manager)
	if status, _, body := s.redeliver(s.manager, original, command, true); status != 404 {
		t.Fatalf("replay without rights: %d %v", status, body)
	}
}
