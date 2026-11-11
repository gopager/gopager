package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
)

// TestInstalledWorkflowRunsOnProductionWiring submits a Transmission that selects a Workflow
// with an installed step and runs its candidate through the production source owner,
// artifact loader and publisher: the run completes inside the worker's own transactions,
// publishes its result and marks the Transmission completed.
func TestInstalledWorkflowRunsOnProductionWiring(t *testing.T) {
	t.Parallel()
	store := newManagementStoreWithFields(t, []string{"unit", "address", "payload", "gateway", "derived"})
	network, gateway, agency := uuid.New(), uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES($2,'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'Installed Network','pocsag',a.id FROM a`, network, agency); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id) VALUES($1)`, gateway); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,priority) VALUES($1,$2,'installed',$3,0)`,
			uuid.New(), gateway, network)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	artifactRoot, mainArtifact, _ := installedPreviewArtifacts(t)
	shared := mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), shared, artifactRoot)
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

	operatorID := managementOperatorID(t, store)
	label, err := processing.Literal(processing.StringValue, "run")
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{"label": label})
	if err != nil {
		t.Fatal(err)
	}
	instance := processing.InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: true,
		ExecutionPrincipalID: operatorID, Main: mainArtifact, Configuration: configuration,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond), CreatedBy: operatorID}
	seedCurrentInstance(t, store, instance)

	workflowResponse := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Installed run","scope":`+workflowScopeJSON(agency)+`,"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowID, _ := managementResultID(t, workflowResponse)
	protocolField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	pocsag, err := processing.Literal(processing.StringValue, "pocsag")
	if err != nil {
		t.Fatal(err)
	}
	textField := processing.FieldRef{Namespace: "source", Path: []string{"payload", "text", "value"}}
	definition := processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocolField, Value: &pocsag},
		Steps: []processing.Step{{
			ID: "command", Kind: "installed", OutputSchema: map[string]processing.FieldSchema{"text": {Kind: processing.StringValue}},
			Installed: &processing.InstalledStep{InstanceID: instance.ID, ConfigurationRevision: 1,
				Input: map[string]processing.Assignment{"text": {Field: &textField}}},
		}},
		OutputSchema: map[string]processing.FieldSchema{"text": {Kind: processing.StringValue}},
		Outputs: map[string]processing.Assignment{"text": {Field: &processing.FieldRef{
			Namespace: "step", Path: []string{"command", "text"},
		}}}}
	versionInput, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1,
		CommandID: uuid.New(), ExpectedRevision: 1, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	versionResponse := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions", string(versionInput),
		login.CSRFToken, "", http.StatusCreated)
	versionID, _ := managementResultID(t, versionResponse)
	managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions/"+versionID.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusOK)

	var credential gatewayCredentialResponse
	rawProcessingRequest(t, client, origin, "/api/v1/gateways/"+gateway.String()+"/credentials",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"1"}`,
		identity.CSRFHeaderName, login.CSRFToken, http.StatusCreated, &credential)
	var submitted struct {
		Data struct {
			TransmissionID   uuid.UUID `json:"transmissionId"`
			MessageID        uuid.UUID `json:"messageId"`
			ProcessingStatus string    `json:"processingStatus"`
		} `json:"data"`
	}
	rawProcessingRequest(t, client, origin, "/api/v1/transmissions",
		`{"schemaVersion":1,"submissionId":"installed-run","protocol":"pocsag",`+
			`"observationTime":"2026-11-04T12:00:00Z","gatewayNetworkIdentifier":"installed",`+
			`"payload":{"form":"text","value":"installed run"},"transmissionTargets":[]}`,
		"Authorization", "Bearer "+credential.Secret, http.StatusAccepted, &submitted)
	receipt := submitted.Data
	if receipt.ProcessingStatus != "pending" {
		t.Fatalf("receipt = %+v, want pending Processing", receipt)
	}

	repository, err := processingstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	service, err := worker.NewWithPublisher(store, repository, runtime.workerSource, processing.CoreRegex{},
		uuid.New(), runtime.workerPublisher)
	if err != nil {
		t.Fatal(err)
	}
	var due []processing.Candidate
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		page, err := repository.SelectDueWork(tx.Context(), tx, 10)
		due = page
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].TransmissionID != receipt.TransmissionID {
		t.Fatalf("due work = %+v, want the submitted Transmission's candidate", due)
	}
	if err := service.ExecuteWork(t.Context(), due[0]); err != nil {
		t.Fatalf("installed-step run failed on production wiring: %s", causeChain(err))
	}
	var state, status string
	var published int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT c.state,t.processing_status,
			(SELECT count(*) FROM processing_results r WHERE r.candidate_id=c.id AND r.published_at IS NOT NULL
				AND r.message_id=t.message_id)
			FROM processing_candidates c JOIN transmissions t ON t.id=c.transmission_id WHERE c.id=$1`, due[0].ID).
			Scan(&state, &status, &published)
	}); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || status != "completed" || published != 1 {
		t.Fatalf("candidate=%s transmission=%s published results=%d; want completed/completed/1", state, status, published)
	}
	var groups string
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT field_groups::text FROM processing_results WHERE candidate_id=$1`,
			due[0].ID).Scan(&groups)
	}); err != nil {
		t.Fatal(err)
	}
	if groups != `["derived", "payload"]` {
		t.Fatalf("stored field groups = %s, want the definition-level classification", groups)
	}
}

// causeChain renders every wrapped layer of err, including joined errors, so a classified
// failure also shows its cause.
func causeChain(err error) string {
	if err == nil {
		return ""
	}
	layer := fmt.Sprintf("%T: %v", err, err)
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		parts := make([]string, 0, len(joined.Unwrap()))
		for _, inner := range joined.Unwrap() {
			parts = append(parts, causeChain(inner))
		}
		return layer + " <- [" + strings.Join(parts, " | ") + "]"
	}
	if inner := errors.Unwrap(err); inner != nil {
		return layer + " <- " + causeChain(inner)
	}
	return layer
}
