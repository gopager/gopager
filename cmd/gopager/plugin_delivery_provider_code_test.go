package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// deliveryView reads a Delivery with its attempts over HTTP as the subscriber.
func (s *pluginDeliveryScene) deliveryView(bearer string, id uuid.UUID) delivery.Delivery {
	s.t.Helper()
	response := managementRequest(s.t, s.client, s.origin, http.MethodGet, "/api/v1/deliveries/"+id.String(), "", "", bearer, http.StatusOK)
	var value delivery.Delivery
	if err := json.Unmarshal(response.Data, &value); err != nil {
		s.t.Fatal(err)
	}
	return value
}

// requireProviderCode checks the enumerated outcome and the provider's code on a Delivery and its
// only attempt.
func requireProviderCode(t *testing.T, view delivery.Delivery, outcome, providerCode string) {
	t.Helper()
	if len(view.Attempts) != 1 {
		t.Fatalf("attempts = %+v", view.Attempts)
	}
	attempt := view.Attempts[0]
	if stringOf(view.OutcomeCode) != outcome || stringOf(view.ProviderCode) != providerCode ||
		stringOf(attempt.ProviderCode) != providerCode {
		t.Fatalf("outcome %q, provider code %q, attempt provider code %q; want %q and %q",
			stringOf(view.OutcomeCode), stringOf(view.ProviderCode), stringOf(attempt.ProviderCode), outcome, providerCode)
	}
}

// TestPushoverWithoutPersonalUserKeyShowsRecipientMissingOnTheDelivery proves that the plugin's safe
// reason code reaches the Delivery and its attempt over HTTP next to the enumerated outcome.
func TestPushoverWithoutPersonalUserKeyShowsRecipientMissingOnTheDelivery(t *testing.T) {
	t.Parallel()
	s := newPluginDeliveryScene(t)
	art := provisionFirstParty(t, s.directory, "pushover")
	receiver := newRecordingReceiver(t)
	instance := s.createInstance(art, map[string]any{"endpoint": typedString(receiver.URL),
		"allowPrivateDestinations": typedBoolean(true)}, map[string]any{"apiToken": plainSecret("app-token-1")})
	bearer, _ := s.subscriber("payload", "unit")
	s.subscribeTarget(bearer, instance, nil)

	ids := s.deliverMessage("alarm")
	s.attempt()
	requireProviderCode(t, s.deliveryView(bearer, ids[0]), delivery.CodePermanent, "recipient-missing")
	if got := len(receiver.requests()); got != 0 {
		t.Fatalf("receiver got %d requests, want none", got)
	}
}
