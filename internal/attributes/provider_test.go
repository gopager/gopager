package attributes

import (
	"testing"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

func providerSchemas() []Schema {
	return []Schema{
		{Key: "vehicle", Subject: SubjectMessage, Fields: map[string]Field{
			"plate": {Kind: processing.StringValue, Label: "Plate", Description: "Registration", Icon: "car"},
			"count": {Kind: processing.IntegerValue},
			"tags":  {Kind: processing.ListValue, Element: &Field{Kind: processing.StringValue}},
			"blobs": {Kind: processing.ListValue, Element: &Field{Kind: processing.BytesValue}},
			"owner": {Kind: processing.ObjectValue, Label: "Owner", Properties: map[string]Field{
				"name": {Kind: processing.StringValue, Label: "Full name"},
				"address": {Kind: processing.ObjectValue, Deprecated: true, Properties: map[string]Field{
					"city": {Kind: processing.StringValue},
				}},
				"items": {Kind: processing.ListValue, Element: &Field{Kind: processing.ObjectValue,
					Properties: map[string]Field{"hidden": {Kind: processing.StringValue}}}},
			}},
		}},
		{Key: "legacy", Subject: SubjectMessage, Deprecated: true, Fields: map[string]Field{
			"code": {Kind: processing.StringValue},
		}},
		{Key: "unitdata", Subject: SubjectUnit, Fields: map[string]Field{
			"badge": {Kind: processing.StringValue, Label: "Badge", Description: "Station badge", Icon: "tag"},
			"tags":  {Kind: processing.ListValue, Element: &Field{Kind: processing.StringValue}},
			"site": {Kind: processing.ObjectValue, Deprecated: true, Properties: map[string]Field{
				"town": {Kind: processing.StringValue},
				"items": {Kind: processing.ListValue, Element: &Field{Kind: processing.ObjectValue,
					Properties: map[string]Field{"hidden": {Kind: processing.StringValue}}}},
			}},
		}},
	}
}

func ref(path ...string) processing.FieldRef {
	return processing.FieldRef{Namespace: "attributes", Path: path}
}

func TestProviderFields(t *testing.T) {
	t.Parallel()
	provider := NewProvider(providerSchemas(), 7)
	if provider.Revision() != 7 {
		t.Fatalf("revision = %d", provider.Revision())
	}
	tests := []struct {
		path       []string
		kind       processing.ValueKind
		label      string
		filterable bool
		deprecated bool
	}{
		{[]string{"vehicle", "plate"}, processing.StringValue, "Plate", true, false},
		{[]string{"vehicle", "count"}, processing.IntegerValue, "count", true, false},
		{[]string{"vehicle", "tags"}, processing.ListValue, "tags", true, false},
		{[]string{"vehicle", "blobs"}, processing.ListValue, "blobs", false, false},
		{[]string{"vehicle", "owner"}, processing.ObjectValue, "Owner", false, false},
		{[]string{"vehicle", "owner", "name"}, processing.StringValue, "Full name", true, false},
		{[]string{"vehicle", "owner", "address"}, processing.ObjectValue, "address", false, true},
		{[]string{"vehicle", "owner", "address", "city"}, processing.StringValue, "city", true, true},
		{[]string{"legacy", "code"}, processing.StringValue, "code", true, true},
	}
	for _, test := range tests {
		field, ok := provider.Field(ref(test.path...))
		if !ok {
			t.Fatalf("%v missing", test.path)
		}
		if field.Type.Kind != test.kind || !field.Type.Optional || field.Label != test.label ||
			field.Filterable != test.filterable || field.Deprecated != test.deprecated || field.Group != "derived" ||
			!field.In(fields.ScopeMessage) || !field.In(fields.ScopeDelivery) || field.In(fields.ScopeWorkflow) {
			t.Fatalf("%v: unexpected %+v", test.path, field)
		}
	}
	plate, _ := provider.Field(ref("vehicle", "plate"))
	if plate.Description != "Registration" || plate.Icon != "car" || plate.Path() != "attributes.vehicle.plate" {
		t.Fatalf("metadata = %+v", plate)
	}
}

func TestProviderDoesNotDescendIntoLists(t *testing.T) {
	t.Parallel()
	provider := NewProvider(providerSchemas(), 1)
	for _, path := range [][]string{
		{"vehicle", "owner", "items", "hidden"},
		{"vehicle", "tags", "x"},
		{"vehicle", "missing"},
	} {
		if _, ok := provider.Field(ref(path...)); ok {
			t.Fatalf("%v must not be declared", path)
		}
	}
	if got := len(provider.Fields()); got != 20 {
		t.Fatalf("fields = %d, want 20", got)
	}
}

func TestProviderInRegistry(t *testing.T) {
	t.Parallel()
	registry := fields.New(NewProvider(providerSchemas(), 2), fields.Builtin())
	if _, err := registry.Lookup(fields.ScopeMessage, ref("vehicle", "owner", "address", "city"), true); err == nil {
		t.Fatal("strict lookup must reject an inherited deprecation")
	}
	if _, err := registry.Lookup(fields.ScopeMessage, ref("vehicle", "owner", "address", "city"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Lookup(fields.ScopeWorkflow, ref("vehicle", "plate"), false); err == nil {
		t.Fatal("attributes are not Workflow fields")
	}
	if registry.Revision() != "2.0" {
		t.Fatalf("revision = %q", registry.Revision())
	}
	for _, field := range registry.List(fields.ScopeMessage) {
		if field.Deprecated {
			t.Fatalf("%s is deprecated", field.Path())
		}
	}
}

func TestProviderUnitFields(t *testing.T) {
	t.Parallel()
	provider := NewProvider(providerSchemas(), 1)
	tests := []struct {
		path       []string
		kind       processing.ValueKind
		label      string
		deprecated bool
	}{
		{[]string{"unitdata", "badge"}, processing.StringValue, "Badge", false},
		{[]string{"unitdata", "tags"}, processing.ListValue, "tags", false},
		{[]string{"unitdata", "site"}, processing.ObjectValue, "site", true},
		{[]string{"unitdata", "site", "town"}, processing.StringValue, "town", true},
	}
	for _, test := range tests {
		for _, shape := range []struct {
			namespace string
			prefix    []string
			scope     fields.Scope
		}{
			{"unit", []string{"attributes"}, fields.ScopeMessage},
			{"unit", []string{"attributes"}, fields.ScopeDelivery},
			{"target", []string{"unit", "attributes"}, fields.ScopeWorkflow},
		} {
			path := append(append([]string{}, shape.prefix...), test.path...)
			field, ok := provider.Field(processing.FieldRef{Namespace: shape.namespace, Path: path})
			if !ok {
				t.Fatalf("%s %v missing", shape.namespace, path)
			}
			if field.Type.Kind != test.kind || !field.Type.Optional || field.Label != test.label ||
				field.Deprecated != test.deprecated || field.Group != "unit" || !field.In(shape.scope) ||
				field.Filterable != fields.FilterableType(field.Type) {
				t.Fatalf("%s %v: unexpected %+v", shape.namespace, path, field)
			}
		}
	}
	badge, _ := provider.Field(processing.FieldRef{Namespace: "unit", Path: []string{"attributes", "unitdata", "badge"}})
	if badge.Description != "Station badge" || badge.Icon != "tag" || badge.Path() != "unit.attributes.unitdata.badge" {
		t.Fatalf("metadata = %+v", badge)
	}
	if badge.In(fields.ScopeWorkflow) {
		t.Fatal("unit.attributes is not a Workflow field")
	}
	if _, ok := provider.Field(processing.FieldRef{Namespace: "unit", Path: []string{"attributes", "unitdata", "site", "items", "hidden"}}); ok {
		t.Fatal("list elements must not be declared")
	}
	if _, ok := provider.Field(ref("unitdata", "badge")); ok {
		t.Fatal("a unit schema declares no Message attribute")
	}
	// 10 Message fields, 5 Unit fields (badge, tags, site, site.town, site.items) in two namespaces.
	if got := len(provider.Fields()); got != 20 {
		t.Fatalf("fields = %d, want 20", got)
	}
}

func TestProviderUnitFieldsInRegistry(t *testing.T) {
	t.Parallel()
	registry := fields.New(NewProvider(providerSchemas(), 2), fields.Builtin())
	badge := processing.FieldRef{Namespace: "unit", Path: []string{"attributes", "unitdata", "badge"}}
	target := processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "unitdata", "badge"}}
	if _, err := registry.Lookup(fields.ScopeMessage, badge, true); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Lookup(fields.ScopeWorkflow, target, true); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Lookup(fields.ScopeMessage, target, false); err == nil {
		t.Fatal("target.unit.attributes is a Workflow field only")
	}
}
