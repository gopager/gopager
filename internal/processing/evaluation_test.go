package processing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

func TestEvaluationValidationRejectsNamespacesAndOriginalPositionSubstitution(t *testing.T) {
	t.Parallel()
	binding := EvaluationBinding{TransmissionID: uuid.New(), SourceRevision: 1, ContextRevision: 2, TargetLimit: 2}
	schema := map[string]FieldSchema{}
	if err := ValidateEvaluation(Evaluation{}, schema, binding); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvaluation(Evaluation{Fields: map[string]FieldValue{}}, schema, binding); err == nil {
		t.Fatal("not-selected outcome created a namespace")
	}
	result := Evaluation{Selected: true, TargetResults: []TargetResult{
		{TargetPosition: 0, Fields: map[string]FieldValue{}},
		{TargetPosition: 1, Fields: map[string]FieldValue{}},
	}}
	if err := ValidateEvaluation(result, schema, binding); err != nil {
		t.Fatal(err)
	}
	result.TargetResults[1].TargetPosition = 2
	if err := ValidateEvaluation(result, schema, binding); err == nil {
		t.Fatal("accepted position outside actual original count")
	}
	result.TargetResults[1].TargetPosition = 0
	if err := ValidateEvaluation(result, schema, binding); err == nil {
		t.Fatal("accepted repeated original position")
	}
	value, err := Literal(BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	position := uint32(2)
	fields := map[string]FieldValue{"flag": {State: FieldPresent, Value: &value, Dependencies: []Dependency{{
		TransmissionID: binding.TransmissionID, NetworkID: uuid.New(), SourceRevision: 1, ContextRevision: 2,
		TargetPosition: &position, Groups: []string{"payload"},
	}}}}
	if err := ValidateEvaluation(Evaluation{Selected: true, Fields: fields},
		map[string]FieldSchema{"flag": {Kind: BooleanValue}}, binding); err == nil {
		t.Fatal("accepted dependency outside actual original count")
	}
}

func TestEvaluationValidationMeasuresExactWholeOutputBoundary(t *testing.T) {
	t.Parallel()
	binding := EvaluationBinding{TransmissionID: uuid.New(), SourceRevision: 1, ContextRevision: 1}
	schema := map[string]FieldSchema{"text": {Kind: StringValue}}
	value, err := Literal(StringValue, "")
	if err != nil {
		t.Fatal(err)
	}
	result := Evaluation{Selected: true, Fields: map[string]FieldValue{"text": {State: FieldPresent, Value: &value}}}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	overhead := len(encoded)
	for _, extra := range []int{0, 1} {
		value, err := Literal(StringValue, strings.Repeat("x", MaxOutputBytes-overhead+extra))
		if err != nil {
			t.Fatal(err)
		}
		result.Fields["text"] = FieldValue{State: FieldPresent, Value: &value}
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) != MaxOutputBytes+extra {
			t.Fatalf("incorrect exact-boundary fixture: len=%d err=%v", len(encoded), err)
		}
		err = ValidateEvaluation(result, schema, binding)
		if extra == 0 && err != nil {
			t.Fatalf("exact whole-output limit rejected: %v", err)
		}
		if extra == 1 && platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Fatalf("oversized whole output accepted: %v", err)
		}
	}
}
