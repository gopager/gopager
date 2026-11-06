package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestWorkflowRetryResponseIsBounded(t *testing.T) {
	response := workflowRetryResponse{
		CandidateID: uuid.MustParse("b29f0e2e-f49d-4c69-90c9-a27e28f4c26c"),
		Revision:    3,
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"candidateId":"b29f0e2e-f49d-4c69-90c9-a27e28f4c26c","revision":"3"}`
	if string(encoded) != want {
		t.Fatalf("retry response = %s, want %s", encoded, want)
	}
}
