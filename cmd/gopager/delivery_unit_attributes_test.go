package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/processing"
)

// namedUnit creates a Unit with the name and attributes and returns its ID.
func (f *attributeUnitsFixture) namedUnit(name string, attributes map[string]any) uuid.UUID {
	f.t.Helper()
	body := f.unitBody(f.north, nil, attributes)
	body["name"] = name
	return f.id(f.expect(f.do(http.MethodPost, "/api/v1/units", body), http.StatusCreated))
}

func unitAttributeRef(schema, field string) map[string]any {
	return map[string]any{"namespace": "unit", "path": []string{"attributes", schema, field}}
}

func unitEquals(field, value string) map[string]any {
	return map[string]any{"op": "eq", "field": unitAttributeRef("vehicle", field),
		"value": map[string]any{"kind": "string", "value": value}}
}

func allOf(args ...map[string]any) map[string]any { return map[string]any{"op": "and", "args": args} }

func negated(arg map[string]any) map[string]any {
	return map[string]any{"op": "not", "args": []any{arg}}
}

// splitKeysBySubscription returns the sorted split keys of the Deliveries per Subscription.
func splitKeysBySubscription(listed []delivery.Delivery) map[uuid.UUID][]string {
	keys := map[uuid.UUID][]string{}
	for _, item := range listed {
		keys[item.SubscriptionID] = append(keys[item.SubscriptionID], item.SplitKey)
	}
	for _, list := range keys {
		sort.Strings(list)
	}
	return keys
}

// TestSubscriptionUnitAttributeFilterIsCorrelatedLikeTheFeed proves that a filter on Unit
// attributes needs one Unit to satisfy the whole predicate, in the feed, in an unsplit
// Subscription and in a split one, which has a Delivery only for the matching Unit.
func TestSubscriptionUnitAttributeFilterIsCorrelatedLikeTheFeed(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.schema("vehicle", "unit", []uuid.UUID{f.north},
		map[string]any{"town": map[string]any{"kind": "string"}, "kind": map[string]any{"kind": "string"}})
	x := f.namedUnit("X", map[string]any{"vehicle": map[string]any{"town": valueEntry(text("A")), "kind": valueEntry(text("ladder"))}})
	y := f.namedUnit("Y", map[string]any{"vehicle": map[string]any{"town": valueEntry(text("B")), "kind": valueEntry(text("engine"))}})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	message := f.message(f.minuteAgo(), "FIRE", x, y)

	cases := []struct {
		name      string
		predicate map[string]any
		matches   bool
		splitKeys []string
	}{
		{"town A and engine", allOf(unitEquals("town", "A"), unitEquals("kind", "engine")), false, nil},
		{"town B and engine", allOf(unitEquals("town", "B"), unitEquals("kind", "engine")), true, []string{y.String()}},
		{"not town A", negated(unitEquals("town", "A")), true, []string{y.String()}},
	}
	unsplit, split := map[string]uuid.UUID{}, map[string]uuid.UUID{}
	for _, tc := range cases {
		encoded, _ := json.Marshal(tc.predicate)
		got := f.feed(bearer, "?filter="+url.QueryEscape(string(encoded)), http.StatusOK)
		if feed := len(got.Data.Items) == 1; feed != tc.matches {
			t.Errorf("%s: feed matched = %v, want %v", tc.name, feed, tc.matches)
		}
		unsplit[tc.name] = f.subCreated(bearer, subInput(instance, map[string]any{"filter": tc.predicate})).ID
		split[tc.name] = f.subCreated(bearer, subInput(instance, map[string]any{"filter": tc.predicate, "splitByUnit": true})).ID
	}
	encoded, _ := json.Marshal(unitEquals("kind", "engine"))
	if got := f.feed(bearer, "?unit="+x.String()+"&filter="+url.QueryEscape(string(encoded)), http.StatusOK); len(got.Data.Items) != 0 {
		t.Errorf("engine on Unit X matched %v", itemIDs(got.Data.Items))
	}
	feed := f.feed(bearer, "", http.StatusOK)
	if len(feed.Data.Items) != 1 {
		t.Fatalf("feed = %v", itemIDs(feed.Data.Items))
	}
	units, _ := feed.Data.Items[0]["units"].([]any)
	if len(units) != 2 {
		t.Fatalf("units = %v", units)
	}
	for _, raw := range units {
		unit := raw.(map[string]any)
		attributes, _ := unit["attributes"].([]any)
		if len(attributes) != 2 || attributes[0].(map[string]any)["schemaKey"] != "vehicle" {
			t.Errorf("attributes of %v = %v", unit["name"], attributes)
		}
	}

	f.messageFact("message.created", message, nil)
	f.fanOutAll()
	keys := splitKeysBySubscription(f.deliveries(bearer, ""))
	for _, tc := range cases {
		wantUnsplit := []string{}
		if tc.matches {
			wantUnsplit = []string{""}
		}
		if got := keys[unsplit[tc.name]]; len(got) != len(wantUnsplit) {
			t.Errorf("%s: unsplit split keys = %v, want %v", tc.name, got, wantUnsplit)
		}
		if got := keys[split[tc.name]]; len(got) != len(tc.splitKeys) || (len(got) == 1 && got[0] != tc.splitKeys[0]) {
			t.Errorf("%s: split keys = %v, want %v", tc.name, got, tc.splitKeys)
		}
	}
}

func integerValue(value string) map[string]any {
	return map[string]any{"kind": "integer", "value": value}
}

// TestSubscriptionInputBindingsFillTargetInputsFromUnitAttributes proves the binding precedence
// (constant, binding, Unit parameter), the conflict of an unsplit Delivery, the failure of a Unit
// without a value, the save validation and that a bound schema cannot be deleted.
func TestSubscriptionInputBindingsFillTargetInputsFromUnitAttributes(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.schema("contact", "unit", []uuid.UUID{f.north}, map[string]any{"pager": map[string]any{"kind": "string"},
		"priority": map[string]any{"kind": "integer"}, "urgent": map[string]any{"kind": "boolean"}})
	x := f.namedUnit("X", map[string]any{"contact": map[string]any{"pager": valueEntry(text("111"))}})
	y := f.namedUnit("Y", map[string]any{"contact": map[string]any{"pager": valueEntry(text("222")), "priority": valueEntry(integerValue("3"))}})
	w := f.namedUnit("W", nil)
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.targets.descriptor = func(configuration processing.InstanceConfiguration) delivery.TargetDescriptor {
		return delivery.TargetDescriptor{Version: delivery.DescriptorVersion, InstanceID: configuration.ID,
			InstanceRevision: configuration.Revision, CapabilityKind: delivery.CapabilityDelivery, ConfigurationAuthority: "admin",
			OutputKinds: []string{processing.LayoutText},
			Inputs: []delivery.InputField{{Name: "recipient", Kind: processing.StringValue, Required: true, MaxBytes: 256},
				{Name: "priority", Kind: processing.IntegerValue}},
			Dependencies: []delivery.TargetDependency{{Kind: "network", Description: "controlled test provider"}}}
	}
	encoded, _ := json.Marshal(integerValue("5"))
	f.exec(`INSERT INTO integration_unit_parameters (instance_id, unit_id, key, value) VALUES ($1, $2, 'priority', $3::jsonb)`,
		instance, y, encoded)
	bound := func(recipientField string) map[string]any {
		return map[string]any{"instanceId": instance, "inputs": map[string]any{"priority": integerValue("1")},
			"inputBindings": map[string]any{"recipient": unitAttributeRef("contact", recipientField),
				"priority": unitAttributeRef("contact", "priority")}}
	}

	f.subRejected(bearer, subInput(instance, map[string]any{"targets": []any{bound("urgent")}}), http.StatusBadRequest, "INVALID_INPUT")
	f.subRejected(bearer, subInput(instance, map[string]any{"targets": []any{map[string]any{"instanceId": instance,
		"inputBindings": map[string]any{"missing": unitAttributeRef("contact", "pager")}}}}), http.StatusBadRequest, "INVALID_INPUT")
	unsplit := f.subCreated(bearer, subInput(instance, map[string]any{"targets": []any{bound("pager")}}))
	split := f.subCreated(bearer, subInput(instance, map[string]any{"targets": []any{bound("pager")}, "splitByUnit": true}))
	if len(split.Targets[0].InputBindings) != 2 {
		t.Fatalf("saved bindings = %+v", split.Targets[0].InputBindings)
	}

	f.messageFact("message.created", f.message(f.minuteAgo(), "BOTH", x, y), nil)
	f.fanOutAll()
	f.attempt()
	listed := f.deliveries(bearer, "")
	splitOf, recipients, priorities := map[uuid.UUID]string{}, map[string]string{}, map[string]string{}
	for _, item := range listed {
		splitOf[item.ID] = item.SplitKey
		if item.SubscriptionID == unsplit.ID && (item.State != delivery.Failed || item.OutcomeCode == nil || *item.OutcomeCode != delivery.CodeParameterConflict) {
			t.Errorf("unsplit Delivery = %+v, want failed parameter-conflict", item)
		}
	}
	for _, call := range f.targets.calls() {
		var recipient, priority string
		_ = json.Unmarshal(call.Input.Inputs["recipient"].Value, &recipient)
		_ = json.Unmarshal(call.Input.Inputs["priority"].Value, &priority)
		recipients[splitOf[call.DeliveryID]], priorities[splitOf[call.DeliveryID]] = recipient, priority
	}
	if len(recipients) != 2 || recipients[x.String()] != "111" || recipients[y.String()] != "222" {
		t.Errorf("recipients = %v", recipients)
	}
	if priorities[x.String()] != "1" || priorities[y.String()] != "5" {
		t.Errorf("priorities = %v, want the constant for X and the Unit parameter for Y", priorities)
	}

	f.messageFact("message.created", f.message(f.minuteAgo(), "NO PAGER", w), nil)
	f.fanOutAll()
	found := false
	for _, item := range f.deliveries(bearer, "?subscriptionId="+split.ID.String()) {
		found = found || item.SplitKey == w.String()
		if item.SplitKey == w.String() && (item.State != delivery.Failed || item.OutcomeCode == nil || *item.OutcomeCode != delivery.CodeInputInvalid) {
			t.Errorf("Delivery of W = %+v, want failed input-invalid", item)
		}
	}
	if !found {
		t.Error("no Delivery for W")
	}
	if got := f.deleteSchema("contact", "1"); got.status != http.StatusConflict || got.Code != "REVISION_CONFLICT" {
		t.Errorf("deleting a bound schema = %d/%s, want 409 REVISION_CONFLICT", got.status, got.Code)
	}
}
