package main

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
)

// subResponse is a raw API response with the error parameters a management helper hides.
type subResponse struct {
	status     int
	Code       string          `json:"code"`
	Parameters map[string]any  `json:"parameters"`
	Data       json.RawMessage `json:"data"`
}

// subCall sends an authenticated JSON request and returns the status and decoded envelope.
func (f *deliveryFixture) subCall(bearer, method, path string, body any) subResponse {
	f.t.Helper()
	payload := ""
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		payload = string(encoded)
	}
	request, err := http.NewRequestWithContext(f.t.Context(), method, f.origin+path, strings.NewReader(payload))
	if err != nil {
		f.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", f.origin)
	if bearer != "" {
		request.Header.Set("Authorization", bearer)
	}
	response, err := f.client.Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		f.t.Fatal(err)
	}
	result := subResponse{status: response.StatusCode}
	if err := json.Unmarshal(raw, &result); err != nil {
		f.t.Fatalf("%s %s: undecodable response %q: %v", method, path, raw, err)
	}
	return result
}

// subInput returns a valid personal create body for a message.created Subscription with one
// target. Entries of extra replace body properties; a nil entry removes the property.
func subInput(instance uuid.UUID, extra map[string]any) map[string]any {
	body := map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "kind": "personal", "state": "active",
		"eventTypes": []string{"message.created"}, "targets": []any{target(instance)}}
	for key, value := range extra {
		if value == nil {
			delete(body, key)
			continue
		}
		body[key] = value
	}
	return body
}

// subCreate posts a Subscription and returns the raw response.
func (f *deliveryFixture) subCreate(bearer string, body map[string]any) subResponse {
	f.t.Helper()
	return f.subCall(bearer, http.MethodPost, "/api/v1/subscriptions", body)
}

// subCreated posts a Subscription that must be accepted and returns it.
func (f *deliveryFixture) subCreated(bearer string, body map[string]any) delivery.Subscription {
	f.t.Helper()
	response := f.subCreate(bearer, body)
	if response.status != http.StatusCreated {
		f.t.Fatalf("create returned %d/%s: %v", response.status, response.Code, response.Parameters)
	}
	var created delivery.Subscription
	if err := json.Unmarshal(response.Data, &created); err != nil {
		f.t.Fatal(err)
	}
	return created
}

// subRejected posts a Subscription that must be refused with the status and code.
func (f *deliveryFixture) subRejected(bearer string, body map[string]any, status int, code string) subResponse {
	f.t.Helper()
	response := f.subCreate(bearer, body)
	if response.status != status || response.Code != code {
		f.t.Fatalf("create returned %d/%s (%v), want %d/%s", response.status, response.Code, response.Parameters, status, code)
	}
	return response
}

// subUpdate replaces a Subscription at the expected revision.
func (f *deliveryFixture) subUpdate(bearer string, id uuid.UUID, revision int64, body map[string]any) subResponse {
	f.t.Helper()
	delete(body, "commandId")
	body["expectedRevision"] = strconv.FormatInt(revision, 10)
	return f.subCall(bearer, http.MethodPut, "/api/v1/subscriptions/"+id.String(), body)
}

// subDelete removes a Subscription at the expected revision.
func (f *deliveryFixture) subDelete(bearer string, id uuid.UUID, revision int64) subResponse {
	f.t.Helper()
	return f.subCall(bearer, http.MethodDelete, "/api/v1/subscriptions/"+id.String(),
		map[string]any{"schemaVersion": 1, "expectedRevision": strconv.FormatInt(revision, 10)})
}

// subIDs lists the Subscription IDs the caller sees.
func (f *deliveryFixture) subIDs(bearer string) []uuid.UUID {
	f.t.Helper()
	response := f.subCall(bearer, http.MethodGet, "/api/v1/subscriptions", nil)
	if response.status != http.StatusOK {
		f.t.Fatalf("list returned %d/%s", response.status, response.Code)
	}
	var page struct {
		Items []delivery.Subscription `json:"items"`
	}
	if err := json.Unmarshal(response.Data, &page); err != nil {
		f.t.Fatal(err)
	}
	ids := []uuid.UUID{}
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

// subStrings runs a query whose single column is text and returns the rows.
func (f *deliveryFixture) subStrings(statement string, args ...any) []string {
	f.t.Helper()
	var out []string
	err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), statement, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return err
			}
			out = append(out, value)
		}
		return rows.Err()
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

// eqText is the typed predicate "message.text equals value".
func eqText(value string) map[string]any {
	return map[string]any{"op": "eq", "field": map[string]any{"namespace": "message", "path": []string{"text"}},
		"value": map[string]any{"kind": "string", "value": value}}
}

// TestSubscriptionRejectsHiddenEventDefaultsAndBadSelections proves that a Subscription must
// name 1 to 16 distinct deliverable event types and 1 to 8 targets, with no default selection.
func TestSubscriptionRejectsHiddenEventDefaultsAndBadSelections(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	seventeen := make([]string, 17)
	for i := range seventeen {
		seventeen[i] = "message.created"
	}
	for name, extra := range map[string]map[string]any{
		"no event types":          {"eventTypes": nil},
		"empty event types":       {"eventTypes": []string{}},
		"unknown type":            {"eventTypes": []string{"message.exploded"}},
		"non-deliverable type":    {"eventTypes": []string{"configuration.changed"}},
		"duplicate types":         {"eventTypes": []string{"message.created", "message.created"}},
		"more than sixteen types": {"eventTypes": seventeen},
		"no targets":              {"targets": nil},
		"empty targets":           {"targets": []any{}},
		"duplicate targets":       {"targets": []any{target(instance), target(instance)}},
		"max age out of range":    {"maxAgeSeconds": 86401},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f.subRejected(bearer, subInput(instance, extra), http.StatusBadRequest, "INVALID_INPUT")
		})
	}
	if ids := f.subIDs(bearer); len(ids) != 0 {
		t.Fatalf("rejected requests stored Subscriptions: %v", ids)
	}
}

// TestSubscriptionOnJoinedTransmissionsGetsOneDeliveryPerFact proves that selecting
// message.transmission-joined delivers every joined fact, and that a message.created-only
// Subscription ignores joined facts.
func TestSubscriptionOnJoinedTransmissionsGetsOneDeliveryPerFact(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, joinedOwner := f.subscriber("payload", "unit")
	createdOnly, createdOwner := f.subscriber("payload", "unit")
	instance := f.instance(true)
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"message.transmission-joined"}}))
	f.subCreated(createdOnly, subInput(instance, nil))
	message := f.message(f.now().Add(-time.Minute), "JOIN", f.unit("Engine 1", nil))
	first := f.messageFact("message.transmission-joined", message, map[string]any{"transmissionId": uuid.New()})
	second := f.messageFact("message.transmission-joined", message, map[string]any{"transmissionId": uuid.New()})

	f.fanOutAll()
	listed := f.deliveriesOf(bearer, joinedOwner)
	got := []uuid.UUID{}
	for _, item := range listed {
		got = append(got, item.DomainEventID)
		if item.EventType != "message.transmission-joined" {
			t.Fatalf("unexpected event type %q", item.EventType)
		}
	}
	if len(listed) != 2 || !slices.Contains(got, first) || !slices.Contains(got, second) {
		t.Fatalf("joined Subscription Deliveries = %v, want one per fact %v and %v", got, first, second)
	}
	if other := f.deliveriesOf(createdOnly, createdOwner); len(other) != 0 {
		t.Fatalf("message.created-only Subscription received %d Deliveries for joined facts", len(other))
	}
}

// TestSubscriptionSaveChecksFieldRights proves that filters, layouts and unit splitting are
// accepted only when the subscriber may read every field group they use, and that unknown
// namespaces and ill-typed literals are refused.
func TestSubscriptionSaveChecksFieldRights(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	instance := f.instance(true)
	withoutPayload, _ := f.subscriber("unit")
	withPayload, _ := f.subscriber("payload", "unit")
	payloadOnly, _ := f.subscriber("payload")
	layout := f.layoutCreated(f.layoutManager(), layoutBody("text", []any{
		map[string]any{"kind": "field", "field": map[string]any{"namespace": "message", "path": []string{"text"}}}}))

	f.subRejected(withoutPayload, subInput(instance, map[string]any{"filter": eqText("x")}), http.StatusBadRequest, "INVALID_INPUT")
	f.subCreated(withPayload, subInput(instance, map[string]any{"filter": eqText("x")}))
	f.subRejected(payloadOnly, subInput(instance, map[string]any{"splitByUnit": true}), http.StatusBadRequest, "INVALID_INPUT")
	f.subCreated(withPayload, subInput(instance, map[string]any{"splitByUnit": true}))
	f.subRejected(withoutPayload, subInput(instance, map[string]any{"layoutId": layout.ID}), http.StatusBadRequest, "INVALID_INPUT")
	f.subCreated(withPayload, subInput(instance, map[string]any{"layoutId": layout.ID}))
	f.subRejected(withPayload, subInput(instance, map[string]any{"layoutId": uuid.New()}), http.StatusBadRequest, "INVALID_INPUT")
	f.subRejected(withPayload, subInput(instance, map[string]any{"filter": map[string]any{"op": "eq",
		"field": map[string]any{"namespace": "bogus", "path": []string{"x"}},
		"value": map[string]any{"kind": "string", "value": "x"}}}), http.StatusBadRequest, "INVALID_INPUT")
	f.subRejected(withPayload, subInput(instance, map[string]any{"filter": map[string]any{"op": "eq",
		"field": map[string]any{"namespace": "message", "path": []string{"text"}},
		"value": map[string]any{"kind": "integer", "value": "5"}}}), http.StatusBadRequest, "INVALID_INPUT")
}

// TestSubscriptionRejectsUnavailableOrInvalidTargets proves that a missing, disabled or
// provider-unavailable instance is reported as unavailable, that undeclared or mistyped inputs
// are invalid, and that a stale target revision is a conflict.
func TestSubscriptionRejectsUnavailableOrInvalidTargets(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	disabled, unavailable, healthy := f.instance(false), f.instance(true), f.instance(true)
	f.targets.setUnavailable(unavailable, true)
	for name, instance := range map[string]uuid.UUID{"unknown instance": uuid.New(), "disabled instance": disabled,
		"provider unavailable": unavailable} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := f.subRejected(bearer, subInput(instance, nil), http.StatusServiceUnavailable, string(platform.CodeAuthorityUnavailable))
			if response.Parameters["status"] != "unavailable" || response.Parameters["instanceId"] != instance.String() {
				t.Fatalf("parameters = %v", response.Parameters)
			}
		})
	}
	t.Run("undeclared input", func(t *testing.T) {
		t.Parallel()
		bad := map[string]any{"instanceId": healthy, "inputs": map[string]any{
			"recipient": map[string]any{"kind": "string", "value": "a"},
			"callback":  map[string]any{"kind": "string", "value": "https://evil.example/"}}}
		f.subRejected(bearer, subInput(healthy, map[string]any{"targets": []any{bad}}), http.StatusBadRequest, "INVALID_INPUT")
	})
	t.Run("wrong input kind", func(t *testing.T) {
		t.Parallel()
		bad := map[string]any{"instanceId": healthy, "inputs": map[string]any{
			"recipient": map[string]any{"kind": "integer", "value": "1"}}}
		f.subRejected(bearer, subInput(healthy, map[string]any{"targets": []any{bad}}), http.StatusBadRequest, "INVALID_INPUT")
	})
	t.Run("stale revision", func(t *testing.T) {
		t.Parallel()
		stale := target(healthy)
		stale["revision"] = "7"
		f.subRejected(bearer, subInput(healthy, map[string]any{"targets": []any{stale}}), http.StatusConflict, "REVISION_CONFLICT")
	})
	t.Run("current revision", func(t *testing.T) {
		t.Parallel()
		current := target(healthy)
		current["revision"] = "1"
		created := f.subCreated(bearer, subInput(healthy, map[string]any{"targets": []any{current}}))
		if len(created.Targets) != 1 || created.Targets[0].Revision != 1 {
			t.Fatalf("targets = %+v", created.Targets)
		}
	})
}

// TestSubscriptionWithoutTargetProviderRejectsEveryTarget proves the production default: a Core
// composed without a target provider reports every target as unavailable.
func TestSubscriptionWithoutTargetProviderRejectsEveryTarget(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixtureWithoutProvider(t)
	bearer, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	response := f.subRejected(bearer, subInput(instance, nil), http.StatusServiceUnavailable, string(platform.CodeAuthorityUnavailable))
	if response.Parameters["status"] != "unavailable" {
		t.Fatalf("parameters = %v", response.Parameters)
	}
}

// TestSubscriptionVisibilityFollowsOwnership proves that a personal Subscription is visible
// and changeable only for its owner, with NOT_FOUND for everyone else.
func TestSubscriptionVisibilityFollowsOwnership(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	owner, _ := f.subscriber("payload", "unit")
	stranger, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	created := f.subCreated(owner, subInput(instance, nil))
	path := "/api/v1/subscriptions/" + created.ID.String()

	if got := f.subCall(owner, http.MethodGet, path, nil); got.status != http.StatusOK {
		t.Fatalf("owner read = %d/%s", got.status, got.Code)
	}
	if ids := f.subIDs(owner); !slices.Contains(ids, created.ID) {
		t.Fatalf("owner list = %v", ids)
	}
	if got := f.subCall(stranger, http.MethodGet, path, nil); got.status != http.StatusNotFound || got.Code != "NOT_FOUND" {
		t.Fatalf("stranger read = %d/%s", got.status, got.Code)
	}
	if got := f.subUpdate(stranger, created.ID, 1, subInput(instance, nil)); got.status != http.StatusNotFound {
		t.Fatalf("stranger update = %d/%s", got.status, got.Code)
	}
	if got := f.subDelete(stranger, created.ID, 1); got.status != http.StatusNotFound {
		t.Fatalf("stranger delete = %d/%s", got.status, got.Code)
	}
	if ids := f.subIDs(stranger); slices.Contains(ids, created.ID) {
		t.Fatalf("stranger list shows the Subscription: %v", ids)
	}
	if got := f.subCall(owner, http.MethodGet, path, nil); got.status != http.StatusOK {
		t.Fatalf("Subscription changed by a stranger: %d", got.status)
	}
}

// TestGlobalSubscriptionNeedsManageGlobalAndTechnicalSubscriber proves that a global
// Subscription is created only by a holder of the global action, only for an active technical
// account, and is visible only to such holders.
func TestGlobalSubscriptionNeedsManageGlobalAndTechnicalSubscriber(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	instance := f.instance(true)
	manager, _ := f.caller([]string{"subscription.manage-global"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	personal, _ := f.subscriber("payload", "unit")
	_, technical := f.subscriber("payload", "unit")
	human := uuid.New()
	f.exec(`INSERT INTO identity_accounts (id, kind, state, created_at) VALUES ($1, 'human', 'active', now())`, human)

	global := func(subscriber uuid.UUID) map[string]any {
		return subInput(instance, map[string]any{"kind": "global", "subscriberId": subscriber})
	}
	created := f.subCreated(manager, global(technical))
	if created.Kind != delivery.KindGlobal || created.SubscriberID != technical {
		t.Fatalf("created = %+v", created)
	}
	f.subRejected(manager, global(human), http.StatusBadRequest, "INVALID_INPUT")
	f.subRejected(manager, global(uuid.New()), http.StatusBadRequest, "INVALID_INPUT")
	f.subRejected(personal, global(technical), http.StatusForbidden, "FORBIDDEN")

	if ids := f.subIDs(manager); !slices.Contains(ids, created.ID) {
		t.Fatalf("manager list = %v", ids)
	}
	if ids := f.subIDs(personal); slices.Contains(ids, created.ID) {
		t.Fatalf("a personal manager sees the global Subscription: %v", ids)
	}
	if got := f.subCall(personal, http.MethodGet, "/api/v1/subscriptions/"+created.ID.String(), nil); got.status != http.StatusNotFound {
		t.Fatalf("personal manager read = %d/%s", got.status, got.Code)
	}
	if got := f.subCall(manager, http.MethodGet, "/api/v1/subscriptions/"+created.ID.String(), nil); got.status != http.StatusOK {
		t.Fatalf("manager read = %d/%s", got.status, got.Code)
	}
}

// TestSubscriptionChangesAppendConfigurationFacts proves that create, update, pause and delete
// each append a configuration.changed fact of the subscription aggregate with their change kind.
func TestSubscriptionChangesAppendConfigurationFacts(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	created := f.subCreated(bearer, subInput(instance, nil))
	if got := f.subUpdate(bearer, created.ID, 1, subInput(instance, map[string]any{"maxAgeSeconds": 600})); got.status != http.StatusOK {
		t.Fatalf("update = %d/%s", got.status, got.Code)
	}
	if got := f.subUpdate(bearer, created.ID, 2, subInput(instance, map[string]any{"state": "paused"})); got.status != http.StatusOK {
		t.Fatalf("pause = %d/%s", got.status, got.Code)
	}
	if got := f.subDelete(bearer, created.ID, 3); got.status != http.StatusOK {
		t.Fatalf("delete = %d/%s", got.status, got.Code)
	}
	kinds := f.subStrings(`SELECT envelope->'data'->>'changeKind' || '/' || (envelope->'aggregateRef'->>'type')
		FROM domain_events WHERE envelope->>'type' = 'configuration.changed' AND envelope->'aggregateRef'->>'id' = $1
		ORDER BY (envelope->>'aggregateRevision')::bigint`, created.ID.String())
	want := []string{"created/subscription", "updated/subscription", "paused/subscription", "deleted/subscription"}
	if !slices.Equal(kinds, want) {
		t.Fatalf("facts = %v, want %v", kinds, want)
	}
}

// TestSubscriptionUpdateWithStaleRevisionConflicts proves optimistic concurrency of updates.
func TestSubscriptionUpdateWithStaleRevisionConflicts(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber("payload", "unit")
	instance := f.instance(true)
	created := f.subCreated(bearer, subInput(instance, nil))
	if got := f.subUpdate(bearer, created.ID, 5, subInput(instance, nil)); got.status != http.StatusConflict || got.Code != "REVISION_CONFLICT" {
		t.Fatalf("stale update = %d/%s", got.status, got.Code)
	}
}

// deliveriesOf lists the Deliveries of one subscriber. A caller with delivery.read on every
// account sees all Deliveries, so tests with several subscribers narrow the list.
func (f *deliveryFixture) deliveriesOf(bearer string, subscriber uuid.UUID) []delivery.Delivery {
	f.t.Helper()
	own := []delivery.Delivery{}
	for _, item := range f.deliveries(bearer, "") {
		if item.SubscriberID == subscriber {
			own = append(own, item)
		}
	}
	return own
}

// fanOutAll runs the fan-out consumer until it has consumed every fact in the log. The event
// log only hands out facts of transactions older than every running transaction of the database
// cluster, so a single run can stop early while parallel tests hold a transaction open.
func (f *deliveryFixture) fanOutAll() {
	f.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		f.fanOut()
		pending := f.subStrings(`SELECT count(*)::text FROM domain_events d WHERE NOT EXISTS (
			SELECT 1 FROM consumer_offsets o WHERE o.consumer = $1 AND (d.tx_id, d.seq) <= (o.last_tx_id, o.last_seq))`,
			delivery.FanOutConsumerName)
		if len(pending) == 1 && pending[0] == "0" {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("fan-out left %v facts unconsumed", pending)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
