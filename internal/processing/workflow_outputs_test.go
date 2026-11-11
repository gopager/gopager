package processing

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

func TestWorkflowOutputsAreTypedBySchemaAndRejectUndeclaredTargets(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t, "111")
	text := FieldRef{"source", []string{"text"}}
	literal, err := Literal(StringValue, "x")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]Definition{
		"type mismatch": withOutput("t", "number", Assignment{Field: &text}),
		"unknown schema": func() Definition {
			definition := selectedDefinition()
			definition.Outputs = map[string]map[string]Assignment{"unknown": {"text": {Literal: &literal}}}
			return definition
		}(),
		"unknown field": withOutput("t", "undeclared", Assignment{Literal: &literal}),
		"no input":      withOutput("t", "text", Assignment{}),
	}
	for name, definition := range cases {
		if _, err := CompileWorkflow(t.Context(), definition, source, testOutputs, nil); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s: compile = %v, want INVALID_INPUT", name, err)
		}
	}
	if _, err := CompileWorkflow(t.Context(), withOutput("t", "text", Assignment{Field: &text}), source, nil, nil); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Errorf("outputs without declarations compiled: %v", err)
	}
	if _, err := CompileWorkflow(t.Context(), withOutput("t", "text", Assignment{Field: &text}), source, testOutputs, nil); err != nil {
		t.Errorf("typed output rejected: %v", err)
	}
}

func TestWorkflowOutputsAreBoundedByCollectionLimit(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t)
	literal, err := Literal(StringValue, "x")
	if err != nil {
		t.Fatal(err)
	}
	definition := selectedDefinition()
	definition.Outputs = map[string]map[string]Assignment{"many": {}}
	for i := range MaxCollection {
		definition.Outputs["many"][fmt.Sprintf("f%d", i)] = Assignment{Literal: &literal}
	}
	if _, err := CompileWorkflow(t.Context(), definition, source, testOutputs, nil); err != nil {
		t.Fatalf("outputs at the limit rejected: %v", err)
	}
	definition.Outputs["many"]["extra"] = Assignment{Literal: &literal}
	if _, err := CompileWorkflow(t.Context(), definition, source, testOutputs, nil); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("outputs above the limit: %v", err)
	}
}

func TestWorkflowAbsentOutputIsOmittedAndExplicitNullIsKept(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t)
	absent := FieldRef{"target", []string{"address"}}
	null := Value{Kind: NullValue, Value: json.RawMessage(`null`)}
	definition := withOutput("t", "address", Assignment{Field: &absent})
	program, err := CompileWorkflow(t.Context(), definition, source, testOutputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(t.Context(), source)
	encoded, encodeErr := json.Marshal(result)
	if err != nil || encodeErr != nil || string(encoded) != `{"selected":true,"fields":{}}` {
		t.Fatalf("absent output stored: %s, %v, %v", encoded, err, encodeErr)
	}

	definition = withOutput("t", "nullable", Assignment{Literal: &null})
	program, err = CompileWorkflow(t.Context(), definition, source, testOutputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err = program.Evaluate(t.Context(), source)
	field, present := result.Fields["t"]["nullable"]
	if err != nil || !present || field.State != FieldPresent || field.Value == nil || field.Value.Kind != NullValue {
		t.Fatalf("explicit null dropped: %+v, %v", result, err)
	}
}

func TestValidateEvaluationChecksResultsAgainstOutputDeclarations(t *testing.T) {
	t.Parallel()
	binding := EvaluationBinding{TransmissionID: uuid.New(), SourceRevision: 1, ContextRevision: 1}
	text, err := Literal(StringValue, "x")
	if err != nil {
		t.Fatal(err)
	}
	present := FieldValue{State: FieldPresent, Value: &text}
	valid := Evaluation{Selected: true, Fields: map[string]map[string]FieldValue{"t": {"text": present}}}
	if err := ValidateEvaluation(valid, testOutputs, binding); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
	for name, fields := range map[string]map[string]map[string]FieldValue{
		"unknown schema": {"unknown": {"text": present}},
		"unknown field":  {"t": {"undeclared": present}},
		"wrong type":     {"t": {"number": present}},
		"absent value":   {"t": {"text": {State: FieldAbsent}}},
		"empty schema":   {"t": {}},
	} {
		if err := ValidateEvaluation(Evaluation{Selected: true, Fields: fields}, testOutputs, binding); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := ValidateEvaluation(valid, nil, binding); err == nil {
		t.Errorf("result without declarations accepted: %v", err)
	}
}

// withOutput returns the selected definition with one output assignment.
func withOutput(schema, field string, assignment Assignment) Definition {
	definition := selectedDefinition()
	definition.Outputs = map[string]map[string]Assignment{schema: {field: assignment}}
	return definition
}
