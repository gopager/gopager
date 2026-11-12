package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/testutil/attributeschemas"
)

func TestProcessingSyntheticPreviewUsesProductionNetworkOwner(t *testing.T) {
	t.Parallel()
	store := newManagementStoreWithFields(t,
		[]string{"unit", "address", "payload", "gateway", "derived"})
	live, deleted, agency := uuid.New(), uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES($3,'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,deleted,agency_id) SELECT v.id,v.name,v.protocol,v.deleted,a.id FROM (VALUES ($1::uuid,'Live POCSAG','pocsag',false),($2::uuid,'Deleted POCSAG','pocsag',true)) AS v(id,name,protocol,deleted), a`, live, deleted, agency)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	shared := mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), shared, pluginFixtureRoot(t))
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
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Production Network",`+
			`"scope":`+workflowScopeJSON(agency)+`,"enabled":true}`,
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
	managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions/"+versionID.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusOK)
	before := processingPreviewSideEffects(t, store)
	preview := func(protocol protocols.Protocol, network uuid.UUID, expected int) managementResponse {
		t.Helper()
		input := synthetic.Input{SchemaVersion: 1, Protocol: protocol, Form: "text",
			NetworkID: network, Payload: json.RawMessage(`{"value":"production network"}`),
			Targets: []protocols.Target{}}
		encodedInput, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return managementRequest(t, client, origin, http.MethodPost,
			"/api/v1/workflows/"+workflowID.String()+"/preview",
			`{"schemaVersion":1,"versionId":"`+versionID.String()+`","syntheticJson":`+
				string(encodedInput)+`}`, login.CSRFToken, "", expected)
	}
	response := preview(protocols.POCSAG, live, http.StatusOK)
	var evaluation processing.Evaluation
	if err := json.Unmarshal(response.Data, &evaluation); err != nil || !evaluation.Selected {
		t.Fatalf("production Network preview = %+v, decode error = %v", evaluation, err)
	}
	for name, failure := range map[string]managementResponse{
		"deleted Network":   preview(protocols.POCSAG, deleted, http.StatusServiceUnavailable),
		"Protocol mismatch": preview(protocols.FLEX, live, http.StatusServiceUnavailable),
	} {
		if failure.Code != string(platform.CodeAuthorityUnavailable) {
			t.Fatalf("%s preview error = %q, want authority unavailable", name, failure.Code)
		}
	}
	if missing := preview(protocols.POCSAG, uuid.New(), http.StatusNotFound); missing.Code != "NOT_FOUND" {
		t.Fatalf("unknown Network preview error = %q, want not found", missing.Code)
	}
	if after := processingPreviewSideEffects(t, store); before != after {
		t.Fatalf("production Network preview wrote durable results/candidates/events: before=%v after=%v", before, after)
	}
}

// TestProcessingSyntheticPreviewUsesOnlyPreviewSupportingInstalledPlugins checks that an editor preview
// reaches an installed plugin in preview mode only when its manifest declares supportsPreview.
func TestProcessingSyntheticPreviewUsesOnlyPreviewSupportingInstalledPlugins(t *testing.T) {
	t.Parallel()
	store := newManagementStoreWithFields(t,
		[]string{"unit", "address", "payload", "gateway", "derived"})
	identityRepository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	networkID := uuid.New()
	agency := seedAgency(t, store)
	networks := previewFixtureNetworks{
		values: map[uuid.UUID]synthetic.Network{
			networkID: {ID: networkID, Name: "Standalone POCSAG", Protocol: protocols.POCSAG, AgencyID: agency},
		},
		identity: identityRepository,
		units:    unitRepository,
	}
	artifactRoot, supportingArtifact, unsupportingArtifact := installedPreviewArtifacts(t)
	shared := mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), shared, artifactRoot, networks)
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
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared,
		networks, runtime.routes)
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
	enabled := true
	label, err := processing.Literal(processing.StringValue, "preview")
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{"label": label})
	if err != nil {
		t.Fatal(err)
	}
	inspector, err := processing.NewLocalInstanceInspector(runtime.host, runtime.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := inspector.InspectInstance(t.Context(), processing.InstanceConfiguration{
		ID: uuid.New(), Revision: 1, Enabled: true, ExecutionPrincipalID: operatorID,
		Main: supportingArtifact, Configuration: configuration,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond), CreatedBy: operatorID,
	}); err != nil {
		t.Fatalf("preview fixture inspection failed: %v", err)
	}
	createInstance := func(artifact processing.ProvisionedArtifact) uuid.UUID {
		t.Helper()
		body, err := json.Marshal(instanceMutation{SchemaVersion: 1, CommandID: uuid.New(),
			InstanceInput: processingmanagement.InstanceInput{Enabled: &enabled,
				ExecutionPrincipalID: operatorID, Main: artifact,
				Configuration: configuration}})
		if err != nil {
			t.Fatal(err)
		}
		created := managementRequest(t, client, origin, http.MethodPost, "/api/v1/instances",
			string(body), login.CSRFToken, "", http.StatusCreated)
		id, revision := managementResultID(t, created)
		if revision != "1" {
			t.Fatalf("created instance revision = %s, want 1", revision)
		}
		return id
	}
	instanceID := createInstance(supportingArtifact)
	withoutPreviewID := createInstance(unsupportingArtifact)
	workflowResponse := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Installed Preview",`+
			`"scope":`+workflowScopeJSON(agency)+`,"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowID, _ := managementResultID(t, workflowResponse)
	protocolField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	pocsag, err := processing.Literal(processing.StringValue, "pocsag")
	if err != nil {
		t.Fatal(err)
	}
	textField := processing.FieldRef{Namespace: "source", Path: []string{"payload", "text", "value"}}
	attributeschemas.Insert(t, store, "plugin", map[string]attributes.Field{"text": {Kind: processing.StringValue}})
	definition := processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocolField, Value: &pocsag},
		Steps: []processing.Step{{
			ID: "command", Kind: "installed", OutputSchema: map[string]processing.FieldSchema{"text": {Kind: processing.StringValue}},
			Installed: &processing.InstalledStep{InstanceID: instanceID, ConfigurationRevision: 1,
				Input: map[string]processing.Assignment{"text": {Field: &textField}}},
		}},
		Outputs: map[string]map[string]processing.Assignment{"plugin": {"text": {Field: &processing.FieldRef{
			Namespace: "step", Path: []string{"command", "text"},
		}}}}}
	syntheticInput := synthetic.Input{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "text",
		NetworkID: networkID, Payload: json.RawMessage(`{"value":"installed preview"}`),
		Targets: []protocols.Target{}}
	observedAt := "2026-11-03T14:00:00Z"
	syntheticInput.ObservedAt = &observedAt
	encodedInput, err := json.Marshal(syntheticInput)
	if err != nil {
		t.Fatal(err)
	}
	bodyFor := func(instance uuid.UUID) string {
		definition.Steps[0].Installed.InstanceID = instance
		encodedDefinition, err := json.Marshal(definition)
		if err != nil {
			t.Fatal(err)
		}
		return `{"schemaVersion":1,"definition":` + string(encodedDefinition) +
			`,"syntheticJson":` + string(encodedInput) + `}`
	}
	savedBody, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1,
		CommandID: uuid.New(), ExpectedRevision: 1, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	saved := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions", string(savedBody),
		login.CSRFToken, "", http.StatusCreated)
	savedID, _ := managementResultID(t, saved)
	managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions/"+savedID.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusOK)
	before := processingPreviewSideEffects(t, store)
	pureDefinition := sourceProtocolPreviewDefinition(t)
	pureBody, err := json.Marshal(workflowPreviewMutation{SchemaVersion: 1,
		Definition: &pureDefinition, SyntheticJSON: encodedInput})
	if err != nil {
		t.Fatal(err)
	}
	pureResponse := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/preview", string(pureBody),
		login.CSRFToken, "", http.StatusOK)
	var pureEvaluation processing.Evaluation
	if err := json.Unmarshal(pureResponse.Data, &pureEvaluation); err != nil || !pureEvaluation.Selected {
		t.Fatalf("pure synthetic preview failed before installed call: %+v, %s, error=%v",
			pureEvaluation, pureResponse.Data, err)
	}
	response := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/preview", bodyFor(instanceID),
		login.CSRFToken, "", http.StatusOK)
	var evaluation processing.Evaluation
	if err := json.Unmarshal(response.Data, &evaluation); err != nil || !evaluation.Selected {
		t.Fatalf("installed preview failed: %+v, %s, error=%v", evaluation, response.Data, err)
	}
	if evaluation.Fields["plugin"]["text"].Value == nil ||
		string(evaluation.Fields["plugin"]["text"].Value.Value) != `"PREVIEW:installed preview"` {
		t.Fatalf("preview mode did not reach the plugin: %s", response.Data)
	}
	missing := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/preview", bodyFor(withoutPreviewID),
		login.CSRFToken, "", http.StatusServiceUnavailable)
	if missing.Code != string(platform.CodeAuthorityUnavailable) {
		t.Fatalf("plugin without preview support returned %q, want authority unavailable", missing.Code)
	}
	if after := processingPreviewSideEffects(t, store); before != after {
		t.Fatalf("installed previews wrote durable results/candidates/events: before=%v after=%v", before, after)
	}
}
