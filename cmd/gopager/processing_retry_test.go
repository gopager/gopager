package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

// failProcessingCandidateTerminally moves a pending candidate to the terminal failed state
// after exhausting its automatic attempts, as the worker does for a final failure.
func failProcessingCandidateTerminally(t *testing.T, store *postgres.Store, candidateID uuid.UUID) {
	t.Helper()
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{"ProcessingCandidate": entql.FieldEQ("id", candidateID)}})
		if err := client.ProcessingCandidate.UpdateOneID(candidateID).AddRevision(1).
			SetState(processingcandidate.StateFailed).SetAttempts(processing.MaxAutomaticAttempts).
			SetAutomaticAttempts(processing.MaxAutomaticAttempts).
			SetFailureCode(string(platform.CodeResourceLimit)).Exec(scope); err != nil {
			return err
		}
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE transmissions SET processing_status='failed'
			WHERE id=(SELECT transmission_id FROM processing_candidates WHERE id=$1)`, candidateID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// rawProcessingRequest posts body with one authentication header and decodes the JSON
// response into result.
func rawProcessingRequest(t *testing.T, client *http.Client, origin, path, body, header, value string,
	expected int, result any) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, origin+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	request.Header.Set(header, value)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	decodeErr := json.NewDecoder(response.Body).Decode(result)
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected || decodeErr != nil {
		t.Fatalf("POST %s returned %d (want %d), decode error = %v", path, response.StatusCode, expected, decodeErr)
	}
}

func TestProcessingCandidateRetryHTTPUsesCurrentSourceOwner(t *testing.T) {
	// The saved definitions select on a payload field, which the author must be able to read.
	store := newManagementStoreWithFields(t, []string{"unit", "address", "payload"})
	network, gateway := uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'Retry Network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id) VALUES($1)`, gateway); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,priority) VALUES($1,$2,'retry',$3,0)`,
			uuid.New(), gateway, network)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	shared := mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), shared, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, nil, runtime.routes)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	client := backend.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	login := managementRequest(t, client, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)

	workflowResponse := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Retry","scope":{"kind":"all"},"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowID, _ := managementResultID(t, workflowResponse)
	versionInput, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1,
		CommandID: uuid.New(), ExpectedRevision: 1, Definition: sourceProtocolPreviewDefinition(t)})
	if err != nil {
		t.Fatal(err)
	}
	versionResponse := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions", string(versionInput),
		login.CSRFToken, "", http.StatusCreated)
	versionID, _ := managementResultID(t, versionResponse)
	activated := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions/"+versionID.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusOK)
	_, workflowRevision := managementResultID(t, activated)

	var credential gatewayCredentialResponse
	rawProcessingRequest(t, client, origin, "/api/v1/gateways/"+gateway.String()+"/credentials",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"1"}`,
		identity.CSRFHeaderName, login.CSRFToken, http.StatusCreated, &credential)
	if credential.Secret == "" {
		t.Fatal("credential issue returned no secret")
	}
	var submitted struct {
		Data struct {
			TransmissionID   uuid.UUID `json:"transmissionId"`
			ProcessingStatus string    `json:"processingStatus"`
		} `json:"data"`
	}
	rawProcessingRequest(t, client, origin, "/api/v1/transmissions",
		`{"schemaVersion":1,"submissionId":"retry-submission","protocol":"pocsag",`+
			`"observationTime":"2026-11-04T12:00:00Z","gatewayNetworkIdentifier":"retry",`+
			`"payload":{"form":"tone-only"},"transmissionTargets":[]}`,
		"Authorization", "Bearer "+credential.Secret, http.StatusAccepted, &submitted)
	receipt := submitted.Data
	if receipt.TransmissionID == uuid.Nil || receipt.ProcessingStatus != "pending" {
		t.Fatalf("accepted Transmission receipt = %+v, want pending Processing", receipt)
	}
	var candidateID uuid.UUID
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT id FROM processing_candidates
			WHERE transmission_id=$1 AND workflow_id=$2`, receipt.TransmissionID, workflowID).Scan(&candidateID)
	}); err != nil {
		t.Fatal(err)
	}
	failProcessingCandidateTerminally(t, store, candidateID)

	candidatePath := "/api/v1/workflows/" + workflowID.String() + "/candidates/" + candidateID.String()
	readCandidate := func() (string, string) {
		t.Helper()
		response := managementRequest(t, client, origin, http.MethodGet, candidatePath, "", "", "", http.StatusOK)
		var status struct {
			State    string `json:"state"`
			Revision string `json:"revision"`
		}
		if err := json.Unmarshal(response.Data, &status); err != nil {
			t.Fatal(err)
		}
		return status.State, status.Revision
	}
	state, revision := readCandidate()
	if state != "failed" {
		t.Fatalf("seeded candidate state = %q, want failed", state)
	}
	retried := managementRequest(t, client, origin, http.MethodPost, candidatePath+"/retry",
		`{"schemaVersion":1,"expectedRevision":"`+revision+`"}`, login.CSRFToken, "", http.StatusOK)
	var retryResult map[string]json.RawMessage
	if err := json.Unmarshal(retried.Data, &retryResult); err != nil {
		t.Fatal(err)
	}
	if _, nested := retryResult["schemaVersion"]; nested || string(retryResult["candidateId"]) != `"`+candidateID.String()+`"` {
		t.Fatalf("retry response data = %s", retried.Data)
	}
	var retriedState, transmissionStatus string
	var retriedRevision int64
	var runnable, manual bool
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT c.state,c.revision,c.due_at<=now() AND c.worker_id IS NULL,
			c.manual_retry_pending,t.processing_status FROM processing_candidates c
			JOIN transmissions t ON t.id=c.transmission_id WHERE c.id=$1`, candidateID).
			Scan(&retriedState, &retriedRevision, &runnable, &manual, &transmissionStatus)
	}); err != nil {
		t.Fatal(err)
	}
	if retriedState != "pending" || strconv.FormatInt(retriedRevision, 10) == revision || !runnable || !manual ||
		transmissionStatus != "pending" {
		t.Fatalf("retried candidate state=%s revision=%d runnable=%v manualRetryPending=%v transmission=%s, want runnable pending manual retry with a pending Transmission",
			retriedState, retriedRevision, runnable, manual, transmissionStatus)
	}

	failProcessingCandidateTerminally(t, store, candidateID)
	_, failedRevision := readCandidate()
	managementRequest(t, client, origin, http.MethodPut, "/api/v1/workflows/"+workflowID.String(),
		`{"schemaVersion":1,"expectedRevision":"`+workflowRevision+`","name":"Retry renamed","scope":{"kind":"all"},"enabled":true}`,
		login.CSRFToken, "", http.StatusOK)
	stale := managementRequest(t, client, origin, http.MethodPost, candidatePath+"/retry",
		`{"schemaVersion":1,"expectedRevision":"`+failedRevision+`"}`, login.CSRFToken, "", http.StatusConflict)
	if stale.Code != string(platform.CodeRevisionConflict) {
		t.Fatalf("retry after Workflow change error = %q, want revision conflict", stale.Code)
	}
	if state, _ := readCandidate(); state != "failed" {
		t.Fatalf("stale retry changed candidate state to %q", state)
	}
}
