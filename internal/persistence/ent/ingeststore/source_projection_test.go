package ingeststore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

func TestProjectProcessingSourcePreservesOriginalTargetContexts(t *testing.T) {
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
		protocols.POCSAG, payload, observed, 1, matched, targets)
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
