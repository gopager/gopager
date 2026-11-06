package processing

import "testing"

func TestInstalledPreviewDoesNotRunLiveBehaviour(t *testing.T) {
	owner := newInstalledOwnerFixture(t)
	owner.previewMode = true
	_, source := sourceFixture(t)
	// The fixture's live "fail" behaviour must stay unreachable in preview mode.
	definition := installedDefinition(owner, "fail", FieldRef{Namespace: "source", Path: []string{"text"}})
	program, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.EvaluateInstalled(t.Context(), source, owner)
	if err != nil || result.Fields["text"].Value == nil ||
		string(result.Fields["text"].Value.Value) != `"PREVIEW:LEVEL=7"` {
		t.Fatalf("preview mode ran the live behaviour: %+v %v", result, err)
	}
}
