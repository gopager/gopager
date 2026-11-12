package ingeststore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/declarations"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

func TestProjectProcessingSourcePreservesOriginalTargetContexts(t *testing.T) {
	t.Parallel()
	var payload protocols.TransmissionPayload
	if err := json.Unmarshal([]byte(`{"form":"numeric","value":"123"}`), &payload); err != nil {
		t.Fatal(err)
	}
	function := uint8(2)
	targets := []protocols.Target{{Address: protocols.NumericAddress(1234), Function: &function}}
	matched := assignments.MatchResult{Revision: 7, Resolutions: []assignments.Resolution{
		{TargetPosition: 0, Status: "unassigned"},
	}, UnitContext: map[uuid.UUID]units.Context{}}
	observed := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)

	projection, err := projectProcessingSource(context.Background(), uuid.New(), uuid.New(), "Network",
		protocols.POCSAG, payload, observed, 1, matched, targets, declarations.UnitAttributes{})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := processing.CaptureSource(context.Background(), projection)
	if err != nil {
		t.Fatalf("CaptureSource rejected projection: %v", err)
	}
	identity, err := captured.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if identity.OriginalTargetCount != 1 || identity.ContextRevision != matched.Revision {
		t.Fatalf("captured identity=%+v", identity)
	}
	if field := findCapturedField(t, projection.Fields, "source", "payload", "numeric", "value"); field.State != processing.FieldPresent ||
		field.Value == nil || string(field.Value.Value) != `"123"` {
		t.Fatalf("numeric Payload field=%+v", field)
	}
	targetFields := projection.Targets[0].Fields
	address := findCapturedField(t, targetFields, "target", "address", "number")
	if address.State != processing.FieldPresent || address.Value == nil || string(address.Value.Value) != `"1234"` {
		t.Fatalf("original numeric Address field=%+v", address)
	}
	functionField := findCapturedField(t, targetFields, "target", "function")
	if functionField.State != processing.FieldPresent || functionField.Value == nil ||
		string(functionField.Value.Value) != `"2"` {
		t.Fatalf("original Function field=%+v", functionField)
	}
	if unitField := findCapturedField(t, targetFields, "target", "unit", "id"); unitField.State != processing.FieldAbsent {
		t.Fatalf("unassigned target Unit field=%+v", unitField)
	}
}

func findCapturedField(t *testing.T, fields []processing.CapturedField, namespace string, path ...string) processing.FieldValue {
	t.Helper()
	for _, field := range fields {
		if field.Reference.Namespace != namespace || len(field.Reference.Path) != len(path) {
			continue
		}
		matches := true
		for i := range path {
			if field.Reference.Path[i] != path[i] {
				matches = false
				break
			}
		}
		if matches {
			return field.Field
		}
	}
	t.Fatalf("missing captured field %s.%v", namespace, path)
	return processing.FieldValue{}
}

func TestProjectProcessingSourceCapturesUnitAttributes(t *testing.T) {
	t.Parallel()
	registry := fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{{
		Key: "station", Subject: attributes.SubjectUnit,
		Fields: map[string]attributes.Field{"town": {Kind: processing.StringValue}},
	}}, 1))
	var payload protocols.TransmissionPayload
	if err := json.Unmarshal([]byte(`{"form":"numeric","value":"123"}`), &payload); err != nil {
		t.Fatal(err)
	}
	unit := uuid.New()
	function := uint8(2)
	targets := []protocols.Target{{Address: protocols.NumericAddress(1), Function: &function},
		{Address: protocols.NumericAddress(2), Function: &function}}
	matched := assignments.MatchResult{Revision: 7, Resolutions: []assignments.Resolution{
		{TargetPosition: 0, Status: "assigned", UnitID: &unit}, {TargetPosition: 1, Status: "unassigned"},
	}, UnitContext: map[uuid.UUID]units.Context{unit: {ID: unit, Revision: 1}}}
	town, err := processing.Literal(processing.StringValue, "A")
	if err != nil {
		t.Fatal(err)
	}
	declared, err := declarations.UnitAttributeDeclarations(registry,
		[]processing.FieldRef{{Namespace: "target", Path: []string{"unit", "attributes", "station", "town"}}})
	if err != nil {
		t.Fatal(err)
	}
	source := declarations.UnitAttributes{Declared: declared,
		Effective: map[uuid.UUID]units.EffectiveAttributes{unit: {"station": {
			"town": {Value: &town, Source: units.SourceUnit}}}}}

	projection, err := projectProcessingSource(context.Background(), uuid.New(), uuid.New(), "Network",
		protocols.POCSAG, payload, time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC), 1, matched, targets, source)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := processing.CaptureSource(context.Background(), projection)
	if err != nil {
		t.Fatalf("CaptureSource rejected projection: %v", err)
	}
	identity, err := captured.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if identity.ContextRevision != matched.Revision {
		t.Fatalf("Unit attributes changed the context revision: %+v", identity)
	}
	assigned := findCapturedField(t, projection.Targets[0].Fields, "target", "unit", "attributes", "station", "town")
	if assigned.State != processing.FieldPresent || assigned.Value == nil || string(assigned.Value.Value) != `"A"` {
		t.Fatalf("assigned target field=%+v", assigned)
	}
	if len(assigned.Dependencies) != 1 || assigned.Dependencies[0].TargetPosition == nil ||
		*assigned.Dependencies[0].TargetPosition != 0 || assigned.Dependencies[0].Groups[0] != "unit" {
		t.Fatalf("dependencies=%+v", assigned.Dependencies)
	}
	if free := findCapturedField(t, projection.Targets[1].Fields, "target", "unit", "attributes", "station", "town"); free.State != processing.FieldAbsent {
		t.Fatalf("unassigned target field=%+v", free)
	}
}
