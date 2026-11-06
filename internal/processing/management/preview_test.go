package management

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
)

func TestPreviewInputRequiresOneDefinitionAndOneSource(t *testing.T) {
	t.Parallel()
	workflowID := uuid.New()
	definition := processing.Definition{}
	for _, input := range []PreviewInput{
		{WorkflowID: workflowID, Definition: &definition},
		{WorkflowID: workflowID, Definition: &definition, SyntheticJSON: []byte(`{}`), StoredTransmissionID: uuid.New()},
		{WorkflowID: workflowID, VersionID: uuid.New()},
		{WorkflowID: workflowID, SyntheticJSON: []byte(`{}`)},
		{WorkflowID: workflowID, VersionID: uuid.New(), Definition: &definition, SyntheticJSON: []byte(`{}`)},
		{Definition: &definition, SyntheticJSON: []byte(`{}`)},
		{WorkflowID: workflowID, Definition: &definition, SyntheticJSON: []byte(`not json`)},
		{WorkflowID: workflowID, Definition: &definition, SyntheticJSON: make([]byte, processing.MaxInputBytes+1)},
	} {
		if _, err := input.validate(); err == nil {
			t.Fatalf("invalid preview input accepted: %+v", input)
		}
	}
	input := PreviewInput{WorkflowID: workflowID, Definition: &definition, SyntheticJSON: []byte(`{}`)}
	if mode, err := input.validate(); err != nil || mode != previewSynthetic {
		t.Fatalf("valid synthetic preview rejected: %v, %v", mode, err)
	}
	for _, input := range []PreviewInput{
		{WorkflowID: workflowID, Definition: &definition, StoredTransmissionID: uuid.New()},
		{WorkflowID: workflowID, VersionID: uuid.New(), SyntheticJSON: []byte(`{}`)},
		{WorkflowID: workflowID, VersionID: uuid.New(), StoredTransmissionID: uuid.New()},
	} {
		if mode, err := input.validate(); err != nil || mode == 0 {
			t.Fatalf("valid preview rejected: %v, %v", mode, err)
		}
	}
}
