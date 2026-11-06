package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
)

// TestProcessingSaveAndDraftPreviewRejectUnreadableFieldGroups pins D-135 save-time
// validation over HTTP: an author who may read only the gateway group can save and preview a
// definition that reads gateway fields, but a selector or output that references a payload
// field is rejected with INVALID_INPUT, both on save and on the preview of an unsaved draft.
func TestProcessingSaveAndDraftPreviewRejectUnreadableFieldGroups(t *testing.T) {
	store := newManagementStoreWithFields(t, []string{"gateway"})
	identityRepository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	networks := previewFixtureNetworks{
		values: make(map[uuid.UUID]synthetic.Network), identity: identityRepository, units: unitRepository,
	}
	networkID := uuid.New()
	networks.values[networkID] = synthetic.Network{ID: networkID, Name: "Standalone", Protocol: protocols.POCSAG}
	shared := mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), shared, "", networks)
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
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, networks, runtime.routes)
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
	workflow := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Field Validation","scope":{"kind":"all"},"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowID, _ := managementResultID(t, workflow)

	gatewayField := processing.FieldRef{Namespace: "source", Path: []string{"network", "name"}}
	payloadField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	name, err := processing.Literal(processing.StringValue, "Standalone")
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := processing.Literal(processing.StringValue, "pocsag")
	if err != nil {
		t.Fatal(err)
	}
	readable := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &gatewayField, Value: &name},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
	bySelector := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &payloadField, Value: &protocol},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
	byOutput := readable
	byOutput.OutputSchema = map[string]processing.FieldSchema{"protocol": {Kind: processing.StringValue}}
	byOutput.Outputs = map[string]processing.Assignment{"protocol": {Field: &payloadField}}
	byBranch := readable
	byBranch.Steps = []processing.Step{{ID: "pick", Kind: "branch",
		OutputSchema: map[string]processing.FieldSchema{},
		Branch: &processing.BranchStep{Condition: bySelector.Selector,
			ThenOutputs: map[string]processing.Assignment{}, ElseOutputs: map[string]processing.Assignment{}}}}

	revision := int64(1)
	save := func(definition processing.Definition, status int) managementResponse {
		t.Helper()
		body, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1, CommandID: uuid.New(),
			ExpectedRevision: revision, Definition: definition})
		if err != nil {
			t.Fatal(err)
		}
		return managementRequest(t, client, origin, http.MethodPost,
			"/api/v1/workflows/"+workflowID.String()+"/versions", string(body), login.CSRFToken, "", status)
	}
	for name, definition := range map[string]processing.Definition{
		"selector": bySelector, "output": byOutput, "branch condition": byBranch,
	} {
		rejected := save(definition, http.StatusBadRequest)
		if rejected.Code != "INVALID_INPUT" {
			t.Fatalf("%s reference to an unreadable group returned %q, want INVALID_INPUT", name, rejected.Code)
		}
	}
	saved := save(readable, http.StatusCreated)
	if _, savedRevision := managementResultID(t, saved); savedRevision != "2" {
		t.Fatalf("readable definition revision = %s, want 2 (rejected saves must not advance it)", savedRevision)
	}

	observedAt := "2026-11-03T14:00:00Z"
	input := syntheticPreviewInputs(networks.values)[0]
	input.ObservedAt = &observedAt
	encodedInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for name, definition := range map[string]processing.Definition{"selector": bySelector, "output": byOutput} {
		encoded, err := json.Marshal(definition)
		if err != nil {
			t.Fatal(err)
		}
		rejected := managementRequest(t, client, origin, http.MethodPost,
			"/api/v1/workflows/"+workflowID.String()+"/preview",
			`{"schemaVersion":1,"definition":`+string(encoded)+`,"syntheticJson":`+string(encodedInput)+`}`,
			login.CSRFToken, "", http.StatusBadRequest)
		if rejected.Code != "INVALID_INPUT" {
			t.Fatalf("draft preview with an unreadable %s reference returned %q, want INVALID_INPUT", name, rejected.Code)
		}
	}
}
