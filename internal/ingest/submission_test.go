package ingest

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/protocols"
)

func TestSubmissionValidationNormalizesObservationAndPreservesOriginal(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"submissionId":"op-1","protocol":"pocsag","observationTime":"2026-11-04T10:00:00.987654+01:00","payload":{"form":"text","value":" e\u0301 "},"transmissionTargets":[{"address":123,"function":1}],"incomplete":true}`)
	var submission Submission
	if err := json.Unmarshal(raw, &submission); err != nil {
		t.Fatal(err)
	}
	validated, err := submission.Validate(len(raw), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if validated.ObservedAt.Format(time.RFC3339) != "2026-11-04T10:00:00+01:00" ||
		string(validated.Payload) != `{"form":"text","value":" e\u0301 "}` ||
		validated.Identity.Form != "text" || string(validated.Identity.Value) != "é" ||
		!submission.IncompleteValue() {
		t.Fatalf("Submission was not normalized/preserved: %+v", validated)
	}
}

func TestSubmissionRejectsMissingNullAndInvalidFormData(t *testing.T) {
	tests := []string{
		`{"schemaVersion":1,"submissionId":"id","protocol":"pocsag","observationTime":"2026-11-04T10:00:00Z","payload":{"form":"text","value":"x"}}`,
		`{"schemaVersion":1,"submissionId":"id","protocol":"pocsag","observationTime":"2026-11-04T10:00:00Z","payload":null,"transmissionTargets":[]}`,
		`{"schemaVersion":1,"submissionId":"id","protocol":"pocsag","observationTime":"2026-11-04T10:00:00","payload":{"form":"text","value":"x"},"transmissionTargets":[]}`,
		`{"schemaVersion":1,"submissionId":"id","protocol":"pocsag","observationTime":"2026-11-04T10:00:00Z","payload":{"form":"text","value":"x","extra":true},"transmissionTargets":[]}`,
		`{"schemaVersion":1,"submissionId":"id","protocol":"pocsag","observationTime":"2026-11-04T10:00:00Z","payload":{"form":"text","value":"x"},"transmissionTargets":null}`,
	}
	for _, raw := range tests {
		var submission Submission
		err := json.Unmarshal([]byte(raw), &submission)
		if err == nil {
			_, err = submission.Validate(len(raw), time.Now())
		}
		if !errors.Is(err, ErrInvalidSubmission) {
			t.Fatalf("accepted malformed submission %s: %v", raw, err)
		}
	}
}

func TestResolveNetworkHonorsDeclarationSingleAndDefault(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	allowed := []GatewayNetwork{
		{Identifier: "first", NetworkID: first, Protocol: protocols.POCSAG},
		{Identifier: "second", NetworkID: second, Protocol: protocols.POCSAG, Default: true},
	}
	input := ValidatedSubmission{Submission: Submission{Protocol: protocols.POCSAG}}
	if network, mode, err := ResolveNetwork(input, allowed); err != nil ||
		network != second || mode != "default" {
		t.Fatalf("several Networks must resolve to the default network=%s mode=%s err=%v", network, mode, err)
	}
	if network, mode, err := ResolveNetwork(input, allowed[:1]); err != nil ||
		network != first || mode != "inferred-single" {
		t.Fatalf("single network=%s mode=%s err=%v", network, mode, err)
	}
	input.GatewayNetworkIdentifier = "first"
	if network, mode, err := ResolveNetwork(input, allowed); err != nil ||
		network != first || mode != "declared" {
		t.Fatalf("declared network=%s mode=%s err=%v", network, mode, err)
	}
	input.GatewayNetworkIdentifier = "unknown"
	if _, _, err := ResolveNetwork(input, allowed); !errors.Is(err, ErrInvalidSubmission) {
		t.Fatalf("unauthorized declaration error=%v", err)
	}
	input.GatewayNetworkIdentifier = ""
	if _, _, err := ResolveNetwork(input, []GatewayNetwork{allowed[0], {Identifier: "other", NetworkID: second, Protocol: protocols.POCSAG}}); !errors.Is(err, ErrInvalidSubmission) {
		t.Fatalf("several Networks without default error=%v", err)
	}
}
