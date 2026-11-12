package units

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/processing"
)

func typed(t *testing.T, kind processing.ValueKind, raw string) *processing.Value {
	t.Helper()
	return &processing.Value{Kind: kind, Value: json.RawMessage(raw)}
}

var (
	countyA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	countyB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
)

// stationSchemas has the schema "station" (Agency A) with a string and an object field, and
// "radio" (Agency B) with an integer field.
func stationSchemas() AttributeSchemas {
	return AttributeSchemas{
		"station": {Agencies: []uuid.UUID{countyA}, Fields: map[string]processing.FieldSchema{
			"town": {Kind: processing.StringValue},
			"site": {Kind: processing.ObjectValue, Properties: map[string]processing.FieldSchema{
				"name": {Kind: processing.StringValue}}},
		}},
		"radio": {Agencies: []uuid.UUID{countyB}, Fields: map[string]processing.FieldSchema{
			"channel": {Kind: processing.IntegerValue},
		}},
	}
}

func TestAttributeSettingsCanonicalDropsInheritAndNormalizesValues(t *testing.T) {
	t.Parallel()
	settings := AttributeSettings{
		"station": {
			"town": {Mode: "value", Value: typed(t, processing.StringValue, ` "North" `)},
			"site": {Mode: "inherit"},
		},
		"radio": {"channel": {}},
	}
	got, err := settings.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	want := AttributeSettings{"station": {"town": {Mode: "value", Value: typed(t, processing.StringValue, `"North"`)}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical = %#v, want %#v", got, want)
	}
	for name, invalid := range map[string]AttributeSettings{
		"value in inherit": {"station": {"town": {Mode: "inherit", Value: typed(t, processing.StringValue, `"x"`)}}},
		"missing value":    {"station": {"town": {Mode: "value"}}},
		"unknown mode":     {"station": {"town": {Mode: "default"}}},
		"empty key":        {"": {"town": {Mode: "unset"}}},
	} {
		if _, err := invalid.Canonical(); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want invalid input", name, err)
		}
	}
}

func TestStoredAndRequestedAttributesCompareEqual(t *testing.T) {
	t.Parallel()
	requested, err := AttributeSettings{"station": {"site": {Mode: "value",
		Value: typed(t, processing.ObjectValue, `{"name":{"kind":"string","value":"Main"}}`)}}}.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL returns jsonb with reordered keys and added white space.
	var stored AttributeSettings
	if err := json.Unmarshal([]byte(`{"station": {"site": {"mode": "value", "value": {"value": {"name": {"value": "Main", "kind": "string"}}, "kind": "object"}}}}`), &stored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requested, stored) {
		t.Fatalf("stored %#v differs from requested %#v", stored, requested)
	}
	var empty AttributeSettings
	if err := json.Unmarshal([]byte(`{}`), &empty); err != nil || empty != nil {
		t.Fatalf("empty object = %#v, %v; want nil", empty, err)
	}
	if encoded, _ := json.Marshal(Settings{}.Attributes); string(encoded) != "{}" {
		t.Fatalf("nil attributes encode as %s", encoded)
	}
}

func TestAttributeCheckUsesSchemasAndAgency(t *testing.T) {
	t.Parallel()
	schemas := stationSchemas()
	valid := AttributeSettings{"station": {"town": {Mode: "value", Value: typed(t, processing.StringValue, `"North"`)},
		"site": {Mode: "unset"}}}
	if err := valid.Check(schemas, &countyA); err != nil {
		t.Fatalf("valid Unit entries: %v", err)
	}
	if err := valid.Check(schemas, nil); err != nil {
		t.Fatalf("valid template entries: %v", err)
	}
	for name, test := range map[string]struct {
		settings AttributeSettings
		agency   *uuid.UUID
	}{
		"schema of another Agency": {valid, &countyB},
		"unknown schema":           {AttributeSettings{"missing": {"town": {Mode: "unset"}}}, nil},
		"unknown field":            {AttributeSettings{"station": {"street": {Mode: "unset"}}}, nil},
		"wrong kind": {AttributeSettings{"station": {"town": {Mode: "value",
			Value: typed(t, processing.IntegerValue, `1`)}}}, nil},
	} {
		if err := test.settings.Check(schemas, test.agency); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want invalid input", name, err)
		}
	}
	defaults := AttributeDefaults{"radio": {"channel": {Mode: "value", Value: typed(t, processing.IntegerValue, `"4"`)}}}
	if err := defaults.Check(schemas); err != nil {
		t.Fatalf("defaults of any Agency's schema: %v", err)
	}
	if err := (AttributeDefaults{"radio": {"band": {Mode: "value", Value: typed(t, processing.StringValue, `"x"`)}}}).Check(schemas); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("undeclared default field: %v", err)
	}
}

func TestResolveAttributesInheritsOnlyAssignedSchemas(t *testing.T) {
	t.Parallel()
	schemas := stationSchemas()
	own := AttributeSettings{"station": {"town": {Mode: "value", Value: typed(t, processing.StringValue, `"North"`)},
		"site": {Mode: "unset"}}}
	defaults := AttributeDefaults{
		"station": {
			"town": {Mode: "value", Value: typed(t, processing.StringValue, `"South"`)},
			"site": {Mode: "value", Value: typed(t, processing.ObjectValue, `{}`)},
		},
		"radio": {"channel": {Mode: "value", Value: typed(t, processing.IntegerValue, `"4"`)}},
	}
	got := ResolveAttributes(own, defaults, schemas.AssignedTo(countyA))
	want := EffectiveAttributes{"station": {"town": {Value: typed(t, processing.StringValue, `"North"`), Source: SourceUnit}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Unit in A = %#v, want %#v (own value wins, unset stays unset, radio is not assigned)", got, want)
	}
	got = ResolveAttributes(nil, defaults, schemas.AssignedTo(countyB))
	if len(got) != 1 || got["radio"]["channel"].Source != SourceOrganization {
		t.Fatalf("Unit in B = %#v, want only the inherited radio channel", got)
	}
	if got := ResolveAttributes(nil, defaults, nil); len(got["station"])+len(got["radio"]) != 3 {
		t.Fatalf("template = %#v, want every default", got)
	}
}

func TestMaterializeAndCopyAttributes(t *testing.T) {
	t.Parallel()
	schemas := stationSchemas()
	own := AttributeSettings{"station": {"site": {Mode: "unset"}}}
	defaults := AttributeDefaults{
		"station": {"town": {Mode: "value", Value: typed(t, processing.StringValue, `"South"`)}},
		"radio":   {"channel": {Mode: "value", Value: typed(t, processing.IntegerValue, `"4"`)}},
	}
	got := MaterializeAttributes(own, defaults, schemas.AssignedTo(countyA))
	want := AttributeSettings{"station": {"site": {Mode: "unset"},
		"town": {Mode: "value", Value: typed(t, processing.StringValue, `"South"`)}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("materialized = %#v, want %#v", got, want)
	}
	before, after := ResolveAttributes(own, defaults, schemas.AssignedTo(countyA)), ResolveAttributes(got, nil, nil)
	if len(before) != len(after) || !reflect.DeepEqual(before["station"]["town"].Value, after["station"]["town"].Value) ||
		after["station"]["town"].Source != SourceUnit {
		t.Fatalf("materialization changed the effective values: %#v, then %#v", before, after)
	}

	effective := ResolveAttributes(nil, defaults, nil)
	copied := CopyAttributes(effective, schemas, countyA)
	want = AttributeSettings{"station": {"site": {Mode: "unset"},
		"town": {Mode: "value", Value: typed(t, processing.StringValue, `"South"`)}}}
	if !reflect.DeepEqual(copied, want) {
		t.Fatalf("copied = %#v, want %#v (only Agency A's schema, missing fields unset)", copied, want)
	}
}

func TestAttributeSettingsRejectUndeclaredProperties(t *testing.T) {
	t.Parallel()
	var settings AttributeSettings
	if err := json.Unmarshal([]byte(`{"station":{"town":{"mode":"unset","extra":true}}}`), &settings); err == nil {
		t.Fatal("undeclared entry property accepted")
	}
	var defaults AttributeDefaults
	if err := json.Unmarshal([]byte(`{"station":{"town":{"mode":"unset","extra":true}}}`), &defaults); err == nil {
		t.Fatal("undeclared default property accepted")
	}
}
