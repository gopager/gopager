package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
)

// fanOutGroups are the field groups the Subscriptions of the fan-out tests may read.
var fanOutGroups = []string{"payload", "unit", "derived", "gateway", "address"}

// newDeliveryFixtureWithoutProvider composes the Core as production does without a target
// provider: the Delivery configuration is empty.
func newDeliveryFixtureWithoutProvider(t *testing.T) *deliveryFixture {
	t.Helper()
	store := newManagementStore(t)
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	shared := mustComposeCore(t, store)
	shared.deliveryConfig = deliveryConfig{}
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	query := &queryFixture{t: t, store: store, origin: origin, client: backend.Client(), gateway: uuid.New(),
		network: uuid.New(), agency: seedAgency(t, store)}
	admin := uuid.New()
	return &deliveryFixture{queryFixture: query, core: shared, targets: newControlledTargets(),
		admin: authz.Authority{PrincipalID: admin.String(), Revision: 1, Active: true,
			Grants: []authz.Grant{{SubjectID: admin.String(), SubjectKind: "principal", Scope: authz.Scope{Kind: "all"},
				Actions: []string{"plugin.configure"}}}}}
}

// subGrant gives an account one more grant.
func (f *deliveryFixture) subGrant(principal uuid.UUID, action string, scope authz.Scope) {
	f.t.Helper()
	actions, _ := json.Marshal([]string{action})
	encoded, _ := json.Marshal(scope)
	f.exec(`INSERT INTO identity_grants (id, subject_id, subject_kind, actions, scope) VALUES ($1, $2, 'principal', $3::jsonb, $4::jsonb)`,
		uuid.New(), principal, actions, encoded)
}

// subUnitParameter stores a typed string parameter of a Unit for an instance.
func (f *deliveryFixture) subUnitParameter(instance, unit uuid.UUID, key, value string) {
	f.t.Helper()
	encoded, _ := json.Marshal(map[string]any{"kind": "string", "value": value})
	f.exec(`INSERT INTO integration_unit_parameters (instance_id, unit_id, key, value) VALUES ($1, $2, $3, $4::jsonb)`,
		instance, unit, key, encoded)
}

// subRecipients returns the recipient input of each invocation keyed by the split key of its Delivery.
func (f *deliveryFixture) subRecipients(listed []delivery.Delivery) map[string]string {
	f.t.Helper()
	splitOf := map[uuid.UUID]string{}
	for _, item := range listed {
		splitOf[item.ID] = item.SplitKey
	}
	recipients := map[string]string{}
	for _, call := range f.targets.calls() {
		var recipient string
		if err := json.Unmarshal(call.Input.Inputs["recipient"].Value, &recipient); err != nil {
			f.t.Fatalf("recipient input: %v", err)
		}
		recipients[splitOf[call.DeliveryID]] = recipient
	}
	return recipients
}

// levelWorkflow stores a Workflow whose active version outputs the integer "level".
func (f *deliveryFixture) levelWorkflow() uuid.UUID {
	f.t.Helper()
	protocol := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	pocsag := processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"pocsag"`)}
	three := processing.Value{Kind: processing.IntegerValue, Value: json.RawMessage(`"3"`)}
	definition, err := json.Marshal(processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &protocol, Value: &pocsag},
		OutputSchema: map[string]processing.FieldSchema{"level": {Kind: processing.IntegerValue}},
		Outputs:      map[string]processing.Assignment{"level": {Literal: &three}}})
	if err != nil {
		f.t.Fatal(err)
	}
	workflow, version := uuid.New(), uuid.New()
	f.exec(`INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', now(), now())`, workflow)
	f.exec(`INSERT INTO processing_workflow_agencies(workflow_id,agency_id) SELECT $1, id FROM agencies`, workflow)
	f.exec(`INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by) VALUES ($1, $2, $3::jsonb, now(), $4)`,
		version, workflow, definition, uuid.New())
	f.exec(`UPDATE processing_workflows SET active_version_id = $2 WHERE id = $1`, workflow, version)
	return workflow
}

// publishLevel publishes a result of the Workflow for the Message with the given level and
// returns the result ID. Its stored classification is what the level Workflow reads: `derived`
// and `payload`.
func (f *deliveryFixture) publishLevel(workflow, message uuid.UUID, level string) uuid.UUID {
	f.t.Helper()
	return f.publishLevelClassified(workflow, message, level, `["derived","payload"]`)
}

// publishLevelClassified publishes a level result with the given stored field groups.
func (f *deliveryFixture) publishLevelClassified(workflow, message uuid.UUID, level, groups string) uuid.UUID {
	f.t.Helper()
	candidate, result := uuid.New(), uuid.New()
	f.exec(`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision, configuration_revision,
		source_revision, context_revision, state, selected, due_at, created_at, updated_at)
		SELECT $1, t.id, w.id, w.active_version_id, 1, 1, 1, 1, 'completed', true, now(), now(), now()
		FROM transmissions t, processing_workflows w WHERE t.message_id = $3 AND w.id = $2`, candidate, workflow, message)
	f.exec(`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
		VALUES ($1, $2, 1, $3::jsonb, $5::jsonb, now(), now(), $4)`, result, candidate,
		`{"selected":true,"fields":{"level":{"state":"present","value":{"kind":"integer","value":"`+level+`"}}}}`, message, groups)
	return result
}

// levelAtLeast is the typed predicate "result.<workflow>.level >= minimum".
func levelAtLeast(workflow uuid.UUID, minimum string) map[string]any {
	return map[string]any{"op": "gte", "field": map[string]any{"namespace": "result", "path": []string{workflow.String(), "level"}},
		"value": map[string]any{"kind": "integer", "value": minimum}}
}

// TestSubscriptionProcessingCompletedSelectionDependsOnPublishedResult proves that a Subscription on
// processing.completed with a result filter delivers only facts whose published result is
// visible and satisfies the filter, and that handling the fact twice creates one Delivery.
func TestSubscriptionProcessingCompletedSelectionDependsOnPublishedResult(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	workflow := f.levelWorkflow()
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed"},
		"filter": levelAtLeast(workflow, "2")}))
	unit := f.unit("Engine 1", nil)
	high := f.message(f.now().Add(-time.Minute), "HIGH", unit)
	low := f.message(f.now().Add(-time.Minute), "LOW", unit)
	highResult, lowResult := f.publishLevel(workflow, high, "3"), f.publishLevel(workflow, low, "1")

	f.messageFact("message.created", high, nil)
	f.fanOutAll()
	if listed := f.deliveries(bearer, ""); len(listed) != 0 {
		t.Fatalf("message.created created %d Deliveries for a processing.completed Subscription", len(listed))
	}
	completed := f.messageFact("processing.completed", high, map[string]any{"resultId": highResult})
	f.messageFact("processing.completed", low, map[string]any{"resultId": lowResult})
	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].DomainEventID != completed || listed[0].MessageID == nil || *listed[0].MessageID != high {
		t.Fatalf("Deliveries = %+v, want exactly the level-3 fact", listed)
	}

	f.exec(`DELETE FROM consumer_offsets WHERE consumer = $1`, delivery.FanOutConsumerName)
	f.fanOutAll()
	if again := f.deliveries(bearer, ""); len(again) != 1 {
		t.Fatalf("handling the fact twice left %d Deliveries", len(again))
	}
}

// TestSubscriptionProcessingCompletedWithoutVisibleResultCreatesNothing proves that a processing.completed
// fact whose result is not part of the subscriber's projection creates no Delivery.
func TestSubscriptionProcessingCompletedWithoutVisibleResultCreatesNothing(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.levelWorkflow()
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed"}}))
	message := f.message(f.now().Add(-time.Minute), "NO RESULT", f.unit("Engine 1", nil))
	f.messageFact("processing.completed", message, map[string]any{"resultId": uuid.New()})

	f.fanOutAll()
	if listed := f.deliveries(bearer, ""); len(listed) != 0 {
		t.Fatalf("a fact without a visible result created %+v", listed)
	}
}

// TestSubscriptionProcessingCompletedWithUnreadableResultGroupsCreatesNothing proves that a
// published result whose stored field groups include one the subscriber cannot read neither
// triggers a Delivery nor reaches another Delivery's input.
func TestSubscriptionProcessingCompletedWithUnreadableResultGroupsCreatesNothing(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit", "derived")
	instance := f.instance(true)
	workflow := f.levelWorkflow()
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed", "message.created"}}))
	message := f.message(f.now().Add(-time.Minute), "HIDDEN RESULT", f.unit("Engine 2", nil))
	result := f.publishLevelClassified(workflow, message, "9", `["derived","payload","quality"]`)
	f.messageFact("processing.completed", message, map[string]any{"resultId": result})
	created := f.messageFact("message.created", message, nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].DomainEventID != created {
		t.Fatalf("Deliveries = %+v, want only the message.created one", listed)
	}
	f.attempt()
	for _, call := range f.targets.calls() {
		if strings.Contains(call.Input.Body, workflow.String()) || strings.Contains(call.Input.Body, `"9"`) {
			t.Fatalf("an unreadable result reached the input: %q", call.Input.Body)
		}
	}
}

// TestSubscriptionFanOutIgnoresReprocessFacts proves that a fact recorded by a historical reprocess creates
// no Delivery while an ordinary fact for the same Subscription does.
func TestSubscriptionFanOutIgnoresReprocessFacts(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	f.subCreated(bearer, subInput(f.instance(true), nil))
	unit := f.unit("Engine 1", nil)
	replayed, ordinary := f.message(f.now().Add(-time.Minute), "R", unit), f.message(f.now().Add(-time.Minute), "N", unit)
	f.fact("message.created", map[string]any{"messageId": replayed, "messageTime": f.now().Format(time.RFC3339Nano)},
		factOptions{replay: events.ReplayReprocess})
	f.messageFact("message.created", ordinary, nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || *listed[0].MessageID != ordinary {
		t.Fatalf("Deliveries = %+v, want only the ordinary fact", listed)
	}
}

// TestSubscriptionFanOutIgnoresFactsOlderThanTheWindow proves that a fact recorded more than 24 hours ago
// creates no Delivery, even for a Subscription that existed at that time.
func TestSubscriptionFanOutIgnoresFactsOlderThanTheWindow(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	created := f.subCreated(bearer, subInput(f.instance(true), nil))
	f.exec(`UPDATE subscriptions SET created_at = $2 WHERE id = $1`, created.ID, f.now().Add(-72*time.Hour))
	unit := f.unit("Engine 1", nil)
	old, recent := f.message(f.now().Add(-time.Minute), "OLD", unit), f.message(f.now().Add(-time.Minute), "NEW", unit)
	longAgo := f.now().Add(-25 * time.Hour)
	f.fact("message.created", map[string]any{"messageId": old, "messageTime": longAgo.Format(time.RFC3339Nano)},
		factOptions{recordedAt: &longAgo})
	f.messageFact("message.created", recent, nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || *listed[0].MessageID != recent {
		t.Fatalf("Deliveries = %+v, want only the recent fact", listed)
	}
}

// TestSubscriptionFanOutDoesNotBackfillFactsBeforeTheSubscription proves that a Subscription created after
// a fact was recorded never receives that fact.
func TestSubscriptionFanOutDoesNotBackfillFactsBeforeTheSubscription(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	unit := f.unit("Engine 1", nil)
	earlier, later := f.message(f.now().Add(-time.Minute), "EARLY", unit), f.message(f.now().Add(-time.Minute), "LATE", unit)
	before := f.now().Add(-time.Hour)
	f.fact("message.created", map[string]any{"messageId": earlier, "messageTime": before.Format(time.RFC3339Nano)},
		factOptions{recordedAt: &before})
	f.subCreated(bearer, subInput(f.instance(true), nil))
	f.messageFact("message.created", later, nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || *listed[0].MessageID != later {
		t.Fatalf("Deliveries = %+v, want only the fact after creation", listed)
	}
}

// TestSubscriptionFanOutSkipsPausedSubscriptions proves that a paused Subscription gets nothing while an
// active one receives the same fact.
func TestSubscriptionFanOutSkipsPausedSubscriptions(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	paused := f.subCreated(bearer, subInput(instance, map[string]any{"state": "paused"}))
	active := f.subCreated(bearer, subInput(instance, nil))
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "P", f.unit("Engine 1", nil)), nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].SubscriptionID != active.ID || listed[0].SubscriptionID == paused.ID {
		t.Fatalf("Deliveries = %+v, want one for the active Subscription", listed)
	}
}

// TestSubscriptionFanOutRespectsTheSubscribersReadRights proves that a Message the subscriber may not read
// creates no Delivery for it while another subscriber with access receives one.
func TestSubscriptionFanOutRespectsTheSubscribersReadRights(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	instance := f.instance(true)
	elsewhere, principal := f.caller([]string{"subscription.manage-own", "delivery.read"}, authz.Scope{Kind: "all"}, fanOutGroups)
	f.subGrant(principal, "message.read", authz.Scope{Kind: "agency", References: []string{seedAgency(t, f.store).String()}})
	allowed, allowedID := f.subscriber(fanOutGroups...)
	f.subCreated(elsewhere, subInput(instance, nil))
	f.subCreated(allowed, subInput(instance, nil))
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "SECRET", f.unit("Engine 1", nil)), nil)

	f.fanOutAll()
	if listed := f.deliveriesOf(elsewhere, principal); len(listed) != 0 {
		t.Fatalf("subscriber without read access received %+v", listed)
	}
	if listed := f.deliveriesOf(allowed, allowedID); len(listed) != 1 {
		t.Fatalf("subscriber with access received %+v", listed)
	}
}

// TestUnsplitDeliveryWithConflictingUnitParametersFails proves that Units supplying different
// values for one input make the single Delivery failed with parameter-conflict, sending nothing.
func TestUnsplitDeliveryWithConflictingUnitParametersFails(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	one, two := f.unit("Engine 1", nil), f.unit("Engine 2", nil)
	f.subUnitParameter(instance, one, "recipient", "alice")
	f.subUnitParameter(instance, two, "recipient", "bob")
	f.subCreated(bearer, subInput(instance, map[string]any{"targets": []any{map[string]any{"instanceId": instance}}}))
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "FIRE", one, two), nil)

	f.fanOutAll()
	f.attempt()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].State != delivery.Failed || listed[0].OutcomeCode == nil ||
		*listed[0].OutcomeCode != delivery.CodeParameterConflict || listed[0].SplitKey != "" {
		t.Fatalf("Deliveries = %+v, want one failed parameter-conflict", listed)
	}
	if calls := f.targets.calls(); len(calls) != 0 {
		t.Fatalf("a conflicting Delivery was sent: %+v", calls)
	}
}

// TestSplitDeliveriesUseTheirOwnUnitParameters proves that splitting by Unit yields one
// Delivery per Unit keyed by the Unit ID, each sent with its own Unit's parameter.
func TestSplitDeliveriesUseTheirOwnUnitParameters(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	one, two := f.unit("Engine 1", nil), f.unit("Engine 2", nil)
	f.subUnitParameter(instance, one, "recipient", "alice")
	f.subUnitParameter(instance, two, "recipient", "bob")
	f.subCreated(bearer, subInput(instance, map[string]any{"splitByUnit": true,
		"targets": []any{map[string]any{"instanceId": instance}}}))
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "FIRE", one, two), nil)

	f.fanOutAll()
	f.attempt()
	listed := f.deliveries(bearer, "")
	if len(listed) != 2 {
		t.Fatalf("Deliveries = %+v, want one per Unit", listed)
	}
	recipients := f.subRecipients(listed)
	if len(recipients) != 2 || recipients[one.String()] != "alice" || recipients[two.String()] != "bob" {
		t.Fatalf("recipients by split key = %v", recipients)
	}
	for _, item := range listed {
		if item.State != delivery.Accepted {
			t.Fatalf("Delivery %+v not accepted", item)
		}
	}
}

// TestUnsplitDeliveryWithAgreeingUnitParametersIsSent proves that the same value from every
// Unit gives one Delivery carrying that value.
func TestUnsplitDeliveryWithAgreeingUnitParametersIsSent(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	one, two := f.unit("Engine 1", nil), f.unit("Engine 2", nil)
	f.subUnitParameter(instance, one, "recipient", "shared")
	f.subUnitParameter(instance, two, "recipient", "shared")
	f.subCreated(bearer, subInput(instance, map[string]any{"targets": []any{map[string]any{"instanceId": instance}}}))
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "FIRE", one, two), nil)

	f.fanOutAll()
	f.attempt()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].State != delivery.Accepted {
		t.Fatalf("Deliveries = %+v", listed)
	}
	if recipients := f.subRecipients(listed); recipients[""] != "shared" {
		t.Fatalf("recipients = %v", recipients)
	}
}

// TestSubscriptionPayloadContentNeverBecomesTargetInput proves that a URL inside the Payload reaches only
// the rendered body: the provider's inputs hold exactly the declared constants.
func TestSubscriptionPayloadContentNeverBecomesTargetInput(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.subCreated(bearer, subInput(instance, nil))
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "https://evil.example/callback", f.unit("Engine 1", nil)), nil)

	f.fanOutAll()
	f.attempt()
	calls := f.targets.calls()
	if len(calls) != 1 {
		t.Fatalf("invocations = %+v", calls)
	}
	encoded, err := json.Marshal(calls[0].Input.Inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls[0].Input.Inputs) != 1 || strings.Contains(string(encoded), "evil") {
		t.Fatalf("inputs = %s, want only the declared recipient", encoded)
	}
}

// TestSubscriptionChangesCancelUnstartedDeliveries proves that pausing, updating and deleting a
// Subscription cancel its unstarted Deliveries with the matching outcome code and that the
// history stays listed after deletion.
func TestSubscriptionChangesCancelUnstartedDeliveries(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	unit := f.unit("Engine 1", nil)
	created := f.subCreated(bearer, subInput(instance, nil))
	prepare := func(text string) uuid.UUID {
		message := f.message(f.now().Add(-time.Minute), text, unit)
		f.messageFact("message.created", message, nil)
		f.fanOutAll()
		return message
	}
	outcomes := func() map[uuid.UUID]string {
		result := map[uuid.UUID]string{}
		for _, item := range f.deliveries(bearer, "") {
			code := ""
			if item.OutcomeCode != nil {
				code = *item.OutcomeCode
			}
			result[*item.MessageID] = item.State + "/" + code
		}
		return result
	}

	paused := prepare("ONE")
	if got := outcomes()[paused]; got != "prepared/" {
		t.Fatalf("prepared Delivery = %q", got)
	}
	if got := f.subUpdate(bearer, created.ID, 1, subInput(instance, map[string]any{"state": "paused"})); got.status != 200 {
		t.Fatalf("pause = %d/%s", got.status, got.Code)
	}
	if got := f.subUpdate(bearer, created.ID, 2, subInput(instance, nil)); got.status != 200 {
		t.Fatalf("resume = %d/%s", got.status, got.Code)
	}
	changed := prepare("TWO")
	if got := f.subUpdate(bearer, created.ID, 3, subInput(instance, map[string]any{"maxAgeSeconds": 3600})); got.status != 200 {
		t.Fatalf("update = %d/%s", got.status, got.Code)
	}
	deleted := prepare("THREE")
	if got := f.subDelete(bearer, created.ID, 4); got.status != 200 {
		t.Fatalf("delete = %d/%s", got.status, got.Code)
	}

	got := outcomes()
	want := map[uuid.UUID]string{paused: "cancelled/" + delivery.CodeSubscriptionPaused,
		changed: "cancelled/" + delivery.CodeSubscriptionChanged, deleted: "cancelled/" + delivery.CodeSubscriptionDeleted}
	if len(got) != len(want) {
		t.Fatalf("history = %v, want %v", got, want)
	}
	for message, state := range want {
		if got[message] != state {
			t.Errorf("Delivery of %v = %q, want %q", message, got[message], state)
		}
	}
	f.attempt()
	if calls := f.targets.calls(); len(calls) != 0 {
		t.Fatalf("cancelled Deliveries were sent: %+v", calls)
	}
}

// TestSubscriptionMaxAgeBoundsDeliveryExpiry proves that an input already older than the
// Subscription's age limit creates no Delivery, that a fresh one expires at its age anchor
// plus the limit, and that without a limit the ceiling is 24 hours after creation.
func TestSubscriptionMaxAgeBoundsDeliveryExpiry(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	unit := f.unit("Engine 1", nil)
	f.subCreated(bearer, subInput(instance, map[string]any{"maxAgeSeconds": 60}))
	stale := f.message(f.now().Add(-10*time.Minute), "STALE", unit)
	freshObserved := f.now().Add(-5 * time.Second).Truncate(time.Microsecond)
	fresh := f.message(freshObserved, "FRESH", unit)
	f.messageFact("message.created", stale, nil)
	f.messageFact("message.created", fresh, nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || *listed[0].MessageID != fresh {
		t.Fatalf("Deliveries = %+v, want only the fresh Message", listed)
	}
	if want := freshObserved.Add(60 * time.Second); !listed[0].ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt = %v, want anchor plus 60 s = %v", listed[0].ExpiresAt, want)
	}
}

// TestDeliveryExpiryCeilingIsOneDayAfterCreation proves the default expiry without an age limit.
func TestDeliveryExpiryCeilingIsOneDayAfterCreation(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	f.subCreated(bearer, subInput(f.instance(true), nil))
	f.messageFact("message.created", f.message(f.now().Add(-time.Minute), "ALARM", f.unit("Engine 1", nil)), nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 {
		t.Fatalf("Deliveries = %+v", listed)
	}
	if got := listed[0].ExpiresAt.Sub(listed[0].CreatedAt); got != delivery.FanOutWindow {
		t.Fatalf("expiresAt - createdAt = %v, want %v", got, delivery.FanOutWindow)
	}
}

// TestSubscriptionGatewayStateTriggerNeedsGatewayReadOnAnAgency proves that a gateway.state-changed
// fact as the intake producer writes it (the Gateway in the aggregate reference) delivers the new state to a subscriber with gateway.read on one of the Gateway's
// Agencies and nothing to one without.
func TestSubscriptionGatewayStateTriggerNeedsGatewayReadOnAnAgency(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	instance := f.instance(true)
	reader, principal := f.subscriber(fanOutGroups...)
	f.subGrant(principal, "gateway.read", authz.Scope{Kind: "agency", References: []string{f.agency.String()}})
	outsider, outsiderID := f.subscriber(fanOutGroups...)
	for _, bearer := range []string{reader, outsider} {
		f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"gateway.state-changed"}}))
	}
	f.producedFact(func(now time.Time) (any, uuid.UUID, error) {
		contact := now
		fact := events.IntakeFact{ID: uuid.New(), Type: events.GatewayStateChanged, GatewayID: f.gateway, Revision: 2,
			OccurredAt: now, OldState: "active", NewState: "contact-lost", ContactTime: &contact}
		envelope, err := fact.Envelope(now)
		return envelope, fact.ID, err
	})

	f.fanOutAll()
	f.attempt()
	if listed := f.deliveriesOf(outsider, outsiderID); len(listed) != 0 {
		t.Fatalf("subscriber without gateway.read received %+v", listed)
	}
	listed := f.deliveriesOf(reader, principal)
	if len(listed) != 1 || listed[0].EventType != "gateway.state-changed" {
		t.Fatalf("Deliveries = %+v", listed)
	}
	calls := f.targets.calls()
	if len(calls) != 1 || !strings.Contains(calls[0].Input.Body, "contact-lost") {
		t.Fatalf("invocations = %+v, want the new state in the body", calls)
	}
}
