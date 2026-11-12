package main

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
)

// workflowScopeJSON renders the JSON scope of a Workflow assigned to the Agencies without narrowing.
func workflowScopeJSON(agencies ...uuid.UUID) string {
	encoded, err := json.Marshal(processing.WorkflowScope{Agencies: agencies})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
