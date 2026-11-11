package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
)

func TestProcessingSyntheticPreviewHTTPUsesCurrentFixtureNetworks(t *testing.T) {
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
	agency := seedAgency(t, store)
	networks := previewFixtureNetworks{
		values: make(map[uuid.UUID]synthetic.Network), identity: identityRepository, units: unitRepository,
	}
	for _, protocol := range []protocols.Protocol{
		protocols.POCSAG, protocols.FLEX, protocols.ZVEI, protocols.EAS,
	} {
		id := uuid.New()
		networks.values[id] = synthetic.Network{ID: id, Name: "Standalone " + string(protocol), Protocol: protocol, AgencyID: agency}
	}
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
	workflow := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Synthetic Preview","scope":`+workflowScopeJSON(agency)+`,"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowID, _ := managementResultID(t, workflow)
	definition := sourceProtocolPreviewDefinition(t)
	versionInput, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1,
		CommandID: uuid.New(), ExpectedRevision: 1, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	saved := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions", string(versionInput),
		login.CSRFToken, "", http.StatusCreated)
	versionID, revision := managementResultID(t, saved)
	if revision != "2" {
		t.Fatalf("version save revision = %s, want 2", revision)
	}
	managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/versions/"+versionID.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusOK)

	before := processingPreviewSideEffects(t, store)
	inputs := syntheticPreviewInputs(networks.values)
	observedAt := "2026-11-03T14:00:00Z"
	for i := range inputs {
		inputs[i].ObservedAt = &observedAt
	}
	transportInput, err := json.Marshal(inputs[0])
	if err != nil {
		t.Fatal(err)
	}
	transportBody := `{"schemaVersion":1,"versionId":"` + versionID.String() +
		`","syntheticJson":` + string(transportInput) + `}`
	previewPath := "/api/v1/workflows/" + workflowID.String() + "/preview"
	managementRequest(t, client, origin, http.MethodPost, previewPath, transportBody,
		"", "", http.StatusForbidden)
	managementRequest(t, client, origin, http.MethodPost, previewPath, transportBody,
		login.CSRFToken, "invalid", http.StatusUnauthorized)
	wrongOrigin, err := http.NewRequest(http.MethodPost, origin+previewPath, strings.NewReader(transportBody))
	if err != nil {
		t.Fatal(err)
	}
	wrongOrigin.Header.Set("Content-Type", "application/json")
	wrongOrigin.Header.Set("Origin", "https://other.invalid")
	wrongOrigin.Header.Set(identity.CSRFHeaderName, login.CSRFToken)
	originResponse, err := client.Do(wrongOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if err := originResponse.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if originResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin preview returned %d", originResponse.StatusCode)
	}
	for _, input := range inputs {
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		response := managementRequest(t, client, origin, http.MethodPost,
			"/api/v1/workflows/"+workflowID.String()+"/preview",
			`{"schemaVersion":1,"versionId":"`+versionID.String()+`","syntheticJson":`+
				string(encoded)+`}`, login.CSRFToken, "", http.StatusOK)
		var evaluation processing.Evaluation
		if err := json.Unmarshal(response.Data, &evaluation); err != nil || !evaluation.Selected {
			t.Fatalf("%s/%s synthetic preview result = %+v, decode error = %v",
				input.Protocol, input.Form, evaluation, err)
		}
	}
	binaryInput := inputs[6]
	binaryInputJSON, err := json.Marshal(binaryInput)
	if err != nil {
		t.Fatal(err)
	}
	binaryDefinition := binarySourcePreviewDefinition(t)
	binaryDefinitionJSON, err := json.Marshal(binaryDefinition)
	if err != nil {
		t.Fatal(err)
	}
	draft := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/preview",
		`{"schemaVersion":1,"definition":`+string(binaryDefinitionJSON)+`,"syntheticJson":`+
			string(binaryInputJSON)+`}`, login.CSRFToken, "", http.StatusOK)
	var binaryEvaluation processing.Evaluation
	if err := json.Unmarshal(draft.Data, &binaryEvaluation); err != nil || !binaryEvaluation.Selected {
		t.Fatalf("typed bytes draft preview = %+v, decode error = %v", binaryEvaluation, err)
	}
	revokeManagementAction(t, store, managementOperatorID(t, store), "workflow.preview")
	revokedInput, err := json.Marshal(inputs[0])
	if err != nil {
		t.Fatal(err)
	}
	managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/preview",
		`{"schemaVersion":1,"versionId":"`+versionID.String()+`","syntheticJson":`+
			string(revokedInput)+`}`, login.CSRFToken, "", http.StatusForbidden)
	after := processingPreviewSideEffects(t, store)
	if before != after {
		t.Fatalf("synthetic previews wrote durable results/candidates/events: before=%v after=%v", before, after)
	}
}

func TestProcessingSyntheticPreviewHonorsCurrentUnitScope(t *testing.T) {
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
	shared := mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), shared, pluginFixtureRoot(t), networks)
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
	unitResponse := managementRequest(t, client, origin, http.MethodPost, "/api/v1/units",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","agencyId":"`+agency.String()+
			`","name":"Matched Unit","collectionIds":[],"organizationId":null,"settings":{}}`,
		login.CSRFToken, "", http.StatusCreated)
	unitID, _ := managementResultID(t, unitResponse)
	managementRequest(t, client, origin, http.MethodPost, "/api/v1/address-rules",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","networkId":"`+networkID.String()+
			`","unitId":"`+unitID.String()+`","protocol":"pocsag","selector":{"from":100,"to":200,`+
			`"mode":"function-bit","functions":[1]},"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowResponse := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Unit Preview",`+
			`"scope":{"agencies":["`+agency.String()+`"],"narrowing":{"kind":"unit","references":["`+
			unitID.String()+`"]}},"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowID, _ := managementResultID(t, workflowResponse)
	definition := unitTargetPreviewDefinition(t, unitID)
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

	target := protocols.Target{Address: protocols.NumericAddress(123)}
	function := uint8(1)
	target.Function = &function
	input := synthetic.Input{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "text",
		NetworkID: networkID, Payload: json.RawMessage(`{"value":"unit scoped"}`),
		Targets: []protocols.Target{target}}
	observedAt := "2026-11-03T14:00:00Z"
	input.ObservedAt = &observedAt
	encodedInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	response := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/preview",
		`{"schemaVersion":1,"versionId":"`+versionID.String()+`","syntheticJson":`+
			string(encodedInput)+`}`, login.CSRFToken, "", http.StatusOK)
	var evaluation processing.Evaluation
	if err := json.Unmarshal(response.Data, &evaluation); err != nil || !evaluation.Selected ||
		len(evaluation.TargetResults) != 1 ||
		!strings.Contains(string(response.Data), unitID.String()) {
		t.Fatalf("matched Unit target was not eligible in preview: %+v, %s, error=%v",
			evaluation, response.Data, err)
	}

	unmatched := input
	unmatchedTarget := target
	otherFunction := uint8(2)
	unmatchedTarget.Function = &otherFunction
	unmatched.Targets = []protocols.Target{unmatchedTarget}
	encodedUnmatched, err := json.Marshal(unmatched)
	if err != nil {
		t.Fatal(err)
	}
	unmatchedResponse := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+workflowID.String()+"/preview",
		`{"schemaVersion":1,"versionId":"`+versionID.String()+`","syntheticJson":`+
			string(encodedUnmatched)+`}`, login.CSRFToken, "", http.StatusOK)
	var ineligible processing.Evaluation
	if err := json.Unmarshal(unmatchedResponse.Data, &ineligible); err != nil || ineligible.Selected ||
		len(ineligible.TargetResults) != 0 {
		t.Fatalf("unmatched target widened Unit-scoped preview: %+v, %s, error=%v",
			ineligible, unmatchedResponse.Data, err)
	}
}
