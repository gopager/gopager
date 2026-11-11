package processing

import (
	"encoding/json"
	"slices"
	"testing"
)

// classificationDefinition reads a payload source field in the selector and the branch
// condition, an address target field in the then arm and a second payload field in the else
// arm. Its outputs copy the branch result and one source field directly.
func classificationDefinition() Definition {
	flag := FieldRef{"source", []string{"flag"}}
	text := FieldRef{"source", []string{"text"}}
	address := FieldRef{"target", []string{"address"}}
	step := FieldRef{"step", []string{"pick", "value"}}
	no := Value{BooleanValue, json.RawMessage(`false`)}
	return Definition{SchemaVersion: 1,
		Selector: Predicate{Op: "not", Args: []Predicate{{Op: "eq", Field: &flag, Value: &no}}},
		Steps: []Step{{ID: "pick", Kind: "branch",
			OutputSchema: map[string]FieldSchema{"value": {Kind: StringValue, Optional: true}},
			Branch: &BranchStep{
				Condition:   Predicate{Op: "eq", Field: &flag, Value: &no},
				ThenOutputs: map[string]Assignment{"value": {Field: &address}},
				ElseOutputs: map[string]Assignment{"value": {Field: &text}},
			}}},
		Outputs: map[string]map[string]Assignment{testOutputSchemaKey: {"picked": {Field: &step}, "text": {Field: &text}}}}
}

// TestEvaluationClassificationIsPerDefinitionNotPerValue expects that every source field the
// definition references (the branch arm that did not run included) contributes its own
// classification to Dependencies(), and no derived output carries a per-value classification.
func TestEvaluationClassificationIsPerDefinitionNotPerValue(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t, "111")
	program, err := CompileWorkflow(t.Context(), classificationDefinition(), source, testOutputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(t.Context(), source)
	if err != nil || !result.Selected || len(result.TargetResults) != 1 {
		t.Fatalf("unexpected evaluation: %+v, %v", result, err)
	}
	for name, field := range result.TargetResults[0].Fields[testOutputSchemaKey] {
		if len(field.Dependencies) != 0 {
			t.Fatalf("derived output %q kept a per-value classification: %+v", name, field.Dependencies)
		}
	}
	if got := string(result.TargetResults[0].Fields["t"]["picked"].Value.Value); got != `"LEVEL=7"` {
		t.Fatalf("false branch condition did not run else: %s", got)
	}
	dependencies, err := result.Dependencies()
	if err != nil {
		t.Fatal(err)
	}
	var groups []string
	for _, dependency := range dependencies {
		groups = append(groups, dependency.Groups...)
	}
	slices.Sort(groups)
	groups = slices.Compact(groups)
	if !slices.Equal(groups, []string{"address", "payload"}) {
		t.Fatalf("Dependencies() groups = %v, want the union of every referenced source field", groups)
	}
}
