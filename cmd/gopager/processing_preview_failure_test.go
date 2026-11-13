package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

// TestPreviewExplainsAFailedStepInsteadOfAnError proves that a failing step answers 200 with the
// failure code, its reason and the step ID, without raw error text, while a request error stays
// an HTTP error.
func TestPreviewExplainsAFailedStepInsteadOfAnError(t *testing.T) {
	t.Parallel()
	f := newPreviewHTTPFixture(t, []string{"unit", "address", "payload", "gateway", "derived"})
	definition := sourceProtocolPreviewDefinition(t)
	definition.Steps = []processing.Step{{ID: "extract-level", Kind: "extract",
		OutputSchema: map[string]processing.FieldSchema{"level": {Kind: processing.IntegerValue}},
		Extract: &processing.ExtractStep{
			Input:   processing.FieldRef{Namespace: "source", Path: []string{"payload", "text", "value"}},
			Pattern: `LEVEL=(?P<level>[0-9]+)`,
			Captures: map[string]processing.CaptureMapping{
				"level": {Name: "level", Conversion: processing.IntegerValue}}}}}
	version := f.saveVersion(definition)
	synthetic := func(text string) string {
		return `{"schemaVersion":1,"versionId":"` + version.String() + `","syntheticJson":{"schemaVersion":1,"protocol":"pocsag",` +
			`"form":"text","networkId":"` + f.network.ID.String() + `","payload":{"value":"` + text + `"},"targets":[]}}`
	}

	failed := f.preview(synthetic("LEVEL=99999999999999999999"), http.StatusOK)
	var body struct {
		Failure *struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
			StepID string `json:"stepId"`
		} `json:"failure"`
	}
	if err := json.Unmarshal(failed.Data, &body); err != nil || body.Failure == nil {
		t.Fatalf("preview = %s, %v; want a failure", failed.Data, err)
	}
	if body.Failure.Code != "INVALID_INPUT" || body.Failure.Reason != "conversion-failed" || body.Failure.StepID != "extract-level" {
		t.Fatalf("failure = %+v", body.Failure)
	}
	if strings.Contains(string(failed.Data), "99999999999999999999") {
		t.Fatalf("the failure carries raw input: %s", failed.Data)
	}

	succeeded := f.preview(synthetic("LEVEL=7"), http.StatusOK)
	var evaluation processing.Evaluation
	if err := json.Unmarshal(succeeded.Data, &evaluation); err != nil || !evaluation.Selected ||
		strings.Contains(string(succeeded.Data), `"failure"`) {
		t.Fatalf("a passing step reported %s, %v", succeeded.Data, err)
	}

	f.preview(`{"schemaVersion":1,"versionId":"`+version.String()+`"}`, http.StatusBadRequest)
}
