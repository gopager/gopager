package fields

import (
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

func TestBuiltinWorkflowCatalog(t *testing.T) {
	t.Parallel()
	registry := New(Builtin())
	declarations := registry.Declarations(ScopeWorkflow, true)
	tests := []struct {
		ref        processing.FieldRef
		kind       processing.ValueKind
		optional   bool
		nullable   bool
		group      string
		filterable bool
	}{
		{ref("source", "protocol"), processing.StringValue, false, false, "payload", true},
		{ref("source", "subtype"), processing.StringValue, true, false, "payload", true},
		{ref("source", "observedAt"), processing.TimestampValue, false, false, "gateway", true},
		{ref("source", "network", "name"), processing.StringValue, false, false, "gateway", true},
		{ref("source", "payload", "binary"), processing.BytesValue, true, false, "payload", false},
		{ref("source", "payload", "flex", "shortSignal", "sourceCode"), processing.IntegerValue, true, false, "payload", true},
		{ref("source", "payload", "zvei", "double_tone"), processing.StringValue, true, true, "payload", true},
		{ref("target", "address", "number"), processing.IntegerValue, true, false, "address", true},
		{ref("target", "function"), processing.IntegerValue, true, false, "address", true},
		{ref("target", "unit", "collections"), processing.ListValue, true, false, "unit", true},
	}
	for _, test := range tests {
		t.Run(refPath(test.ref), func(t *testing.T) {
			t.Parallel()
			field, err := registry.Lookup(ScopeWorkflow, test.ref, true)
			if err != nil {
				t.Fatal(err)
			}
			if field.Type.Kind != test.kind || field.Type.Optional != test.optional || field.Type.Nullable != test.nullable ||
				field.Group != test.group || field.Filterable != test.filterable || field.Label == "" {
				t.Fatalf("unexpected field %+v", field)
			}
			if schema, err := declarations.FieldSchema(test.ref); err != nil || schema.Kind != test.kind {
				t.Fatalf("FieldSchema = %+v, %v", schema, err)
			}
		})
	}
	if got := len(registry.List(ScopeWorkflow)); got != 20 {
		t.Fatalf("workflow fields = %d, want 20", got)
	}
}

func TestBuiltinMessageAndDeliveryCatalog(t *testing.T) {
	t.Parallel()
	registry := New(Builtin())
	for _, path := range []string{"id", "time", "processingStatus", "correctionStatus", "protocol", "form", "text", "numeric", "unitIds", "unitNames"} {
		for _, scope := range []Scope{ScopeMessage, ScopeDelivery} {
			field, err := registry.Lookup(scope, ref("message", path), true)
			if err != nil || !field.Type.Optional || !field.Filterable {
				t.Fatalf("message.%s in %s: %+v, %v", path, scope, field, err)
			}
		}
	}
	groups := map[string]string{
		"message.id": "", "message.correctionStatus": "", "message.text": "payload", "message.unitNames": "unit",
		"event.type": "", "event.reason": "", "unit.id": "unit", "gateway.contactTime": "gateway",
	}
	for path, group := range groups {
		for _, field := range registry.List(ScopeDelivery) {
			if field.Path() == path && field.Group != group {
				t.Fatalf("%s group = %q, want %q", path, field.Group, group)
			}
		}
	}
	for _, path := range []string{"type", "occurredAt", "lateJoin", "reason"} {
		if _, err := registry.Lookup(ScopeMessage, ref("event", path), false); err == nil {
			t.Fatalf("event.%s must not be a message field", path)
		}
		if _, err := registry.Lookup(ScopeDelivery, ref("event", path), false); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(registry.List(ScopeMessage)); got != 10 {
		t.Fatalf("message fields = %d, want 10", got)
	}
	if got := len(registry.List(ScopeDelivery)); got != 21 {
		t.Fatalf("delivery fields = %d, want 21", got)
	}
	if Builtin().Revision() != 0 {
		t.Fatal("built-in revision must be 0")
	}
}
