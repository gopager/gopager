package processing

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

func TestBytesEncodedInputAndDecodedAggregateExactBoundaries(t *testing.T) {
	t.Parallel()
	value, err := Literal(BytesValue, make([]byte, (MaxInputBytes-64)/4*3))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	exact := append(encoded, []byte(strings.Repeat(" ", MaxInputBytes-len(encoded)))...)
	var decoded Value
	if err := Decode(exact, &decoded); err != nil {
		t.Fatalf("exact 1048576 encoded bytes rejected: %v", err)
	}
	if err := Decode(append(exact, ' '), &decoded); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("1048577 encoded bytes accepted: %v", err)
	}
	for _, extra := range []int{0, 1} {
		left, err := Literal(BytesValue, make([]byte, MaxInputBytes/2))
		if err != nil {
			t.Fatal(err)
		}
		right, err := Literal(BytesValue, make([]byte, MaxInputBytes/2+extra))
		if err != nil {
			t.Fatal(err)
		}
		aggregate, err := json.Marshal([]Value{left, right})
		if err != nil {
			t.Fatal(err)
		}
		size, err := decodedBytesInJSON(aggregate)
		if extra == 0 && (err != nil || size != MaxInputBytes) {
			t.Fatalf("exact 1048576 decoded-byte accounting failed: size=%d err=%v", size, err)
		}
		if extra == 1 && platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Fatalf("1048577 aggregate decoded bytes accepted: %v", err)
		}
	}
}

func TestBytesEvaluationEncodedOutputExactBoundary(t *testing.T) {
	t.Parallel()
	binary, _ := Literal(BytesValue, []byte{})
	padding, _ := Literal(StringValue, "")
	result := Evaluation{Selected: true, Fields: map[string]map[string]FieldValue{testOutputSchemaKey: {
		"binary": {State: FieldPresent, Value: &binary}, "padding": {State: FieldPresent, Value: &padding}}}}
	base, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	binary, err = Literal(BytesValue, make([]byte, (MaxOutputBytes-len(base))/4*3))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	paddingSize := MaxOutputBytes - len(encoded)
	binding := EvaluationBinding{TransmissionID: uuid.New(), SourceRevision: 1, ContextRevision: 1}
	schema := testOutputs
	for _, extra := range []int{0, 1} {
		padding, err = Literal(StringValue, strings.Repeat("x", paddingSize+extra))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) != MaxOutputBytes+extra {
			t.Fatalf("incorrect encoded output boundary fixture: len=%d err=%v", len(encoded), err)
		}
		err = ValidateEvaluation(result, schema, binding)
		if extra == 0 && err != nil {
			t.Fatalf("262144 encoded output bytes rejected: %v", err)
		}
		if extra == 1 && platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Fatalf("262145 encoded output bytes accepted: %v", err)
		}
	}
}

func TestBytesComparisonCostUsesMaximumLengthsAndSumsCompositeLeaves(t *testing.T) {
	t.Parallel()
	if got := byteComparisonCost([]byte{1, 2, 3}, []byte{1}); got != 3 {
		t.Fatalf("comparison cost %d, want max(3,1)", got)
	}
	left := map[string]any{"items": []any{[]byte{1, 2, 3}, []byte{1}}}
	right := map[string]any{"items": []any{[]byte{1}, []byte{1, 2, 3, 4}}}
	if got := byteComparisonCost(left, right); got != 7 {
		t.Fatalf("nested cost %d, want max(3,1)+max(1,4)", got)
	}
	ref := FieldRef{Namespace: "source", Path: []string{"binary"}}
	null := Value{Kind: NullValue, Value: json.RawMessage("null")}
	fixture := fixtureFor(ref, FieldSchema{Kind: BytesValue, Nullable: true}, &null, FieldPresent)
	program, err := CompilePredicate(context.Background(),
		Predicate{Op: "in", Field: &ref, Values: []Value{null}}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(context.Background(), fixture)
	if err != nil || !result {
		t.Fatalf("nullable bytes membership failed: %+v, %v", result, err)
	}
}
