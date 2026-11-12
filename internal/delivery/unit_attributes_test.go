package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

func unitSchemas() []attributes.Schema {
	return []attributes.Schema{
		{Key: "station", Subject: attributes.SubjectUnit, Fields: map[string]attributes.Field{
			"town": {Kind: processing.StringValue},
			"kind": {Kind: processing.StringValue},
			"old":  {Kind: processing.StringValue, Deprecated: true},
		}},
		{Key: "contact", Subject: attributes.SubjectUnit, Fields: map[string]attributes.Field{
			"pager":    {Kind: processing.StringValue},
			"priority": {Kind: processing.IntegerValue},
			"urgent":   {Kind: processing.BooleanValue},
		}},
	}
}

func unitRegistry() *fields.Registry {
	return fields.New(fields.Builtin(), attributes.NewProvider(unitSchemas(), 1))
}

func unitRef(path ...string) processing.FieldRef {
	return processing.FieldRef{Namespace: "unit", Path: append([]string{"attributes"}, path...)}
}

func literal(t *testing.T, kind processing.ValueKind, value any) processing.Value {
	t.Helper()
	result, err := processing.Literal(kind, value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func equals(t *testing.T, ref processing.FieldRef, text string) processing.Predicate {
	return processing.Predicate{Op: "eq", Field: &ref, Value: pointer(literal(t, processing.StringValue, text))}
}

func unitWith(name string, values map[string]string) readmodel.UnitRecord {
	unit := readmodel.UnitRecord{UnitID: uuid.New(), Name: name}
	for field, text := range values {
		unit.Attributes = append(unit.Attributes, readmodel.UnitAttribute{SchemaKey: "station", Field: field,
			Value: stringValue(text), Source: "unit"})
	}
	return unit
}

func unitView(units []readmodel.UnitRecord, withoutUnit bool) *view {
	return &view{message: &readmodel.DeliveryProjection{}, units: units, groups: map[string]bool{"unit": true},
		declarations: unitRegistry().Declarations(fields.ScopeDelivery, true), withoutUnit: withoutUnit}
}

func compileFilter(t *testing.T, predicate processing.Predicate) *fields.Filter {
	t.Helper()
	filter, err := fields.CompileFilter(predicate, unitRegistry().Declarations(fields.ScopeDelivery, true))
	if err != nil {
		t.Fatal(err)
	}
	return filter
}

func TestUnitAttributeFilterIsCorrelatedPerUnit(t *testing.T) {
	t.Parallel()
	x := unitWith("X", map[string]string{"town": "A", "kind": "ladder"})
	y := unitWith("Y", map[string]string{"town": "B", "kind": "engine"})
	town, kind := unitRef("station", "town"), unitRef("station", "kind")
	cases := map[string]struct {
		predicate processing.Predicate
		unsplit   bool
		splitX    bool
		splitY    bool
	}{
		"both on one unit A and engine": {processing.Predicate{Op: "and", Args: []processing.Predicate{
			equals(t, town, "A"), equals(t, kind, "engine")}}, false, false, false},
		"both on one unit B and engine": {processing.Predicate{Op: "and", Args: []processing.Predicate{
			equals(t, town, "B"), equals(t, kind, "engine")}}, true, false, true},
		"not A": {processing.Predicate{Op: "not", Args: []processing.Predicate{equals(t, town, "A")}}, true, false, true},
		"mixed with a message field": {processing.Predicate{Op: "and", Args: []processing.Predicate{
			equals(t, town, "A"), {Op: "present", Field: &processing.FieldRef{Namespace: "message", Path: []string{"id"}},
				Value: pointer(literal(t, processing.BooleanValue, true))}}}, true, true, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			filter := compileFilter(t, tc.predicate)
			if got := matches(filter, unitView([]readmodel.UnitRecord{x, y}, false)); got != tc.unsplit {
				t.Errorf("unsplit = %v, want %v", got, tc.unsplit)
			}
			if got := matches(filter, unitView([]readmodel.UnitRecord{x}, false)); got != tc.splitX {
				t.Errorf("split X = %v, want %v", got, tc.splitX)
			}
			if got := matches(filter, unitView([]readmodel.UnitRecord{y}, false)); got != tc.splitY {
				t.Errorf("split Y = %v, want %v", got, tc.splitY)
			}
		})
	}
}

func TestUnitAttributeFilterBindsNoUnitForUnitlessAppearances(t *testing.T) {
	t.Parallel()
	x := unitWith("X", map[string]string{"town": "A"})
	notA := compileFilter(t, processing.Predicate{Op: "not", Args: []processing.Predicate{equals(t, unitRef("station", "town"), "A")}})
	if matches(notA, unitView([]readmodel.UnitRecord{x}, false)) {
		t.Error("a Message whose only appearance is a Unit in A must not match not(A)")
	}
	if !matches(notA, unitView([]readmodel.UnitRecord{x}, true)) {
		t.Error("an eligible Unit-less appearance has no candidates, so not(A) holds on it")
	}
	if !matches(notA, unitView(nil, false)) {
		t.Error("a view without Units is evaluated with none bound")
	}
	isA := compileFilter(t, equals(t, unitRef("station", "town"), "A"))
	if matches(isA, unitView(nil, true)) {
		t.Error("a Unit-less appearance never has a value")
	}
}

func TestFilterWithoutUnitAttributesIsUnchanged(t *testing.T) {
	t.Parallel()
	x := unitWith("X", map[string]string{"town": "A"})
	name := compileFilter(t, processing.Predicate{Op: "eq", Field: &processing.FieldRef{Namespace: "message", Path: []string{"unitNames"}},
		Value: pointer(stringValue("X"))})
	if !matches(name, unitView([]readmodel.UnitRecord{x}, false)) {
		t.Error("filter on Unit names must keep matching")
	}
	if !matches(nil, unitView(nil, false)) {
		t.Error("no filter matches everything")
	}
}

func TestLayoutReadsTheFirstUnitWithAValue(t *testing.T) {
	t.Parallel()
	first := unitWith("A", nil)
	second := unitWith("B", map[string]string{"town": "Bern"})
	third := unitWith("C", map[string]string{"town": "Chur"})
	current := unitView([]readmodel.UnitRecord{first, second, third}, false)
	got, err := current.LookupField(context.Background(), unitRef("station", "town"))
	if err != nil || got.State != processing.FieldPresent || string(got.Value.Value) != `"Bern"` {
		t.Fatalf("LookupField = %+v, %v", got, err)
	}
	none, _ := current.LookupField(context.Background(), unitRef("station", "kind"))
	if none.State != processing.FieldAbsent {
		t.Errorf("a field no Unit has is absent, got %v", none.State)
	}
	current.groups = map[string]bool{}
	hidden, _ := current.LookupField(context.Background(), unitRef("station", "town"))
	if hidden.State != processing.FieldAbsent {
		t.Error("an unreadable group has no values")
	}
}

type parameterStore struct {
	Store
	parameters map[uuid.UUID]map[string]processing.Value
}

func (s parameterStore) UnitParameters(context.Context, platform.TransactionContext, uuid.UUID, []uuid.UUID) (map[uuid.UUID]map[string]processing.Value, error) {
	return s.parameters, nil
}

func contactUnit(t *testing.T, name string, values map[string]processing.Value) readmodel.UnitRecord {
	unit := readmodel.UnitRecord{UnitID: uuid.New(), Name: name}
	for field, value := range values {
		unit.Attributes = append(unit.Attributes, readmodel.UnitAttribute{SchemaKey: "contact", Field: field, Value: value, Source: "unit"})
	}
	return unit
}

func bindingTarget(t *testing.T) (TargetDescriptor, TargetReference) {
	descriptor := TargetDescriptor{Inputs: []InputField{
		{Name: "recipient", Kind: processing.StringValue, Required: true},
		{Name: "priority", Kind: processing.IntegerValue},
	}}
	target := TargetReference{InstanceID: uuid.New(),
		Inputs: map[string]processing.Value{"priority": literal(t, processing.IntegerValue, "1")},
		InputBindings: map[string]processing.FieldRef{"recipient": unitRef("contact", "pager"),
			"priority": unitRef("contact", "priority")}}
	return descriptor, target
}

func TestInputBindingsFollowPrecedenceAndConflict(t *testing.T) {
	t.Parallel()
	descriptor, target := bindingTarget(t)
	x := contactUnit(t, "X", map[string]processing.Value{"pager": stringValue("111")})
	y := contactUnit(t, "Y", map[string]processing.Value{"pager": stringValue("222"), "priority": literal(t, processing.IntegerValue, "3")})
	w := contactUnit(t, "W", nil)
	null := contactUnit(t, "N", map[string]processing.Value{"pager": {Kind: processing.NullValue, Value: []byte("null")}})
	store := parameterStore{parameters: map[uuid.UUID]map[string]processing.Value{
		y.UnitID: {"priority": literal(t, processing.IntegerValue, "5")}}}
	deps := Dependencies{Store: store}
	resolve := func(units ...readmodel.UnitRecord) (map[string]processing.Value, string) {
		inputs, code, err := resolveInputs(context.Background(), nil, deps, descriptor, target, units)
		if err != nil {
			t.Fatal(err)
		}
		return inputs, code
	}
	if _, code := resolve(x, y); code != CodeParameterConflict {
		t.Errorf("unsplit with different pagers: code %q", code)
	}
	inputs, code := resolve(x)
	if code != "" || string(inputs["recipient"].Value) != `"111"` || string(inputs["priority"].Value) != `"1"` {
		t.Errorf("X: %v %q (constant is the fallback)", inputs, code)
	}
	inputs, code = resolve(y)
	if code != "" || string(inputs["recipient"].Value) != `"222"` || string(inputs["priority"].Value) != `"5"` {
		t.Errorf("Y: %v %q (Unit parameter beats the binding)", inputs, code)
	}
	if _, code = resolve(w); code != CodeInputInvalid {
		t.Errorf("W without a pager: code %q", code)
	}
	if _, code = resolve(null, x); code != "" {
		t.Errorf("a null does not count, so X decides: code %q", code)
	}
	target.Inputs["recipient"] = stringValue("fallback")
	if inputs, code = resolve(w); code != "" || string(inputs["recipient"].Value) != `"fallback"` {
		t.Errorf("constant stays when no Unit has a value: %v %q", inputs, code)
	}
}

func TestCheckBindingsValidatesAtSave(t *testing.T) {
	t.Parallel()
	declarations := unitRegistry().Declarations(fields.ScopeDelivery, true)
	descriptor, _ := bindingTarget(t)
	cases := map[string]map[string]processing.FieldRef{
		"valid":                {"recipient": unitRef("contact", "pager")},
		"undeclared input":     {"other": unitRef("contact", "pager")},
		"kind mismatch":        {"recipient": unitRef("contact", "urgent")},
		"deprecated field":     {"recipient": unitRef("station", "old")},
		"unknown field":        {"recipient": unitRef("contact", "missing")},
		"not a unit attribute": {"recipient": {Namespace: "unit", Path: []string{"name"}}},
		"message attribute":    {"recipient": {Namespace: "attributes", Path: []string{"contact", "pager"}}},
	}
	for name, bindings := range cases {
		err := checkBindings(bindings, descriptor, declarations)
		if (name == "valid") != (err == nil) || (err != nil && !errors.Is(err, authz.ErrInvalidInput)) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestHealthCheckCoversInputBindings(t *testing.T) {
	t.Parallel()
	deps := Dependencies{Fields: fixedRegistry{unitRegistry()}, Layouts: fixedLayouts{err: authz.ErrNotFound}}
	_, binding := bindingTarget(t)
	subscription := Subscription{Targets: []TargetReference{binding}}
	if _, reason, err := healthCheck(context.Background(), nil, deps, subscription, map[string]bool{"unit": true}); err != nil || reason != "" {
		t.Fatalf("valid binding: reason %q, err %v", reason, err)
	}
	if _, reason, _ := healthCheck(context.Background(), nil, deps, subscription, map[string]bool{}); reason != ReasonFieldUnavailable {
		t.Errorf("lost group: reason %q", reason)
	}
	binding.InputBindings = map[string]processing.FieldRef{"recipient": unitRef("contact", "gone")}
	subscription = Subscription{Targets: []TargetReference{binding}}
	if _, reason, _ := healthCheck(context.Background(), nil, deps, subscription, map[string]bool{"unit": true}); reason != ReasonFieldUnavailable {
		t.Errorf("lost field: reason %q", reason)
	}
}
