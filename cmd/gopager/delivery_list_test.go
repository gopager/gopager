package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// TestDeliveryListFiltersByStateAndSubscription proves that the Delivery list accepts the state and
// subscriptionId filters together with paging, and still rejects unknown and repeated parameters.
func TestDeliveryListFiltersByStateAndSubscription(t *testing.T) {
	t.Parallel()
	s := newAttemptScene(t, delivery.AdmissionLimits{})
	other := s.subscribeTo(s.subscriberBearer)
	accepted := s.deliverOne("accepted")
	s.attempt()
	s.useStatus(delivery.StatusPermanent)
	failed := s.deliverOne("failed")
	s.attempt()

	byState := s.deliveries(s.manager, "?state=failed&limit=10")
	for _, item := range byState {
		if item.State != delivery.Failed {
			t.Fatalf("state filter returned %+v", item)
		}
	}
	if !containsDelivery(byState, failed) || containsDelivery(byState, accepted) {
		t.Fatalf("state filter: %+v", byState)
	}
	bySubscription := s.deliveries(s.manager, "?subscriptionId="+other.String()+"&offset=0")
	for _, item := range bySubscription {
		if item.SubscriptionID != other {
			t.Fatalf("subscription filter returned %+v", item)
		}
	}
	if len(bySubscription) != 2 {
		t.Fatalf("subscription filter: %d Deliveries, want one per fact", len(bySubscription))
	}
	for _, query := range []string{"?unknown=1", "?state=failed&state=accepted", "?subscriptionId=not-a-uuid", "?state=bogus"} {
		managementRequest(t, s.client, s.origin, "GET", "/api/v1/deliveries"+query, "", "", s.manager, 400)
	}
}

// containsDelivery reports whether the list holds the Delivery.
func containsDelivery(items []delivery.Delivery, id uuid.UUID) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
