package processing

import (
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

func TestSourceContextClassificationSurvivesWorkflowDerivation(t *testing.T) {
	t.Parallel()
	projection, _ := sourceFixture(t)
	projection.Subtype = "alpha"
	contextual, err := projection.WithContextFields("Dispatch")
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Fields) != 2 || len(contextual.Fields) != 7 {
		t.Fatal("context builder changed its source fields")
	}
	source, err := CaptureSource(t.Context(), contextual)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path  []string
		value string
		group string
	}{
		{[]string{"protocol"}, string(projection.Protocol), "payload"},
		{[]string{"form"}, projection.Form, "payload"},
		{[]string{"subtype"}, "alpha", "payload"},
		{[]string{"network", "id"}, projection.NetworkID.String(), "gateway"},
		{[]string{"network", "name"}, "Dispatch", "gateway"},
	}
	for _, test := range tests {
		t.Run(test.path[len(test.path)-1], func(t *testing.T) {
			t.Parallel()
			reference := FieldRef{Namespace: "source", Path: test.path}
			definition := selectedDefinition()
			definition.OutputSchema["context"] = FieldSchema{Kind: StringValue}
			definition.Outputs["context"] = Assignment{Field: &reference}
			program, err := CompileWorkflow(t.Context(), definition, source, nil)
			if err != nil {
				t.Fatal(err)
			}
			evaluation, err := program.EvaluateScoped(t.Context(), source, EvaluationScope{SourceEligible: true}, nil)
			if err != nil || !evaluation.Selected {
				t.Fatalf("context evaluation: %+v err=%v", evaluation, err)
			}
			field := evaluation.Fields["context"]
			if field.State != FieldPresent || field.Value == nil {
				t.Fatalf("missing context value: %+v", field)
			}
			value, err := field.Value.Native(definition.OutputSchema["context"])
			dependencies, dependencyErr := evaluation.Dependencies()
			if err != nil || dependencyErr != nil || value != test.value || len(field.Dependencies) != 0 || len(dependencies) != 1 {
				t.Fatalf("context projection: %+v err=%v, %v", field, err, dependencyErr)
			}
			dependency := dependencies[0]
			groups := map[string]bool{}
			for _, group := range dependency.Groups {
				groups[group] = true
			}
			if !groups[test.group] || (test.group == "payload" && len(groups) != 1) ||
				(test.group == "gateway" && (len(groups) != 2 || !groups["payload"])) ||
				dependency.TransmissionID != projection.TransmissionID ||
				dependency.NetworkID != projection.NetworkID || dependency.SourceRevision != projection.SourceRevision ||
				dependency.ContextRevision != projection.ContextRevision || dependency.TargetPosition != nil {
				t.Fatalf("lost classified context: %+v", dependency)
			}
		})
	}
}

func TestSourceContextAbsenceAndInvalidOwnerInput(t *testing.T) {
	t.Parallel()
	projection, _ := sourceFixture(t)
	contextual, err := projection.WithContextFields("Dispatch")
	if err != nil {
		t.Fatal(err)
	}
	subtype := contextual.Fields[4]
	if subtype.Field.State != FieldAbsent || subtype.Field.Value != nil || !subtype.Schema.Optional ||
		subtype.Field.Dependencies[0].Groups[0] != "payload" {
		t.Fatalf("absent subtype lost classification: %+v", subtype)
	}
	if _, err := contextual.WithContextFields("other"); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("duplicate context overwritten: %v", err)
	}
	if _, err := projection.WithContextFields(string([]byte{0xff})); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("invalid Unicode silently replaced: %v", err)
	}
	if _, err := (MissingSourceOwner{}).Capture(t.Context(), nil, projection.TransmissionID); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("context fixture replaced missing real owner: %v", err)
	}
}
