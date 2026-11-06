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
	t.Parallel()
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
	t.Parallel()
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

// TestResolveNetworkFollowsDeclarationSingleAddressAndPriority covers the five resolution cases of
// a Gateway with several Networks per protocol; priority 0 is the highest.
func TestResolveNetworkFollowsDeclarationSingleAddressAndPriority(t *testing.T) {
	t.Parallel()
	high, middle, low, tone := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	// Listed out of priority order on purpose: resolution must not depend on slice order.
	pocsag := []GatewayNetwork{
		{Identifier: "low", NetworkID: low, Protocol: protocols.POCSAG, Priority: 2},
		{Identifier: "high", NetworkID: high, Protocol: protocols.POCSAG, Priority: 0},
		{Identifier: "middle", NetworkID: middle, Protocol: protocols.POCSAG, Priority: 1},
	}
	alarm := GatewayNetwork{Identifier: "alarm", NetworkID: tone, Protocol: protocols.ZVEI, Priority: 3}
	mixed := append(append([]GatewayNetwork{}, pocsag...), alarm)
	tests := []struct {
		name     string
		protocol protocols.Protocol
		declared string
		assigned []GatewayNetwork
		matches  []uuid.UUID
		wantNet  uuid.UUID
		wantMode string
		wantErr  bool
	}{
		{name: "declared wins over matches", declared: "low", assigned: mixed, matches: []uuid.UUID{high},
			wantNet: low, wantMode: "declared"},
		{name: "unknown declaration", declared: "nope", assigned: mixed, wantErr: true},
		{name: "declaration of another protocol", declared: "alarm", assigned: mixed, wantErr: true},
		{name: "only Network of the protocol", protocol: protocols.ZVEI, assigned: mixed,
			wantNet: tone, wantMode: "inferred-single"},
		{name: "exactly one matching Network", assigned: mixed, matches: []uuid.UUID{middle},
			wantNet: middle, wantMode: "inferred-address"},
		{name: "match of another protocol is ignored", assigned: mixed, matches: []uuid.UUID{tone, low},
			wantNet: low, wantMode: "inferred-address"},
		{name: "several match, highest priority among them wins", assigned: mixed,
			matches: []uuid.UUID{high, low}, wantNet: high, wantMode: "address-priority"},
		{name: "several match without the highest-priority Network", assigned: mixed,
			matches: []uuid.UUID{low, middle}, wantNet: middle, wantMode: "address-priority"},
		{name: "none match picks the highest priority", assigned: mixed,
			wantNet: high, wantMode: "priority"},
		{name: "no Network of the protocol", protocol: protocols.FLEX, assigned: mixed, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			protocol := test.protocol
			if protocol == "" {
				protocol = protocols.POCSAG
			}
			input := ValidatedSubmission{Submission: Submission{Protocol: protocol,
				GatewayNetworkIdentifier: test.declared}}
			network, mode, err := ResolveNetwork(input, test.assigned, test.matches)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidSubmission) {
					t.Fatalf("want ErrInvalidSubmission, got network=%s mode=%s err=%v", network, mode, err)
				}
				return
			}
			if err != nil || network != test.wantNet || mode != test.wantMode {
				t.Fatalf("network=%s mode=%s err=%v, want %s %s", network, mode, err, test.wantNet, test.wantMode)
			}
		})
	}
}

func TestResolveNetworkRejectsDuplicateIdentifiers(t *testing.T) {
	t.Parallel()
	input := ValidatedSubmission{Submission: Submission{Protocol: protocols.POCSAG}}
	if _, _, err := ResolveNetwork(input, []GatewayNetwork{
		{Identifier: "a", NetworkID: uuid.New(), Protocol: protocols.POCSAG},
		{Identifier: "a", NetworkID: uuid.New(), Protocol: protocols.POCSAG, Priority: 1}}, nil); !errors.Is(err, ErrInvalidSubmission) {
		t.Fatalf("duplicate identifier error=%v", err)
	}
}
