package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// setPersonalConfiguration writes a subscriber's first personal configuration of an instance over
// HTTP: a device and a Secret user key.
func (s *pluginDeliveryScene) setPersonalConfiguration(bearer string, instance uuid.UUID, device, userKey string) {
	s.t.Helper()
	body := mustJSON(s.t, map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "expectedRevision": "0",
		"configuration": typedObject(map[string]any{"device": typedString(device)}),
		"secrets":       map[string]any{"userKey": plainSecret(userKey)}})
	managementRequest(s.t, s.client, s.origin, http.MethodPut,
		"/api/v1/instances/"+instance.String()+"/personal-configuration", body, "", bearer, http.StatusOK)
}

// TestPushoverEachSubscriberReachesOnlyTheirOwnUserKey proves the whole path with the shipped
// Pushover plugin: the administrator stores the application token, two subscribers store their own
// user key through the personal configuration, and every Delivery carries exactly its subscriber's
// key and device.
func TestPushoverEachSubscriberReachesOnlyTheirOwnUserKey(t *testing.T) {
	t.Parallel()
	s := newPluginDeliveryScene(t)
	art := provisionFirstParty(t, s.directory, "pushover")
	receiver := newRecordingReceiver(t)
	instance := s.createInstance(art, map[string]any{"endpoint": typedString(receiver.URL),
		"allowPrivateDestinations": typedBoolean(true)}, map[string]any{"apiToken": plainSecret("app-token-1")})

	alice, aliceID := s.subscriber("payload", "unit")
	bob, bobID := s.subscriber("payload", "unit")
	s.setPersonalConfiguration(alice, instance, "phone-a", "alice-user-key")
	s.setPersonalConfiguration(bob, instance, "phone-b", "bob-user-key")
	s.subscribeTarget(alice, instance, nil)
	s.subscribeTarget(bob, instance, nil)

	ids := s.deliverMessage("alarm")
	if len(ids) != 2 {
		t.Fatalf("Deliveries = %v", ids)
	}
	s.attempt()
	for _, id := range ids {
		if got := s.status(id); got != "accepted/accepted/1" {
			t.Fatalf("Delivery %s = %s", id, got)
		}
	}

	requests := receiver.requests()
	if len(requests) != 2 {
		t.Fatalf("receiver got %d requests", len(requests))
	}
	want := map[string]string{"alice-user-key": "phone-a", "bob-user-key": "phone-b"}
	for _, request := range requests {
		user, device := request.form.Get("user"), request.form.Get("device")
		if request.form.Get("token") != "app-token-1" || want[user] == "" || want[user] != device {
			t.Fatalf("request mixes subscribers: %v", request.form)
		}
		delete(want, user)
		for _, other := range []string{"alice-user-key", "bob-user-key"} {
			if other != user && strings.Contains(string(request.body), other) {
				t.Fatalf("request carries the other subscriber's key: %s", request.body)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("subscribers never reached: %v", want)
	}
	for _, subscriber := range []uuid.UUID{aliceID, bobID} {
		if got := s.text(`SELECT count(*)::text FROM deliveries WHERE subscriber_id = $1 AND state = 'accepted'`, subscriber); got != "1" {
			t.Fatalf("subscriber %s has %s accepted Deliveries", subscriber, got)
		}
	}
	view := string(s.adminRequest(http.MethodGet, "/api/v1/instances/"+instance.String(), "", http.StatusOK).Data)
	for _, leaked := range []string{"app-token-1", "alice-user-key", "bob-user-key"} {
		if strings.Contains(view, leaked) {
			t.Fatalf("instance view leaked %q: %s", leaked, view)
		}
	}
}
