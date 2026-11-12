package main

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// TestSubscriptionProcessingCompletedSelectionDependsOnPublishedAttribute proves that a
// Subscription on processing.completed with an attribute filter delivers only facts whose
// published result is visible and whose effective attribute satisfies the filter, and that
// handling the fact twice creates one Delivery.
func TestSubscriptionProcessingCompletedSelectionDependsOnPublishedAttribute(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	workflow := f.resultWorkflow()
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed"},
		"filter": levelAtLeast("2")}))
	unit := f.unit("Engine 1", nil)
	high, low := f.message(f.minuteAgo(), "HIGH", unit), f.message(f.minuteAgo(), "LOW", unit)
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

// TestSubscriptionProcessingCompletedWithoutVisibleResultCreatesNothing proves that a
// processing.completed fact whose result is not part of the subscriber's projection creates no
// Delivery.
func TestSubscriptionProcessingCompletedWithoutVisibleResultCreatesNothing(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed"}}))
	message := f.message(f.minuteAgo(), "NO RESULT", f.unit("Engine 1", nil))
	f.messageFact("processing.completed", message, map[string]any{"resultId": uuid.New()})

	f.fanOutAll()
	if listed := f.deliveries(bearer, ""); len(listed) != 0 {
		t.Fatalf("a fact without a visible result created %+v", listed)
	}
}

// TestSubscriptionProcessingCompletedDoesNotDependOnEffectiveValues proves that an unfiltered
// Subscription receives processing.completed for a readable published result also when that
// result has no present outputs or when a later result of another Workflow superseded its
// values: the result is visible to the subscriber even though it contributes no effective value.
func TestSubscriptionProcessingCompletedDoesNotDependOnEffectiveValues(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed"}}))
	unit := f.unit("Engine 1", nil)

	empty := f.message(f.minuteAgo(), "EMPTY", unit)
	emptyResult := f.publishResult(f.resultWorkflow(), empty, `{"selected":true,"fields":{}}`, `["derived","payload"]`)
	emptyFact := f.messageFact("processing.completed", empty, map[string]any{"resultId": emptyResult})

	superseded := f.message(f.minuteAgo(), "SUPERSEDED", unit)
	earlier := f.publishLevel(f.resultWorkflow(), superseded, "1")
	f.publishLevel(f.resultWorkflow(), superseded, "5")
	supersededFact := f.messageFact("processing.completed", superseded, map[string]any{"resultId": earlier})

	f.fanOutAll()
	got := map[uuid.UUID]bool{}
	for _, item := range f.deliveries(bearer, "") {
		got[item.DomainEventID] = true
	}
	if len(got) != 2 || !got[emptyFact] || !got[supersededFact] {
		t.Fatalf("Delivered facts = %v, want the empty-output and the superseded result", got)
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
	f.attributeSchema("incident", incidentFields)
	workflow := f.resultWorkflow()
	f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed", "message.created"}}))
	message := f.message(f.minuteAgo(), "HIDDEN RESULT", f.unit("Engine 2", nil))
	result := f.publishResult(workflow, message, untargetedResult(attributeEntry("level", "integer", `"9"`)),
		`["derived","payload","quality"]`)
	f.messageFact("processing.completed", message, map[string]any{"resultId": result})
	created := f.messageFact("message.created", message, nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].DomainEventID != created {
		t.Fatalf("Deliveries = %+v, want only the message.created one", listed)
	}
	f.attempt()
	for _, call := range f.targets.calls() {
		if strings.Contains(call.Input.Body, `"9"`) {
			t.Fatalf("an unreadable result reached the input: %q", call.Input.Body)
		}
	}
}

// TestSubscriptionFilterReadsTargetValuesOfTheRelevantUnits proves the candidates of an attribute
// filter: an unsplit Delivery matches when any target value of its Units does, a split Delivery
// only sees the target values of its own Unit, and an untargeted value counts for every split.
func TestSubscriptionFilterReadsTargetValuesOfTheRelevantUnits(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	workflow := f.resultWorkflow()
	alpha, bravo := f.unit("Alpha", nil), f.unit("Bravo", nil)
	message := f.message(f.minuteAgo(), "TARGETS", alpha, bravo)
	f.publishResult(workflow, message, targetedResult(0, attributeEntry("city", "string", `"Aachen"`)), `["derived","payload"]`)
	f.publishResult(f.resultWorkflow(), message, targetedResult(1, attributeEntry("city", "string", `"Bonn"`)), `["derived","payload"]`)

	unsplit := f.subCreated(bearer, subInput(instance, map[string]any{"filter": cityIs("Bonn")}))
	split := f.subCreated(bearer, subInput(instance, map[string]any{"filter": cityIs("Bonn"), "splitByUnit": true}))
	nowhere := f.subCreated(bearer, subInput(instance, map[string]any{"filter": cityIs("Cologne"), "splitByUnit": true}))
	f.messageFact("message.created", message, nil)

	f.fanOutAll()
	counts := map[uuid.UUID][]string{}
	for _, item := range f.deliveries(bearer, "") {
		counts[item.SubscriptionID] = append(counts[item.SubscriptionID], item.SplitKey)
	}
	if got := counts[unsplit.ID]; len(got) != 1 || got[0] != "" {
		t.Errorf("unsplit Subscription split keys = %v, want the one unsplit Delivery", got)
	}
	if got := counts[split.ID]; len(got) != 1 || got[0] != bravo.String() {
		t.Errorf("split Subscription split keys = %v, want only Bravo's", got)
	}
	if got := counts[nowhere.ID]; len(got) != 0 {
		t.Errorf("a value nobody has matched: %v", got)
	}
}

// TestSubscriptionFilterUntargetedAttributeCountsForEverySplit proves that an untargeted effective
// value, also below an object property, is a candidate of every split.
func TestSubscriptionFilterUntargetedAttributeCountsForEverySplit(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	workflow := f.resultWorkflow()
	alpha, bravo := f.unit("Alpha", nil), f.unit("Bravo", nil)
	message := f.message(f.minuteAgo(), "OWNER", alpha, bravo)
	f.publishResult(workflow, message, untargetedResult(
		`"owner":{"state":"present","value":{"kind":"object","value":{"name":{"kind":"string","value":"Ada"}}}}`),
		`["derived","payload"]`)
	ownerAda := map[string]any{"op": "eq", "field": attributeRef("owner", "name"),
		"value": map[string]any{"kind": "string", "value": "Ada"}}
	created := f.subCreated(bearer, subInput(instance, map[string]any{"filter": ownerAda, "splitByUnit": true}))
	f.messageFact("message.created", message, nil)

	f.fanOutAll()
	if listed := f.deliveries(bearer, "?subscriptionId="+created.ID.String()); len(listed) != 2 {
		t.Fatalf("Deliveries = %+v, want one per Unit", listed)
	}
}

// TestSubscriptionFilterMatchesAnyElementOfAListField proves that a filter on a list field holds
// when one element satisfies it.
func TestSubscriptionFilterMatchesAnyElementOfAListField(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	alpha, bravo := f.unit("Alpha", nil), f.unit("Bravo", nil)
	unitIs := func(id uuid.UUID) map[string]any {
		return map[string]any{"op": "eq", "field": map[string]any{"namespace": "message", "path": []string{"unitIds"}},
			"value": map[string]any{"kind": "string", "value": id.String()}}
	}
	hit := f.subCreated(bearer, subInput(instance, map[string]any{"filter": unitIs(bravo)}))
	miss := f.subCreated(bearer, subInput(instance, map[string]any{"filter": unitIs(uuid.New())}))
	f.messageFact("message.created", f.message(f.minuteAgo(), "LIST", alpha, bravo), nil)

	f.fanOutAll()
	listed := f.deliveries(bearer, "")
	if len(listed) != 1 || listed[0].SubscriptionID != hit.ID || listed[0].SubscriptionID == miss.ID {
		t.Fatalf("Deliveries = %+v, want only the Subscription naming Bravo", listed)
	}
}

// TestLayoutReadsTheAttributeOfTheDeliverysUnit proves the single-value rule of a Layout: a split
// Delivery renders the target value of its own Unit.
func TestLayoutReadsTheAttributeOfTheDeliverysUnit(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	workflow := f.resultWorkflow()
	alpha, bravo := f.unit("Alpha", nil), f.unit("Bravo", nil)
	message := f.message(f.minuteAgo(), "RENDER", alpha, bravo)
	f.publishResult(workflow, message, targetedResult(0, attributeEntry("city", "string", `"Aachen"`)), `["derived","payload"]`)
	f.publishResult(f.resultWorkflow(), message, targetedResult(1, attributeEntry("city", "string", `"Bonn"`)), `["derived","payload"]`)
	layout := f.layoutCreated(f.layoutManager(), layoutBody("text", []any{
		map[string]any{"kind": "field", "field": attributeRef("city"), "label": "City"}}))
	f.subCreated(bearer, subInput(instance, map[string]any{"layoutId": layout.ID, "splitByUnit": true}))
	f.messageFact("message.created", message, nil)

	f.fanOutAll()
	f.attempt()
	bodies := map[string]string{}
	for _, call := range f.targets.calls() {
		bodies[call.Input.Body] = call.Input.OutputKind
	}
	if len(bodies) != 2 {
		t.Fatalf("bodies = %v, want one per Unit", bodies)
	}
	var texts []string
	for body := range bodies {
		texts = append(texts, body)
	}
	joined := strings.Join(texts, "|")
	if !strings.Contains(joined, "Aachen") || !strings.Contains(joined, "Bonn") {
		t.Fatalf("bodies = %q, want each Unit's own city", joined)
	}
}
