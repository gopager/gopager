package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
)

func TestGatewayBearerRequiresOneValidAuthorizationHeader(t *testing.T) {
	secret, _, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		headers []string
		valid   bool
	}{
		{"valid", []string{"Bearer " + secret}, true},
		{"wrong scheme", []string{"Basic " + secret}, false},
		{"multiple", []string{"Bearer " + secret, "Bearer " + secret}, false},
		{"extra secret token", []string{"Bearer " + secret + " other"}, false},
		{"missing", nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/api/v1/transmissions", nil)
			for _, value := range test.headers {
				request.Header.Add("Authorization", value)
			}
			_, err := gatewayBearer(request)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t err=%v", test.valid, err)
			}
		})
	}
}

func TestIngestHTTPSubmissionAndGatewayStateRoutes(t *testing.T) {
	fixture := newGatewayHTTPFixture(t)
	issuePath := "/api/v1/gateways/" + fixture.gateway.String() + "/credentials"
	issueBody := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","expectedRevision":"1"}`
	issuedBody := fixture.request(t, http.MethodPost, issuePath, issueBody, fixture.csrf, "", http.StatusCreated)
	var credential gatewayCredentialResponse
	if err := json.Unmarshal(issuedBody, &credential); err != nil || credential.Secret == "" {
		t.Fatalf("credential issue response = %s, error=%v", issuedBody, err)
	}
	bearer := "Bearer " + credential.Secret
	submissionPath := "/api/v1/transmissions"
	submission := `{"schemaVersion":1,"submissionId":"http-submission","protocol":"pocsag",` +
		`"observationTime":"2026-11-04T12:00:00Z","gatewayNetworkIdentifier":"current",` +
		`"payload":{"form":"tone-only"},"transmissionTargets":[]}`

	fixture.request(t, http.MethodPost, submissionPath, submission, "", "", http.StatusUnauthorized)
	fixture.request(t, http.MethodPost, submissionPath, submission, "", "Bearer invalid", http.StatusUnauthorized)
	malformed := fixture.request(t, http.MethodPost, submissionPath, `{"schemaVersion":1`, "", bearer,
		http.StatusBadRequest)
	var malformedResponse struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(malformed, &malformedResponse); err != nil || malformedResponse.Code != "INVALID_INPUT" {
		t.Fatalf("malformed Submission response = %s, error=%v", malformed, err)
	}
	accepted := fixture.request(t, http.MethodPost, submissionPath, submission, "", bearer, http.StatusOK)
	var receiptResponse struct {
		Data ingest.Receipt `json:"data"`
	}
	if err := json.Unmarshal(accepted, &receiptResponse); err != nil || receiptResponse.Data.TransmissionID == uuid.Nil ||
		receiptResponse.Data.ProcessingStatus != "none" || receiptResponse.Data.SubmissionID != "http-submission" {
		t.Fatalf("accepted Submission response = %s, error=%v", accepted, err)
	}
	retry := fixture.request(t, http.MethodPost, submissionPath, submission, "", bearer, http.StatusOK)
	var retried struct {
		Data ingest.Receipt `json:"data"`
	}
	if err := json.Unmarshal(retry, &retried); err != nil ||
		retried.Data.TransmissionID != receiptResponse.Data.TransmissionID ||
		retried.Data.MessageID != receiptResponse.Data.MessageID {
		t.Fatalf("Submission retry response = %s, error=%v", retry, err)
	}
	changed := strings.Replace(submission, `"transmissionTargets":[]`, `"transmissionTargets":[],"incomplete":true`, 1)
	conflict := fixture.request(t, http.MethodPost, submissionPath, changed, "", bearer, http.StatusConflict)
	var conflictResponse struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(conflict, &conflictResponse); err != nil ||
		conflictResponse.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("changed Submission retry response = %s, error=%v", conflict, err)
	}

	statePath := "/api/v1/gateway/state"
	fixture.request(t, http.MethodPost, statePath, `{"schemaVersion":1,"operation":"heartbeat"}`,
		"", "", http.StatusUnauthorized)
	fixture.request(t, http.MethodPost, statePath, `{"schemaVersion":1,"operation":"heartbeat"}`,
		"", "Bearer invalid", http.StatusUnauthorized)
	invalidState := fixture.request(t, http.MethodPost, statePath,
		`{"schemaVersion":1,"operation":"unknown"}`, "", bearer, http.StatusBadRequest)
	var invalidStateResponse struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(invalidState, &invalidStateResponse); err != nil ||
		invalidStateResponse.Code != "INVALID_INPUT" {
		t.Fatalf("invalid state response = %s, error=%v", invalidState, err)
	}
	for _, step := range []struct {
		report, state, operation, detail string
		changes                          int
	}{
		{`{"schemaVersion":1,"operation":"heartbeat"}`, "active", "heartbeat", "", 0},
		{`{"schemaVersion":1,"operation":"degraded","reasonCode":"signal-loss","detail":"first diagnostic"}`,
			"degraded", "degraded", "first diagnostic", 1},
		{`{"schemaVersion":1,"operation":"degraded","reasonCode":"signal-loss","detail":"updated diagnostic"}`,
			"degraded", "degraded", "updated diagnostic", 1},
		{`{"schemaVersion":1,"operation":"heartbeat"}`, "degraded", "heartbeat", "", 1},
		{`{"schemaVersion":1,"operation":"recovered"}`, "active", "recovered", "", 2},
		{`{"schemaVersion":1,"operation":"recovered"}`, "active", "recovered", "", 2},
		{`{"schemaVersion":1,"operation":"terminated"}`, "stopped", "terminated", "", 3},
	} {
		fixture.request(t, http.MethodPost, statePath, step.report, "", bearer, http.StatusNoContent)
		var state, operation, detail string
		var changes int
		var hasContact bool
		if err := fixture.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := fixture.store.Resolve(tx)
			if err != nil {
				return err
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT state,last_report->>'operation',
				COALESCE(last_report->>'detail',''),last_contact_at IS NOT NULL
				FROM gateways WHERE id=$1`, fixture.gateway).Scan(&state, &operation, &detail, &hasContact); err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events
				WHERE envelope->>'type'='gateway.state-changed'`).Scan(&changes)
		}); err != nil {
			t.Fatal(err)
		}
		if state != step.state || operation != step.operation || detail != step.detail ||
			!hasContact || changes != step.changes {
			t.Fatalf("Gateway state=%q report=%q detail=%q contact=%t state-changed facts=%d; want %+v",
				state, operation, detail, hasContact, changes, step)
		}
	}
}
