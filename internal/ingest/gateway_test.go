package ingest

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestGatewayStateReportValidatesClosedOperationFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"heartbeat", `{"schemaVersion":1,"operation":"heartbeat"}`, true},
		{"degraded", `{"schemaVersion":1,"operation":"degraded","reasonCode":"receiver/locked","detail":"antenna offline"}`, true},
		{"duplicate", `{"schemaVersion":1,"operation":"heartbeat","operation":"recovered"}`, false},
		{"unknown", `{"schemaVersion":1,"operation":"heartbeat","state":"active"}`, false},
		{"null diagnostic", `{"schemaVersion":1,"operation":"degraded","reasonCode":null}`, false},
		{"missing reason", `{"schemaVersion":1,"operation":"degraded"}`, false},
		{"diagnostics on heartbeat", `{"schemaVersion":1,"operation":"heartbeat","reasonCode":"ok"}`, false},
		{"non-ascii reason", `{"schemaVersion":1,"operation":"degraded","reasonCode":"é"}`, false},
		{"trailing document", `{"schemaVersion":1,"operation":"heartbeat"}{}`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var report GatewayStateReport
			err := json.Unmarshal([]byte(test.input), &report)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t err=%v", test.valid, err)
			}
			if err != nil && !errors.Is(err, ErrInvalidGatewayStateReport) && test.name != "trailing document" {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSubmissionRejectsDuplicateTopLevelFields(t *testing.T) {
	var submission Submission
	err := json.Unmarshal([]byte(`{"schemaVersion":1,"schemaVersion":1,"submissionId":"id",
		"protocol":"pocsag","observationTime":"2026-01-01T00:00:00Z","payload":{"form":"tone-only"},
		"transmissionTargets":[]}`), &submission)
	if !errors.Is(err, ErrInvalidSubmission) {
		t.Fatalf("duplicate top-level member accepted: %v", err)
	}
}
