package declarations_test

import (
	"encoding/json"
	"testing"

	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/declarations"
	"github.com/gopager/gopager/internal/units"
)

func unitRegistry() *fields.Registry {
	schemas := []attributes.Schema{{Key: "station", Subject: attributes.SubjectUnit, Fields: map[string]attributes.Field{
		"town": {Kind: processing.StringValue},
		"tags": {Kind: processing.ListValue, Element: &attributes.Field{Kind: processing.StringValue}},
		"site": {Kind: processing.ObjectValue, Properties: map[string]attributes.Field{
			"floor": {Kind: processing.IntegerValue},
		}},
	}}}
	return fields.New(fields.Builtin(), attributes.NewProvider(schemas, 1))
}

// declareAll declares every Unit attribute path the registry offers, which is what a Workflow
// reading all of them would need.
func declareAll(t *testing.T, registry *fields.Registry) []processing.FieldDeclaration {
	t.Helper()
	var refs []processing.FieldRef
	for _, field := range registry.ListAll(fields.ScopeWorkflow) {
		if len(field.Ref.Path) >= 4 && field.Ref.Path[0] == "unit" && field.Ref.Path[1] == "attributes" {
			refs = append(refs, field.Ref)
		}
	}
	declared, err := declarations.UnitAttributeDeclarations(registry, refs)
	if err != nil {
		t.Fatal(err)
	}
	return declared
}

func typedValue(t *testing.T, kind processing.ValueKind, value any) processing.Value {
	t.Helper()
	typed, err := processing.Literal(kind, value)
	if err != nil {
		t.Fatal(err)
	}
	return typed
}

func effective(value processing.Value, field string) units.EffectiveAttributes {
	return units.EffectiveAttributes{"station": {field: {Value: &value, Source: units.SourceUnit}}}
}

func TestUnitAttributeDeclarationsCoverFieldsAndProperties(t *testing.T) {
	t.Parallel()
	declared := declareAll(t, unitRegistry())
	got := map[string]bool{}
	for _, declaration := range declared {
		if declaration.Reference.Namespace != "target" || !declaration.Schema.Optional ||
			len(declaration.Groups) != 1 || declaration.Groups[0] != "unit" {
			t.Fatalf("declaration=%+v", declaration)
		}
		got[declaration.Reference.Path[2]+"."+declaration.Reference.Path[3]] = true
	}
	for _, want := range []string{"station.town", "station.tags", "station.site"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if len(declareAll(t, fields.New(fields.Builtin()))) != 0 {
		t.Fatal("builtin registry declares Unit attributes")
	}
}

// TestUnitAttributeDeclarationsCoverOnlyTheReadPaths shows that a catalog with many Unit fields
// declares just the references a Workflow reads, and none for a Workflow reading none.
func TestUnitAttributeDeclarationsCoverOnlyTheReadPaths(t *testing.T) {
	t.Parallel()
	registry := unitRegistry()
	town := processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", "town"}}
	declared, err := declarations.UnitAttributeDeclarations(registry, []processing.FieldRef{town})
	if err != nil {
		t.Fatal(err)
	}
	if len(declared) != 1 || declared[0].Reference.Path[3] != "town" || !declared[0].Schema.Optional ||
		declared[0].Schema.Kind != processing.StringValue {
		t.Fatalf("declared=%+v", declared)
	}
	if none, err := declarations.UnitAttributeDeclarations(registry, nil); err != nil || len(none) != 0 {
		t.Fatalf("declared=%+v err=%v for a Workflow reading no Unit attribute", none, err)
	}
	unknown := processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", "nope"}}
	if _, err := declarations.UnitAttributeDeclarations(registry, []processing.FieldRef{unknown}); err == nil {
		t.Fatal("unknown reference was declared")
	}
}

// TestDefinitionUnitAttributeRefsFindEveryRead expects every Unit-attribute reference of a
// definition, wherever it appears, once each, and no other field.
func TestDefinitionUnitAttributeRefsFindEveryRead(t *testing.T) {
	t.Parallel()
	town := processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", "town"}}
	floor := processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", "site", "floor"}}
	unitID := processing.FieldRef{Namespace: "target", Path: []string{"unit", "id"}}
	empty, err := processing.Literal(processing.StringValue, "")
	if err != nil {
		t.Fatal(err)
	}
	definition := processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "and", Args: []processing.Predicate{
			{Op: "eq", Field: &town, Value: &empty}, {Op: "present", Field: &unitID}}},
		Outputs: map[string]map[string]processing.Assignment{"routing": {"town": {Field: &town}, "floor": {Field: &floor}}}}
	refs, err := declarations.DefinitionUnitAttributeRefs(definition)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, ref := range refs {
		got[ref.String()] = true
	}
	if len(refs) != 2 || !got[town.String()] || !got[floor.String()] {
		t.Fatalf("refs=%v, want town and site.floor once each", refs)
	}
}

func TestUnitAttributeFieldsDescendAndAbsent(t *testing.T) {
	t.Parallel()
	declared := declareAll(t, unitRegistry())
	site := typedValue(t, processing.ObjectValue, map[string]processing.Value{
		"floor": typedValue(t, processing.IntegerValue, "3")})
	tags := typedValue(t, processing.ListValue, []processing.Value{typedValue(t, processing.StringValue, "a")})
	attrs := units.EffectiveAttributes{"station": {
		"town": {Value: ptr(typedValue(t, processing.StringValue, "A")), Source: units.SourceUnit},
		"site": {Value: &site, Source: units.SourceOrganization},
		"tags": {Value: &tags, Source: units.SourceUnit},
	}}
	dependencies := []processing.Dependency{{Groups: []string{"unit"}}}
	captured, err := declarations.UnitAttributeFields(declared, attrs, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != len(declared) {
		t.Fatalf("captured %d fields for %d declarations", len(captured), len(declared))
	}
	for _, field := range captured {
		path := field.Reference.Path
		var want string
		switch len(path) {
		case 4:
			want = map[string]string{"town": `"A"`, "site": string(site.Value), "tags": string(tags.Value)}[path[3]]
		case 5:
			want = `"3"`
		}
		if field.Field.State != processing.FieldPresent || field.Field.Value == nil ||
			string(field.Field.Value.Value) != want || len(field.Field.Dependencies) != 1 {
			t.Errorf("%v=%+v want %s", path, field.Field, want)
		}
	}

	for name, absent := range map[string]units.EffectiveAttributes{
		"no Unit": nil, "no value": {"station": {}},
		"missing property": effective(typedValue(t, processing.ObjectValue, json.RawMessage(`{}`)), "site"),
	} {
		captured, err := declarations.UnitAttributeFields(declared, absent, dependencies)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range captured {
			if name == "missing property" && len(field.Reference.Path) != 5 {
				continue
			}
			if field.Field.State != processing.FieldAbsent || field.Field.Value != nil {
				t.Errorf("%s: %v=%+v", name, field.Reference.Path, field.Field)
			}
		}
	}
}

func ptr[T any](value T) *T { return &value }

// TestUnitAttributeDeclarationsKeepDeprecatedFields covers a Workflow version activated before a
// field was deprecated: its runs must still capture the field, or deprecation would stop them.
func TestUnitAttributeDeclarationsKeepDeprecatedFields(t *testing.T) {
	t.Parallel()
	schemas := []attributes.Schema{{Key: "station", Subject: attributes.SubjectUnit, Fields: map[string]attributes.Field{
		"town": {Kind: processing.StringValue, Deprecated: true},
	}}}
	declared := declareAll(t, fields.New(fields.Builtin(), attributes.NewProvider(schemas, 1)))
	if len(declared) != 1 || declared[0].Reference.Path[3] != "town" {
		t.Fatalf("declared=%+v, want the deprecated town field", declared)
	}
}
