package delivery

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

func incidentDeclarations() fields.Declarations {
	level := attributes.Field{Kind: processing.IntegerValue}
	schema := attributes.Schema{Key: "incident", Subject: attributes.SubjectMessage, Fields: map[string]attributes.Field{
		"city":  {Kind: processing.StringValue},
		"level": level,
		"owner": {Kind: processing.ObjectValue, Properties: map[string]attributes.Field{"name": {Kind: processing.StringValue}}},
	}}
	return fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{schema}, 1)).Declarations(fields.ScopeDelivery, true)
}

func attribute(field string, position *int, unit *uuid.UUID, value processing.Value) readmodel.AttributeValue {
	return readmodel.AttributeValue{SchemaKey: "incident", Field: field, TargetPosition: position, UnitID: unit, Value: value}
}

func ref(path ...string) processing.FieldRef {
	return processing.FieldRef{Namespace: "attributes", Path: path}
}

func viewOf(units []readmodel.UnitRecord, groups []string, values ...readmodel.AttributeValue) *view {
	granted := map[string]bool{}
	for _, group := range groups {
		granted[group] = true
	}
	projection := &readmodel.DeliveryProjection{}
	projection.Item.Attributes = &values
	return &view{message: projection, units: units, groups: granted, declarations: incidentDeclarations()}
}

func pointer[T any](value T) *T { return &value }

func texts(values []processing.Value) []string {
	out := make([]string, len(values))
	for i, value := range values {
		_ = json.Unmarshal(value.Value, &out[i])
	}
	return out
}

func TestAttributeCandidatesAreUntargetedThenRelevantTargetsByPosition(t *testing.T) {
	t.Parallel()
	first, second, foreign := uuid.New(), uuid.New(), uuid.New()
	values := []readmodel.AttributeValue{
		attribute("city", nil, nil, stringValue("everywhere")),
		attribute("city", pointer(0), &first, stringValue("A")),
		attribute("city", pointer(1), &second, stringValue("B")),
		attribute("city", pointer(2), &foreign, stringValue("elsewhere")),
		attribute("city", pointer(3), nil, stringValue("unit hidden")),
		attribute("level", nil, nil, processing.Value{Kind: processing.IntegerValue, Value: json.RawMessage(`"3"`)})}
	v := viewOf([]readmodel.UnitRecord{{UnitID: second}, {UnitID: first}}, []string{"derived"}, values...)

	if got := texts(v.Values(ref("incident", "city"))); len(got) != 3 || got[0] != "everywhere" || got[1] != "A" || got[2] != "B" {
		t.Fatalf("unsplit candidates = %v", got)
	}
	v = viewOf([]readmodel.UnitRecord{{UnitID: second}}, []string{"derived"}, values...)
	if got := texts(v.Values(ref("incident", "city"))); len(got) != 2 || got[1] != "B" {
		t.Fatalf("split candidates = %v", got)
	}
	v = viewOf(nil, []string{"derived"}, values...)
	if got := texts(v.Values(ref("incident", "city"))); len(got) != 1 || got[0] != "everywhere" {
		t.Fatalf("Unit-less candidates = %v", got)
	}
}

func TestLookupFieldPrefersUntargetedThenLowestRelevantPosition(t *testing.T) {
	t.Parallel()
	first, second := uuid.New(), uuid.New()
	units := []readmodel.UnitRecord{{UnitID: first}, {UnitID: second}}
	targets := []readmodel.AttributeValue{
		attribute("city", pointer(5), &second, stringValue("late")),
		attribute("city", pointer(2), &first, stringValue("early")),
	}
	lookup := func(v *view) string {
		t.Helper()
		got, err := v.LookupField(context.Background(), ref("incident", "city"))
		if err != nil || got.State != processing.FieldPresent {
			t.Fatalf("lookup = %+v, %v", got, err)
		}
		return texts([]processing.Value{*got.Value})[0]
	}
	// The read side orders target values by position, the untargeted one first.
	ordered := []readmodel.AttributeValue{targets[1], targets[0]}
	if got := lookup(viewOf(units, []string{"derived"}, ordered...)); got != "early" {
		t.Fatalf("lowest position = %q", got)
	}
	withUntargeted := append([]readmodel.AttributeValue{attribute("city", nil, nil, stringValue("whole"))}, ordered...)
	if got := lookup(viewOf(units, []string{"derived"}, withUntargeted...)); got != "whole" {
		t.Fatalf("untargeted = %q", got)
	}
}

func TestAttributeValuesBelowObjectsAndUndecodableValues(t *testing.T) {
	t.Parallel()
	owner := processing.Value{Kind: processing.ObjectValue,
		Value: json.RawMessage(`{"name":{"kind":"string","value":"Ada"}}`)}
	v := viewOf(nil, []string{"derived"}, attribute("owner", nil, nil, owner),
		attribute("level", nil, nil, stringValue("not an integer")))
	if got := texts(v.Values(ref("incident", "owner", "name"))); len(got) != 1 || got[0] != "Ada" {
		t.Fatalf("property candidates = %v", got)
	}
	if got := v.Values(ref("incident", "owner", "missing")); len(got) != 0 {
		t.Fatalf("missing property candidates = %v", got)
	}
	absent, err := v.LookupField(context.Background(), ref("incident", "level"))
	if err != nil || absent.State != processing.FieldAbsent {
		t.Fatalf("a value of the wrong kind must read as absent, got %+v, %v", absent, err)
	}
}

func TestFieldsOfAnUnreadableGroupHaveNoValues(t *testing.T) {
	t.Parallel()
	v := viewOf(nil, nil, attribute("city", nil, nil, stringValue("secret")))
	if got := v.Values(ref("incident", "city")); len(got) != 0 {
		t.Fatalf("candidates without the derived group = %v", got)
	}
	if got, _ := v.LookupField(context.Background(), ref("incident", "city")); got.State != processing.FieldAbsent {
		t.Fatalf("lookup without the derived group = %+v", got)
	}
}

func TestMessageUnitIDsIsAListOfCandidates(t *testing.T) {
	t.Parallel()
	first, second := uuid.New(), uuid.New()
	v := viewOf([]readmodel.UnitRecord{{UnitID: first}, {UnitID: second}}, []string{"unit"})
	v.event.Type = "message.created"
	v.message.Item.MessageID = uuid.New()
	operand := stringValue(second.String())
	filter, err := fields.CompileFilter(processing.Predicate{Op: "eq",
		Field: &processing.FieldRef{Namespace: "message", Path: []string{"unitIds"}}, Value: &operand}, v.declarations)
	if err != nil {
		t.Fatal(err)
	}
	if !filter.Match(v) {
		t.Fatal("one element equal to the operand must match")
	}
	v.units = []readmodel.UnitRecord{{UnitID: first}}
	if filter.Match(v) {
		t.Fatal("no element equal to the operand must not match")
	}
}

// BenchmarkRelevantUnit asks for the relevance of many Units of a view, as filtering and layout
// do for each attribute value.
func BenchmarkRelevantUnit(b *testing.B) {
	units := make([]readmodel.UnitRecord, 50)
	for i := range units {
		units[i].UnitID = uuid.New()
	}
	v := viewOf(units, []string{"derived"})
	probe := units[len(units)-1].UnitID
	b.ResetTimer()
	for range b.N {
		if !v.relevantUnit(probe) {
			b.Fatal("unit not relevant")
		}
	}
}
