package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
)

// renderedLevelItem returns the single rendered item of the JSON body of the only Delivery
// attempt that has not been inspected yet.
func renderedLevelItem(t *testing.T, f *deliveryFixture, attempts int) (label, icon string) {
	t.Helper()
	calls := f.targets.calls()
	if len(calls) != attempts {
		t.Fatalf("invocations = %d, want %d", len(calls), attempts)
	}
	var items []struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
		Icon  string `json:"icon"`
	}
	if err := json.Unmarshal([]byte(calls[attempts-1].Input.Body), &items); err != nil || len(items) != 1 || items[0].Kind != "field" {
		t.Fatalf("body = %q (%v), want one field item", calls[attempts-1].Input.Body, err)
	}
	return items[0].Label, items[0].Icon
}

// TestLayoutFieldWithoutLabelFollowsTheSchemaFieldPresentation proves that a field block without
// a label renders the label and icon declared by the Attribute schema, and that a label changed
// through the schema service shows in the next rendering although the Layout version is the same.
func TestLayoutFieldWithoutLabelFollowsTheSchemaFieldPresentation(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	schemaManager, _ := f.caller([]string{"attribute-schema.manage"}, authz.Scope{Kind: "all"}, []string{"payload"})
	content := func(label string) map[string]any {
		return map[string]any{"schemaVersion": 1, "label": "Incident", "subject": "message", "agencies": []uuid.UUID{f.agency},
			"fields": map[string]any{"level": map[string]any{"kind": "integer", "label": label, "icon": "alarm-bell"}}}
	}
	create := content("Level")
	create["commandId"] = uuid.New()
	create["key"] = "incident"
	if got := f.subCall(schemaManager, http.MethodPost, "/api/v1/attribute-schemas", create); got.status != http.StatusCreated {
		t.Fatalf("schema create = %d/%s", got.status, got.Code)
	}
	workflow := f.resultWorkflow()
	layout := f.layoutCreated(f.layoutManager(), layoutBody("json", []any{
		map[string]any{"kind": "field", "field": attributeRef("level")}}))
	f.subCreated(bearer, subInput(instance, map[string]any{"layoutId": layout.ID}))

	first := f.message(f.minuteAgo(), "FIRST", f.unit("Engine 1", nil))
	f.publishLevel(workflow, first, "3")
	f.messageFact("message.created", first, nil)
	f.fanOutAll()
	f.attempt()
	if label, icon := renderedLevelItem(t, f, 1); label != "Level" || icon != "alarm-bell" {
		t.Fatalf("first rendering label/icon = %q/%q, want the declared Level/alarm-bell", label, icon)
	}

	update := content("Severity")
	update["expectedRevision"] = "1"
	if got := f.subCall(schemaManager, http.MethodPut, "/api/v1/attribute-schemas/incident", update); got.status != http.StatusOK {
		t.Fatalf("schema update = %d/%s", got.status, got.Code)
	}
	second := f.message(f.minuteAgo(), "SECOND", f.unit("Engine 2", nil))
	f.publishLevel(workflow, second, "4")
	f.messageFact("message.created", second, nil)
	f.fanOutAll()
	f.attempt()
	if label, icon := renderedLevelItem(t, f, 2); label != "Severity" || icon != "alarm-bell" {
		t.Fatalf("second rendering label/icon = %q/%q, want Severity/alarm-bell", label, icon)
	}
	for _, item := range f.deliveries(bearer, "") {
		if item.LayoutVersion != 1 {
			t.Fatalf("Delivery %s rendered layout version %d, want the unchanged version 1", item.ID, item.LayoutVersion)
		}
	}
}
