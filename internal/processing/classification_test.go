package processing

import (
	"encoding/json"
	"slices"
	"strings"
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
		OutputSchema: map[string]FieldSchema{"picked": {Kind: StringValue, Optional: true}, "text": {Kind: StringValue}},
		Outputs:      map[string]Assignment{"picked": {Field: &step}, "text": {Field: &text}}}
}

// TestEvaluationClassificationIsPerDefinitionNotPerValue expects that every source field the
// definition references (the branch arm that did not run included) contributes its own
// classification to Dependencies(), and no derived output carries a per-value classification.
func TestEvaluationClassificationIsPerDefinitionNotPerValue(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t, "111")
	program, err := CompileWorkflow(t.Context(), classificationDefinition(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(t.Context(), source)
	if err != nil || !result.Selected || len(result.TargetResults) != 1 {
		t.Fatalf("unexpected evaluation: %+v, %v", result, err)
	}
	for name, field := range result.TargetResults[0].Fields {
		if len(field.Dependencies) != 0 {
			t.Fatalf("derived output %q kept a per-value classification: %+v", name, field.Dependencies)
		}
	}
	if got := string(result.TargetResults[0].Fields["picked"].Value.Value); got != `"LEVEL=7"` {
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

// TestSyntheticCatalogDeclaresOneGroupPerField keeps save-time classification total: every
// catalog field a definition can reference declares exactly one field group, and the
// compiled program reports each referenced field with it.
func TestSyntheticCatalogDeclaresOneGroupPerField(t *testing.T) {
	t.Parallel()
	declarations, err := NewSyntheticDeclarationOwner().ProcessingDeclarations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	grouped, ok := declarations.(FieldGroupDeclarations)
	if !ok {
		t.Fatal("synthetic declarations do not declare field groups")
	}
	want := map[string]string{"protocol": "payload", "form": "payload", "subtype": "payload",
		"observedAt": "gateway", "network.id": "gateway", "network.name": "gateway", "payload.text.value": "payload"}
	for _, ref := range SyntheticSourceDeclarationRefs() {
		groups, err := grouped.FieldGroups(ref)
		if err != nil || len(groups) != 1 {
			t.Fatalf("%s groups = %v, %v", ref, groups, err)
		}
		if expected, ok := want[strings.Join(ref.Path, ".")]; ok && groups[0] != expected {
			t.Fatalf("%s group = %s, want %s", ref, groups[0], expected)
		}
	}
	if _, err := grouped.FieldGroups(FieldRef{Namespace: "source", Path: []string{"undeclared"}}); err == nil {
		t.Fatal("undeclared field reported a group")
	}
	protocol := FieldRef{Namespace: "source", Path: []string{"protocol"}}
	name := FieldRef{Namespace: "source", Path: []string{"network", "name"}}
	value := Value{StringValue, json.RawMessage(`"pocsag"`)}
	program, err := CompileWorkflow(t.Context(), Definition{SchemaVersion: 1,
		Selector:     Predicate{Op: "eq", Field: &protocol, Value: &value},
		OutputSchema: map[string]FieldSchema{"network": {Kind: StringValue}},
		Outputs:      map[string]Assignment{"network": {Field: &name}}}, declarations, nil)
	if err != nil {
		t.Fatal(err)
	}
	references := program.SourceReferences()
	if len(references) != 2 || references[0].Field.String() != "source.network.name" ||
		!slices.Equal(references[0].Groups, []string{"gateway"}) || references[1].Field.String() != "source.protocol" ||
		!slices.Equal(references[1].Groups, []string{"payload"}) {
		t.Fatalf("selector and output references were not classified: %+v", references)
	}
}
