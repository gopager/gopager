package processing

import "testing"

func TestInstalledDedicatedPreviewDoesNotInvokeFailingMainCommand(t *testing.T) {
	owner := newInstalledOwnerFixture(t)
	owner.artifact.manifest.PurePreview = false
	supplyInstalledPreviewFixture(t, owner)
	owner.previewMode = true
	_, source := sourceFixture(t)
	definition := installedDefinition(owner, "fail", FieldRef{Namespace: "source", Path: []string{"text"}})
	program, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.EvaluateInstalled(t.Context(), source, owner)
	if err != nil || result.Fields["text"].Value == nil ||
		string(result.Fields["text"].Value.Value) != `"PREVIEW:LEVEL=7"` {
		t.Fatalf("dedicated preview invoked or substituted failing main: %+v %v", result, err)
	}
}
