package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

func TestWorkflowExactResultAndStepBoundaries(t *testing.T) {
	ctx := context.Background()
	_, source := sourceFixture(t)
	definition := selectedDefinition()
	text := Value{StringValue, json.RawMessage(`"x"`)}
	definition.OutputSchema["blob"] = FieldSchema{Kind: StringValue}
	definition.Outputs["blob"] = Assignment{Literal: &text}
	program, err := CompileWorkflow(ctx, definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	base, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for extra := 0; extra <= 1; extra++ {
		text, err = Literal(StringValue, strings.Repeat("x", MaxOutputBytes-len(base)+1+extra))
		if err != nil {
			t.Fatal(err)
		}
		program, err = CompileWorkflow(ctx, definition, source, nil)
		if err != nil {
			t.Fatal(err)
		}
		result, err := program.Evaluate(ctx, source)
		if extra == 0 {
			encoded, encodeErr := json.Marshal(result)
			if err != nil || encodeErr != nil || len(encoded) != MaxOutputBytes {
				t.Fatalf("exact result boundary failed: %d, %v, %v", len(encoded), err, encodeErr)
			}
		} else if platform.FailureCode(err) != platform.CodeResourceLimit || result.Selected {
			t.Fatalf("result +1 yielded success: %+v, %v", result, err)
		}
	}
	for count := MaxSteps; count <= MaxSteps+1; count++ {
		definition := selectedDefinition()
		for i := range count {
			definition.Steps = append(definition.Steps, Step{ID: fmt.Sprintf("step%d", i), Kind: "map",
				OutputSchema: map[string]FieldSchema{}, Map: map[string]Assignment{}})
		}
		program, err := CompileWorkflow(ctx, definition, source, nil)
		if count > MaxSteps {
			if platform.FailureCode(err) != platform.CodeResourceLimit {
				t.Fatalf("step +1 accepted: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := program.Evaluate(ctx, source); err != nil {
			t.Fatalf("exact step boundary failed: %v", err)
		}
	}
}

func TestWorkflowAggregateIntermediateBoundaryAndLiteralOwnership(t *testing.T) {
	ctx := context.Background()
	_, source := sourceFixture(t, "111", "222")
	ref := FieldRef{"target", []string{"address"}}
	yes := Value{BooleanValue, json.RawMessage(`true`)}
	definition := selectedDefinition()
	definition.Selector = Predicate{Op: "present", Field: &ref, Value: &yes}
	large, err := Literal(StringValue, strings.Repeat("x", MaxInputBytes/4))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		definition.Steps = append(definition.Steps, Step{ID: fmt.Sprintf("step%d", i), Kind: "map",
			OutputSchema: map[string]FieldSchema{"blob": {Kind: StringValue}},
			Map:          map[string]Assignment{"blob": {Literal: &large}}})
	}
	program, err := CompileWorkflow(ctx, definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := program.Evaluate(ctx, source); platform.FailureCode(err) != platform.CodeResourceLimit || result.Selected {
		t.Fatalf("per-target reset bypassed aggregate budget: %+v, %v", result, err)
	}
	definition = selectedDefinition()
	constant := Value{StringValue, json.RawMessage(`"constant"`)}
	definition.OutputSchema["constant"] = FieldSchema{Kind: StringValue}
	definition.Outputs["constant"] = Assignment{Literal: &constant}
	program, err = CompileWorkflow(ctx, definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	result.Fields["constant"].Value.Value[1] = 'X'
	result, err = program.Evaluate(ctx, source)
	if err != nil || string(result.Fields["constant"].Value.Value) != `"constant"` {
		t.Fatalf("result mutation changed immutable definition: %+v, %v", result, err)
	}
}

func TestWorkflowConversionsAreExplicitAndExact(t *testing.T) {
	for _, item := range []struct {
		text string
		kind ValueKind
		ok   bool
	}{
		{"9223372036854775807", IntegerValue, true}, {"9223372036854775808", IntegerValue, false},
		{"+7", IntegerValue, true}, {" 7", IntegerValue, false}, {"1.5", IntegerValue, false},
		{"true", BooleanValue, true}, {"TRUE", BooleanValue, false}, {"1", BooleanValue, false},
		{"1e3", NumberValue, true}, {" 1", NumberValue, false}, {"NaN", NumberValue, false},
		{"1e9999", NumberValue, false}, {"1,5", NumberValue, false},
		{"2026-11-01T12:00:00+01:00", TimestampValue, true}, {"2026-11-01T12:00:00", TimestampValue, false},
	} {
		t.Run(item.text, func(t *testing.T) {
			value, err := Literal(StringValue, item.text)
			if err != nil {
				t.Fatal(err)
			}
			_, err = convertValue(value, FieldSchema{Kind: item.kind}, item.kind)
			if (err == nil) != item.ok {
				t.Fatalf("conversion validity differs: %v", err)
			}
		})
	}
}
