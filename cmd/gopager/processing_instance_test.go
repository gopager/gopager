package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

func TestProcessingInstanceHTTPUsesRootAndRedactsCurrentConfiguration(t *testing.T) {
	t.Parallel()
	store := newManagementStore(t)
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
	operatorID := managementOperatorID(t, store)
	enabled := true
	configuration := func(fields map[string]string) processing.Value {
		t.Helper()
		typedFields := make(map[string]processing.Value, len(fields))
		for name, value := range fields {
			typed, err := processing.Literal(processing.StringValue, value)
			if err != nil {
				t.Fatal(err)
			}
			typedFields[name] = typed
		}
		result, err := processing.Literal(processing.ObjectValue, typedFields)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	makeMutation := func(command uuid.UUID, expected int64, path string, value processing.Value) string {
		encoded, err := json.Marshal(instanceMutation{
			SchemaVersion: 1, CommandID: command, ExpectedRevision: expected,
			InstanceInput: processingmanagement.InstanceInput{
				Enabled: &enabled, ExecutionPrincipalID: operatorID,
				Main: processing.ProvisionedArtifact{
					ManifestPath: path,
					Pin: processing.ArtifactPin{PluginID: "test-command", Version: "1",
						CapabilityID: "test", Digest: fixtureDigest(t)},
				},
				Configuration: value,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	createBody := makeMutation(uuid.New(), 0, "instance-manifest.json", configuration(map[string]string{"label": "first"}))
	var decodedMutation instanceMutation
	decoder := json.NewDecoder(strings.NewReader(createBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decodedMutation); err != nil {
		t.Fatalf("instance request could not be decoded: %v; body=%s", err, createBody)
	}
	if decodedMutation.SchemaVersion != 1 || decodedMutation.CommandID == uuid.Nil ||
		decodedMutation.ExecutionPrincipalID != operatorID ||
		decodedMutation.Configuration.Kind != processing.ObjectValue ||
		decodedMutation.ExpectedRevision != 0 {
		t.Fatalf("instance request decoded incorrectly: %+v; body=%s", decodedMutation, createBody)
	}
	inputBytes, err := json.Marshal(decodedMutation.InstanceInput)
	if err != nil {
		t.Fatal(err)
	}
	var detachedInput processingmanagement.InstanceInput
	if err := processing.Decode(inputBytes, &detachedInput); err != nil {
		t.Fatalf("instance input failed service detachment: %v; input=%s", err, inputBytes)
	}
	created := managementRequest(t, client, origin, http.MethodPost, "/api/v1/instances",
		createBody, login.CSRFToken, "", http.StatusCreated)
	instanceID, revision := managementResultID(t, created)
	if revision != "1" {
		t.Fatalf("created revision = %s, want 1", revision)
	}

	managementRequest(t, client, origin, http.MethodPut, "/api/v1/instances/"+instanceID.String(),
		makeMutation(uuid.New(), 1, "instance-manifest.json", configuration(map[string]string{"label": "current"})),
		login.CSRFToken, "", http.StatusOK)

	page := managementRequest(t, client, origin, http.MethodGet, "/api/v1/instances?limit=10",
		"", "", "", http.StatusOK)
	var summaries struct {
		Items []struct {
			ID       uuid.UUID `json:"id"`
			Revision string    `json:"revision"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(page.Data, &summaries); err != nil || summaries.Total != 1 ||
		len(summaries.Items) != 1 || summaries.Items[0].ID != instanceID || summaries.Items[0].Revision != "2" {
		t.Fatalf("instance page did not return only its current pointer: %+v, %v", summaries, err)
	}
	if strings.Contains(string(page.Data), "manifestPath") || strings.Contains(string(page.Data), "digest") {
		t.Fatalf("instance summary leaked local artifact identity: %s", page.Data)
	}
	detail := managementRequest(t, client, origin, http.MethodGet, "/api/v1/instances/"+instanceID.String(),
		"", "", "", http.StatusOK)
	if !strings.Contains(string(detail.Data), `"current"`) || strings.Contains(string(detail.Data), "manifestPath") ||
		strings.Contains(string(detail.Data), "digest") || strings.Contains(string(detail.Data), "createdBy") {
		t.Fatalf("instance detail was not safely projected: %s", detail.Data)
	}

	secretValue := configuration(map[string]string{
		"label": "visible", "secret": "secret-value", "personal": "personal-value",
	})
	seededID := uuid.New()
	seedCurrentInstance(t, store, processing.InstanceConfiguration{
		ID: seededID, Revision: 1, Enabled: true, ExecutionPrincipalID: operatorID,
		Main: processing.ProvisionedArtifact{
			ManifestPath: "instance-redaction-manifest.json",
			Pin: processing.ArtifactPin{PluginID: "test-command", Version: "1",
				CapabilityID: "test", Digest: fixtureDigest(t)},
		},
		Configuration: secretValue, CreatedAt: time.Now().UTC().Truncate(time.Microsecond), CreatedBy: operatorID,
	})
	redacted := managementRequest(t, client, origin, http.MethodGet, "/api/v1/instances/"+seededID.String(),
		"", "", "", http.StatusOK)
	if strings.Contains(string(redacted.Data), "secret-value") || strings.Contains(string(redacted.Data), "personal-value") {
		t.Fatalf("redacted configuration values leaked: %s", redacted.Data)
	}
	for _, classification := range []string{`"secret":true`, `"scope":"user"`} {
		if !strings.Contains(string(redacted.Data), classification) {
			t.Fatalf("classified configuration status %s missing from %s", classification, redacted.Data)
		}
	}

	pathFailure := managementRequest(t, client, origin, http.MethodPost, "/api/v1/instances",
		makeMutation(uuid.New(), 0, "../outside.json", configuration(map[string]string{"label": "path"})),
		login.CSRFToken, "", http.StatusServiceUnavailable)
	if pathFailure.Code != string(platform.CodeAuthorityUnavailable) {
		t.Fatalf("artifact traversal failure = %q, want authority unavailable", pathFailure.Code)
	}

	_, username, password := createReadReviewer(t, store)
	reviewer := backend.Client()
	reviewerJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	reviewer.Jar = reviewerJar
	managementRequest(t, reviewer, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"`+username+`","password":"`+password+`"}`, "", "", http.StatusOK)
	managementRequest(t, reviewer, origin, http.MethodGet, "/api/v1/instances", "", "", "", http.StatusForbidden)
}
