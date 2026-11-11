package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

func TestProcessingInstanceArtifactFixturesLoadFromConfiguredRoot(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot(pluginFixtureRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	host := processing.NewPluginHost()
	defer func() {
		if err := host.Close(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	pin := processing.ArtifactPin{PluginID: "test-command", Version: "1", CapabilityID: "test", Digest: fixtureDigest(t)}
	for _, path := range []string{"instance-manifest.json", "instance-redaction-manifest.json"} {
		artifact, err := host.LoadInstalled(t.Context(), root, path, pin)
		if err != nil {
			t.Fatalf("load fixture %q: %v", path, err)
		}
		if err := artifact.Close(t.Context()); err != nil {
			t.Fatalf("close fixture %q: %v", path, err)
		}
	}
}

func TestProcessingWorkflowHTTPRequiresCurrentCSRFAndUsesSyntheticDeclarations(t *testing.T) {
	t.Parallel()
	// The saved definitions select on a payload field, which the author must be able to read.
	store := newManagementStoreWithFields(t, []string{"unit", "address", "payload"})
	agency := seedAgency(t, store)
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
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
	managementRequest(t, client, origin, http.MethodGet, "/api/v1/workflows", "", "", "******", http.StatusUnauthorized)
	managementRequest(t, client, origin, http.MethodGet, "/api/v1/instances?limit=1",
		"", "", "", http.StatusOK)
	operatorID := managementOperatorID(t, store)
	enabled := true
	emptyConfiguration, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
	if err != nil {
		t.Fatal(err)
	}
	instanceInput, err := json.Marshal(instanceMutation{
		SchemaVersion: 1, CommandID: uuid.New(),
		InstanceInput: processingmanagement.InstanceInput{
			Enabled: &enabled, ExecutionPrincipalID: operatorID,
			Main: processing.ProvisionedArtifact{
				ManifestPath: "instance-manifest.json",
				Pin: processing.ArtifactPin{PluginID: "test-command", Version: "1",
					CapabilityID: "test", Digest: fixtureDigest(t)},
			},
			Configuration: emptyConfiguration,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	unavailable := managementRequest(t, client, origin, http.MethodPost, "/api/v1/instances",
		string(instanceInput), login.CSRFToken, "", http.StatusServiceUnavailable)
	if unavailable.Code != string(platform.CodeAuthorityUnavailable) {
		t.Fatalf("omitted artifact root failure = %q, want authority unavailable", unavailable.Code)
	}
	body := `{"schemaVersion":1,"commandId":"` + "6d881468-4f69-4d81-a08a-95ea30bb1653" +
		`","name":"Rules","scope":` + workflowScopeJSON(agency) + `,"enabled":true}`
	managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows", body, "", "", http.StatusForbidden)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, origin+"/api/v1/workflows", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://foreign.example")
	request.Header.Set(identity.CSRFHeaderName, login.CSRFToken)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusForbidden || closeErr != nil {
		t.Fatalf("cross-origin Workflow mutation status = %d, close error = %v; want forbidden", response.StatusCode, closeErr)
	}

	created := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows", body,
		login.CSRFToken, "", http.StatusCreated)
	id, revision := managementResultID(t, created)
	managementRequest(t, client, origin, http.MethodGet, "/api/v1/instances?limit=1",
		"", "", "", http.StatusOK)
	managementRequest(t, client, origin, http.MethodGet, "/api/v1/workflows?limit=1", "", "", "", http.StatusOK)
	managementRequest(t, client, origin, http.MethodGet, "/api/v1/workflows/"+id.String(), "", "", "", http.StatusOK)
	managementRequest(t, client, origin, http.MethodPut, "/api/v1/workflows/"+id.String(),
		`{"schemaVersion":1,"expectedRevision":"`+revision+`","name":"Rules updated","scope":`+workflowScopeJSON(agency)+`,"enabled":true}`,
		login.CSRFToken, "", http.StatusOK)
	managementRequest(t, client, origin, http.MethodGet, "/api/v1/workflows/"+id.String()+"/versions",
		"", "", "", http.StatusOK)
	field := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	expectedProtocol, err := processing.Literal(processing.StringValue, "pocsag")
	if err != nil {
		t.Fatal(err)
	}
	definition := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &field, Value: &expectedProtocol},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
	versionInput, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1,
		CommandID: uuid.New(), ExpectedRevision: 2, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	saved := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows/"+id.String()+"/versions",
		string(versionInput), login.CSRFToken, "", http.StatusCreated)
	versionID, revision := managementResultID(t, saved)
	if revision != "3" {
		t.Fatalf("version save Workflow revision = %s, want 3", revision)
	}
	activated := managementRequest(t, client, origin, http.MethodPost,
		"/api/v1/workflows/"+id.String()+"/versions/"+versionID.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"3"}`, login.CSRFToken, "", http.StatusOK)
	_, revision = managementResultID(t, activated)
	if revision != "4" {
		t.Fatalf("activation Workflow revision = %s, want 4", revision)
	}
}
