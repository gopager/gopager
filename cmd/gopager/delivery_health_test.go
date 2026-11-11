package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
)

// deprecatedLevelFields declares the schema "incident" with the field "level" deprecated.
const deprecatedLevelFields = `{"level":{"kind":"integer","deprecated":true},"city":{"kind":"string"},
	"owner":{"kind":"object","properties":{"name":{"kind":"string"}}}}`

// subscriptionHealth reads a Subscription through the API.
func (f *deliveryFixture) subscriptionHealth(bearer string, id uuid.UUID) (health string, reason *string, revision int64) {
	f.t.Helper()
	response := f.subCall(bearer, http.MethodGet, "/api/v1/subscriptions/"+id.String(), nil)
	if response.status != http.StatusOK {
		f.t.Fatalf("read returned %d/%s", response.status, response.Code)
	}
	var read delivery.Subscription
	if err := json.Unmarshal(response.Data, &read); err != nil {
		f.t.Fatal(err)
	}
	return read.Health, read.HealthReason, read.Revision
}

// healthFacts returns the data of the subscription.health-changed facts of a Subscription.
func (f *deliveryFixture) healthFacts(id uuid.UUID) []string {
	f.t.Helper()
	return f.subStrings(`SELECT envelope->'data'::text FROM domain_events
		WHERE type = 'subscription.health-changed' AND envelope->'aggregateRef'->>'id' = $1 ORDER BY seq`, id.String())
}

// requireHealth fails unless the Subscription has the health and reason at the revision.
func (f *deliveryFixture) requireHealth(bearer string, id uuid.UUID, health, reason string, revision int64) {
	f.t.Helper()
	gotHealth, gotReason, gotRevision := f.subscriptionHealth(bearer, id)
	if gotHealth != health || gotRevision != revision || (reason == "") != (gotReason == nil) ||
		(gotReason != nil && *gotReason != reason) {
		f.t.Fatalf("health = %s/%v at revision %d, want %s/%q at %d", gotHealth, gotReason, gotRevision, health, reason, revision)
	}
}

// TestFanOutMarksSubscriptionInvalidWhenItsFieldIsDeprecated proves the health life cycle: a
// field deprecated after the save makes the next fan-out mark the Subscription invalid without a
// new revision, once, with one fact; it creates nothing afterwards; a save that compiles makes it
// valid again and appends the second fact.
func TestFanOutMarksSubscriptionInvalidWhenItsFieldIsDeprecated(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	unit := f.unit("Engine 1", nil)
	broken := f.subCreated(bearer, subInput(instance, map[string]any{"filter": levelAtLeast("2")}))
	healthy := f.subCreated(bearer, subInput(instance, nil))
	if broken.Health != delivery.HealthValid || broken.HealthReason != nil {
		t.Fatalf("created Subscription = %+v", broken)
	}
	f.replaceAttributeFields("incident", deprecatedLevelFields)

	f.messageFact("message.created", f.message(f.minuteAgo(), "ONE", unit), nil)
	f.fanOutAll()
	f.requireHealth(bearer, broken.ID, delivery.HealthInvalid, delivery.ReasonFieldUnavailable, 1)
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].SubscriptionID != healthy.ID {
		t.Fatalf("Deliveries = %+v, want only the healthy Subscription's", listed)
	}
	if facts := f.healthFacts(broken.ID); len(facts) != 1 ||
		facts[0] != `{"health": "invalid", "reason": "field-unavailable", "revision": "1"}` {
		t.Fatalf("health facts = %v", facts)
	}

	f.messageFact("message.created", f.message(f.minuteAgo(), "TWO", unit), nil)
	f.fanOutAll()
	if facts := f.healthFacts(broken.ID); len(facts) != 1 {
		t.Fatalf("a later fact appended %d health facts", len(facts))
	}
	if listed := f.deliveries(bearer, "?subscriptionId="+broken.ID.String()); len(listed) != 0 {
		t.Fatalf("an invalid Subscription created %+v", listed)
	}

	saved := f.subUpdate(bearer, broken.ID, 1, subInput(instance, map[string]any{"filter": cityIs("Bonn")}))
	if saved.status != http.StatusOK {
		t.Fatalf("valid save returned %d/%s", saved.status, saved.Code)
	}
	f.requireHealth(bearer, broken.ID, delivery.HealthValid, "", 2)
	if facts := f.healthFacts(broken.ID); len(facts) != 2 || facts[1] != `{"health": "valid", "revision": "2"}` {
		t.Fatalf("health facts = %v", facts)
	}
}

// TestSaveRejectsAFilterOnADeprecatedField proves that a save compiles strictly: a field
// deprecated before the save is refused.
func TestSaveRejectsAFilterOnADeprecatedField(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", deprecatedLevelFields)
	f.subRejected(bearer, subInput(instance, map[string]any{"filter": levelAtLeast("2")}), http.StatusBadRequest, "INVALID_INPUT")
	f.subRejected(bearer, subInput(instance, map[string]any{"filter": map[string]any{"op": "eq",
		"field": map[string]any{"namespace": "result", "path": []string{uuid.NewString(), "level"}},
		"value": map[string]any{"kind": "integer", "value": "1"}}}), http.StatusBadRequest, "INVALID_INPUT")
}

// TestFanOutMarksSubscriptionInvalidWhenTheSubscriberLosesTheFieldGroup proves that a subscriber
// who can no longer read the field group of a filter field makes the Subscription invalid
// instead of reading the field as absent.
func TestFanOutMarksSubscriptionInvalidWhenTheSubscriberLosesTheFieldGroup(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, principal := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	created := f.subCreated(bearer, subInput(instance, map[string]any{"filter": levelAtLeast("2")}))
	f.exec(`UPDATE identity_field_permissions SET field_names = '["payload","unit"]'::jsonb WHERE subject_id = $1`, principal)

	f.messageFact("message.created", f.message(f.minuteAgo(), "LOST", f.unit("Engine 1", nil)), nil)
	f.fanOutAll()
	f.requireHealth(bearer, created.ID, delivery.HealthInvalid, delivery.ReasonFieldUnavailable, 1)
	if listed := f.deliveries(bearer, ""); len(listed) != 0 {
		t.Fatalf("Deliveries = %+v", listed)
	}
}

// TestStartCheckMarksSubscriptionInvalidAndCancelsTheDelivery proves that a Delivery prepared
// while its Subscription was valid is cancelled at the start check once a field it reads is
// deprecated, and that the Subscription becomes invalid.
func TestStartCheckMarksSubscriptionInvalidAndCancelsTheDelivery(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	workflow := f.resultWorkflow()
	created := f.subCreated(bearer, subInput(instance, map[string]any{"filter": levelAtLeast("2")}))
	message := f.message(f.minuteAgo(), "START", f.unit("Engine 1", nil))
	f.publishLevel(workflow, message, "5")
	f.messageFact("message.created", message, nil)
	f.fanOutAll()
	if listed := f.deliveries(bearer, ""); len(listed) != 1 || listed[0].State != delivery.Prepared {
		t.Fatalf("Deliveries = %+v, want one prepared", listed)
	}

	f.replaceAttributeFields("incident", deprecatedLevelFields)
	f.attempt()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].State != delivery.Cancelled || listed[0].OutcomeCode == nil ||
		*listed[0].OutcomeCode != delivery.CodeNoLongerMatching {
		t.Fatalf("Deliveries = %+v, want one cancelled no-longer-matching", listed)
	}
	if calls := f.targets.calls(); len(calls) != 0 {
		t.Fatalf("a cancelled Delivery was sent: %+v", calls)
	}
	f.requireHealth(bearer, created.ID, delivery.HealthInvalid, delivery.ReasonFieldUnavailable, 1)
	if facts := f.healthFacts(created.ID); len(facts) != 1 {
		t.Fatalf("health facts = %v", facts)
	}
}

// TestSubscriptionLayoutMustBeUsableByTheAuthor proves that a save naming a Layout that the
// author may not use is refused, and that a Layout of an Agency the author reads is accepted.
func TestSubscriptionLayoutMustBeUsableByTheAuthor(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	instance := f.instance(true)
	layout := f.layoutCreated(f.layoutManager(), layoutBody("text", []any{textBlock("x")}))
	other := seedAgency(t, f.store)
	bearer, principal := f.caller([]string{"subscription.manage-own", "delivery.read", "message.read"}, authz.Scope{Kind: "all"}, fanOutGroups)
	f.exec(`UPDATE identity_grants SET actions = '["subscription.manage-own","delivery.read"]'::jsonb WHERE subject_id = $1`, principal)
	f.subGrant(principal, "message.read", authz.Scope{Kind: "agency", References: []string{other.String()}})
	f.subRejected(bearer, subInput(instance, map[string]any{"layoutId": layout.ID}), http.StatusBadRequest, "INVALID_INPUT")
	f.subRejected(bearer, subInput(instance, map[string]any{"layoutId": uuid.New()}), http.StatusBadRequest, "INVALID_INPUT")

	f.subGrant(principal, "message.read", authz.Scope{Kind: "agency", References: []string{f.agency.String()}})
	f.subCreated(bearer, subInput(instance, map[string]any{"layoutId": layout.ID}))
}

// TestLayoutSaveRejectsUnknownAndDeprecatedFields proves that a Layout is checked against the
// registry when it is saved.
func TestLayoutSaveRejectsUnknownAndDeprecatedFields(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	manager := f.layoutManager()
	f.attributeSchema("incident", deprecatedLevelFields)
	field := func(path ...string) []any {
		return []any{map[string]any{"kind": "field", "field": map[string]any{"namespace": "attributes", "path": path}}}
	}
	for name, path := range map[string][]string{"unknown schema": {"nothing", "level"}, "unknown field": {"incident", "nothing"},
		"deprecated field": {"incident", "level"}} {
		body := layoutBody("text", field(path...))
		body["agencies"] = []uuid.UUID{f.agency}
		if got := f.subCall(manager, http.MethodPost, "/api/v1/layouts", body); got.status != http.StatusBadRequest ||
			got.Code != "INVALID_INPUT" {
			t.Errorf("%s: layout create returned %d/%s", name, got.status, got.Code)
		}
	}
	f.layoutCreated(manager, layoutBody("text", field("incident", "city")))
}
