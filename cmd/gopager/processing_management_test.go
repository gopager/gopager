package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

type previewFixtureNetworks struct {
	values   map[uuid.UUID]synthetic.Network
	identity *identitystore.Store
	units    *unitstore.Store
}

func (owner previewFixtureNetworks) ReadSyntheticNetwork(ctx context.Context, _ platform.TransactionContext,
	id uuid.UUID) (synthetic.Network, error) {
	if err := ctx.Err(); err != nil {
		return synthetic.Network{}, err
	}
	network, exists := owner.values[id]
	if !exists {
		return synthetic.Network{}, authz.ErrNotFound
	}
	return network, nil
}

func (owner previewFixtureNetworks) ReadNetwork(ctx context.Context, tx platform.TransactionContext,
	id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	network, err := owner.ReadSyntheticNetwork(ctx, tx, id)
	if err != nil {
		return "", uuid.Nil, err
	}
	return network.Protocol, network.AgencyID, nil
}

func TestWorkflowRetryResponseIsBounded(t *testing.T) {
	response := workflowRetryResponse{
		CandidateID: uuid.MustParse("b29f0e2e-f49d-4c69-90c9-a27e28f4c26c"),
		Revision:    3,
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"candidateId":"b29f0e2e-f49d-4c69-90c9-a27e28f4c26c","revision":"3"}`
	if string(encoded) != want {
		t.Fatalf("retry response = %s, want %s", encoded, want)
	}
}

// fixtureDigest is the SHA-256 of the command plugin fixture built for this test process.
func fixtureDigest(t *testing.T) string {
	t.Helper()
	sum := sha256.Sum256(pluginfixture.Binary(t, "command"))
	return hex.EncodeToString(sum[:])
}

// pluginFixtureRoot provisions the built command plugin and the testdata manifests, pinned to it.
func pluginFixtureRoot(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "plugin"), pluginfixture.Binary(t, "command"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"instance-manifest.json", "instance-redaction-manifest.json"} {
		raw, err := os.ReadFile("../../internal/processing/testdata/" + path)
		if err != nil {
			t.Fatal(err)
		}
		var manifest processing.InstalledManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.Artifact.Digest = fixtureDigest(t)
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, path), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func TestProcessingInstanceArtifactFixturesLoadFromConfiguredRoot(t *testing.T) {
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
	for _, secureProxy := range []bool{false, true} {
		name := "direct-http"
		if secureProxy {
			name = "https-reverse-proxy"
		}
		t.Run(name, func(t *testing.T) {
			// The saved definitions select on a payload field, which the author must be able to read.
			store := newManagementStoreWithFields(t, []string{"unit", "address", "payload"})
			backend := httptest.NewUnstartedServer(nil)
			t.Cleanup(backend.Close)
			origin := "http://" + backend.Listener.Addr().String()
			var proxy *httptest.Server
			if secureProxy {
				target, err := url.Parse(origin)
				if err != nil {
					t.Fatal(err)
				}
				proxy = httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
				t.Cleanup(proxy.Close)
				origin = "https://" + proxy.Listener.Addr().String()
			}
			runtime, err := composeProcessingAPIRuntime(t.Context(), store, "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := runtime.Close(t.Context()); err != nil {
					t.Error(err)
				}
			})
			handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store, nil, runtime.routes)
			if err != nil {
				t.Fatal(err)
			}
			backend.Config.Handler = handler
			backend.Start()
			client := backend.Client()
			if secureProxy {
				proxy.StartTLS()
				client = proxy.Client()
			}
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
				`","name":"Rules","scope":{"kind":"all"},"enabled":true}`
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
				`{"schemaVersion":1,"expectedRevision":"`+revision+`","name":"Rules updated","scope":{"kind":"all"},"enabled":true}`,
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
		})
	}
}

func TestProcessingSyntheticPreviewHTTPUsesCurrentFixtureNetworks(t *testing.T) {
	for _, secureProxy := range []bool{false, true} {
		name := "direct-http"
		if secureProxy {
			name = "https-reverse-proxy"
		}
		t.Run(name, func(t *testing.T) {
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
			networks := previewFixtureNetworks{
				values: make(map[uuid.UUID]synthetic.Network), identity: identityRepository, units: unitRepository,
			}
			for _, protocol := range []protocols.Protocol{
				protocols.POCSAG, protocols.FLEX, protocols.ZVEI, protocols.EAS,
			} {
				id := uuid.New()
				networks.values[id] = synthetic.Network{ID: id, Name: "Standalone " + string(protocol), Protocol: protocol}
			}
			runtime, err := composeProcessingAPIRuntime(t.Context(), store, "", networks)
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
			var proxy *httptest.Server
			if secureProxy {
				target, err := url.Parse(origin)
				if err != nil {
					t.Fatal(err)
				}
				proxy = httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
				t.Cleanup(proxy.Close)
				origin = "https://" + proxy.Listener.Addr().String()
			}
			handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store,
				networks, runtime.routes)
			if err != nil {
				t.Fatal(err)
			}
			backend.Config.Handler = handler
			backend.Start()
			client := backend.Client()
			if secureProxy {
				proxy.StartTLS()
				client = proxy.Client()
			}
			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			client.Jar = jar
			login := managementRequest(t, client, origin, http.MethodPost, "/api/v1/auth/login",
				`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)
			workflow := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
				`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Synthetic Preview","scope":{"kind":"all"},"enabled":true}`,
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
			revokeManagementAction(t, store, managementOperatorID(t, store), "rule.preview")
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
		})
	}
}

func TestProcessingSyntheticPreviewHonorsCurrentUnitScope(t *testing.T) {
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
	networks := previewFixtureNetworks{
		values: map[uuid.UUID]synthetic.Network{
			networkID: {ID: networkID, Name: "Standalone POCSAG", Protocol: protocols.POCSAG},
		},
		identity: identityRepository,
		units:    unitRepository,
	}
	runtime, err := composeProcessingAPIRuntime(t.Context(), store, pluginFixtureRoot(t), networks)
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
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store,
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
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","agencyId":"`+seedAgency(t, store).String()+
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
			`"scope":{"kind":"unit","references":["`+unitID.String()+`"]},"enabled":true}`,
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

func TestProcessingSyntheticPreviewUsesProductionNetworkOwner(t *testing.T) {
	store := newManagementStoreWithFields(t,
		[]string{"unit", "address", "payload", "gateway", "derived"})
	live, deleted := uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,deleted,agency_id) SELECT v.id,v.name,v.protocol,v.deleted,a.id FROM (VALUES ($1::uuid,'Live POCSAG','pocsag',false),($2::uuid,'Deleted POCSAG','pocsag',true)) AS v(id,name,protocol,deleted), a`, live, deleted)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := composeProcessingAPIRuntime(t.Context(), store, pluginFixtureRoot(t))
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
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store, nil, runtime.routes)
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
			`"scope":{"kind":"all"},"enabled":true}`,
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
	networks := previewFixtureNetworks{
		values: map[uuid.UUID]synthetic.Network{
			networkID: {ID: networkID, Name: "Standalone POCSAG", Protocol: protocols.POCSAG},
		},
		identity: identityRepository,
		units:    unitRepository,
	}
	artifactRoot, supportingArtifact, unsupportingArtifact := installedPreviewArtifacts(t)
	runtime, err := composeProcessingAPIRuntime(t.Context(), store, artifactRoot, networks)
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
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store,
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
			`"scope":{"kind":"all"},"enabled":true}`,
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
			Installed: &processing.InstalledStep{InstanceID: instanceID, ConfigurationRevision: 1,
				Input: map[string]processing.Assignment{"text": {Field: &textField}}},
		}},
		OutputSchema: map[string]processing.FieldSchema{"text": {Kind: processing.StringValue}},
		Outputs: map[string]processing.Assignment{"text": {Field: &processing.FieldRef{
			Namespace: "step", Path: []string{"command", "text"},
		}}}}
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
	if evaluation.Fields["text"].Value == nil ||
		string(evaluation.Fields["text"].Value.Value) != `"PREVIEW:installed preview"` {
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

// installedPreviewArtifacts provisions the command plugin twice: once whose manifest declares
// supportsPreview and once whose manifest does not.
func installedPreviewArtifacts(t *testing.T) (string, processing.ProvisionedArtifact, processing.ProvisionedArtifact) {
	t.Helper()
	root := t.TempDir()
	raw, err := os.ReadFile("../../internal/processing/testdata/instance-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest processing.InstalledManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ResultSchema = map[string]processing.FieldSchema{"text": {Kind: processing.StringValue}}
	binary := pluginfixture.Binary(t, "command")
	digest := sha256.Sum256(binary)
	if err := os.WriteFile(filepath.Join(root, "plugin"), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	artifacts := make([]processing.ProvisionedArtifact, 2)
	for index, supports := range []bool{true, false} {
		manifest.Artifact = processing.ArtifactReference{Path: "plugin", Digest: hex.EncodeToString(digest[:])}
		manifest.SupportsPreview = supports
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		name := "supports-preview.json"
		if !supports {
			name = "no-preview.json"
		}
		if err := os.WriteFile(filepath.Join(root, name), encoded, 0600); err != nil {
			t.Fatal(err)
		}
		artifacts[index] = processing.ProvisionedArtifact{ManifestPath: name,
			Pin: processing.ArtifactPin{PluginID: manifest.PluginID, Version: manifest.Version,
				CapabilityID: manifest.CapabilityID, Digest: manifest.Artifact.Digest}}
	}
	return root, artifacts[0], artifacts[1]
}

func sourceProtocolPreviewDefinition(t *testing.T) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	arguments := make([]processing.Predicate, 0, 4)
	for _, protocol := range []protocols.Protocol{
		protocols.POCSAG, protocols.FLEX, protocols.ZVEI, protocols.EAS,
	} {
		expected, err := processing.Literal(processing.StringValue, string(protocol))
		if err != nil {
			t.Fatal(err)
		}
		arguments = append(arguments, processing.Predicate{
			Op: "eq", Field: &field, Value: &expected,
		})
	}
	return processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "or", Args: arguments},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
}

func unitTargetPreviewDefinition(t *testing.T, unitID uuid.UUID) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "target", Path: []string{"unit", "id"}}
	expected, err := processing.Literal(processing.StringValue, unitID.String())
	if err != nil {
		t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &field, Value: &expected},
		OutputSchema: map[string]processing.FieldSchema{"unit": {Kind: processing.StringValue, Optional: true}},
		Outputs:      map[string]processing.Assignment{"unit": {Field: &field}}}
}

func binarySourcePreviewDefinition(t *testing.T) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"payload", "binary"}}
	expected, err := processing.Literal(processing.BytesValue, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &field, Value: &expected},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
}

func syntheticPreviewInputs(networks map[uuid.UUID]synthetic.Network) []synthetic.Input {
	networkByProtocol := make(map[protocols.Protocol]uuid.UUID, len(networks))
	for id, network := range networks {
		networkByProtocol[network.Protocol] = id
	}
	pocsagTarget := protocols.Target{Address: protocols.NumericAddress(123)}
	function := uint8(1)
	pocsagTarget.Function = &function
	zveiTarget := protocols.Target{Address: protocols.TextAddress("12345")}
	easTarget := protocols.Target{Address: protocols.TextAddress("123456")}
	return []synthetic.Input{
		{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "text", NetworkID: networkByProtocol[protocols.POCSAG],
			Payload: json.RawMessage(`{"value":"sample text"}`), Targets: []protocols.Target{pocsagTarget}},
		{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "numeric", NetworkID: networkByProtocol[protocols.POCSAG],
			Payload: json.RawMessage(`{"value":"123"}`), Targets: []protocols.Target{pocsagTarget}},
		{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "tone-only", NetworkID: networkByProtocol[protocols.POCSAG],
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "text", NetworkID: networkByProtocol[protocols.FLEX],
			Payload: json.RawMessage(`{"value":"sample text"}`), Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "numeric", NetworkID: networkByProtocol[protocols.FLEX],
			Payload: json.RawMessage(`{"value":"123"}`), Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "tone-only", NetworkID: networkByProtocol[protocols.FLEX],
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "binary", Subtype: "secure",
			NetworkID: networkByProtocol[protocols.FLEX], Payload: json.RawMessage(`{"encoding":"base64","data":"AQID"}`),
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "flex-short-signal",
			NetworkID: networkByProtocol[protocols.FLEX], Payload: json.RawMessage(`{"sourceCode":3}`),
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.ZVEI, Form: "zvei-signal", NetworkID: networkByProtocol[protocols.ZVEI],
			Payload: json.RawMessage(`{"double_tone":null}`), Targets: []protocols.Target{zveiTarget}},
		{SchemaVersion: 1, Protocol: protocols.EAS, Form: "eas-header", NetworkID: networkByProtocol[protocols.EAS],
			Payload: json.RawMessage(`{"originator":"EAS","eventCode":"TOR","purgeTime":"0015","issueTime":"0010000"}`),
			Targets: []protocols.Target{easTarget}},
	}
}

func processingPreviewSideEffects(t *testing.T, store *postgres.Store) [3]int64 {
	t.Helper()
	var counts [3]int64
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM processing_candidates),
			(SELECT count(*) FROM processing_results),
			(SELECT count(*) FROM domain_events)`).
			Scan(&counts[0], &counts[1], &counts[2])
	})
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func revokeManagementAction(t *testing.T, store *postgres.Store, principalID uuid.UUID, action string) {
	t.Helper()
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"Grant":           entql.FieldEQ("subject_id", principalID),
				"IdentityControl": entql.FieldEQ("id", 1),
			},
		})
		grants, err := client.Grant.Query().Where(grant.SubjectID(principalID)).All(scope)
		if err != nil {
			return err
		}
		for _, record := range grants {
			actions := make([]string, 0, len(record.Actions))
			for _, current := range record.Actions {
				if current != action {
					actions = append(actions, current)
				}
			}
			if len(actions) == len(record.Actions) {
				continue
			}
			if err := client.Grant.UpdateOneID(record.ID).SetActions(actions).AddRevision(1).Exec(scope); err != nil {
				return err
			}
		}
		return client.IdentityControl.UpdateOneID(1).AddAccessRevision(1).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func managementOperatorID(t *testing.T, store *postgres.Store) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"Account": entql.FieldEQ("username_key", "operator"),
			},
		})
		row, err := client.Account.Query().Where(account.UsernameKey("operator")).Only(scope)
		if err != nil {
			return err
		}
		id = row.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedCurrentInstance(t *testing.T, store *postgres.Store, value processing.InstanceConfiguration) {
	t.Helper()
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Create: map[string]bool{
				"ProcessingInstance":        true,
				"ProcessingInstanceVersion": true,
			},
		})
		if err := client.ProcessingInstance.Create().SetID(value.ID).SetRevision(value.Revision).Exec(scope); err != nil {
			return err
		}
		return client.ProcessingInstanceVersion.Create().SetID(uuid.New()).SetInstanceID(value.ID).
			SetRevision(value.Revision).SetConfiguration(value).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessingInstanceHTTPUsesRootAndRedactsCurrentConfiguration(t *testing.T) {
	store := newManagementStore(t)
	runtime, err := composeProcessingAPIRuntime(t.Context(), store, pluginFixtureRoot(t))
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
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store, nil, runtime.routes)
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
	decodeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/instances", strings.NewReader(createBody))
	decodeRequest.Header.Set("Content-Type", "application/json")
	if err := decodeManagementJSON(decodeRequest, &decodedMutation); err != nil {
		t.Fatalf("instance request could not be decoded: %v; body=%s", err, createBody)
	}
	if decodedMutation.SchemaVersion != 1 || decodedMutation.CommandID == uuid.Nil ||
		decodedMutation.InstanceInput.ExecutionPrincipalID != operatorID ||
		decodedMutation.InstanceInput.Configuration.Kind != processing.ObjectValue ||
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

type processingReadFixture struct {
	candidateID    uuid.UUID
	transmissionID uuid.UUID
	resultID       uuid.UUID
}

func seedProcessingReadCandidate(t *testing.T, store *postgres.Store, workflowID uuid.UUID,
	state processingcandidate.State, selected *bool, failureCode string, at time.Time,
) processingReadFixture {
	t.Helper()
	fixture := processingReadFixture{candidateID: uuid.New(), transmissionID: uuid.New()}
	if state == processingcandidate.StateCompleted && selected != nil && *selected {
		fixture.resultID = uuid.New()
	}
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"WorkflowVersion":     entql.FieldEQ("workflow_id", workflowID),
				"ProcessingCandidate": entql.FieldEQ("workflow_id", workflowID),
				"ProcessingResult":    entql.FieldNotNil("candidate_id"),
			},
			Create: map[string]bool{
				"WorkflowVersion": true, "ProcessingCandidate": true, "ProcessingResult": true,
			},
		})
		versionID := uuid.New()
		if err := client.WorkflowVersion.Create().SetID(versionID).SetWorkflowID(workflowID).
			SetDefinition(processing.Definition{}).SetCreatedAt(at).SetCreatedBy(uuid.New()).Exec(scope); err != nil {
			return err
		}
		candidate := client.ProcessingCandidate.Create().SetID(fixture.candidateID).
			SetTransmissionID(fixture.transmissionID).SetWorkflowID(workflowID).
			SetDefinitionVersionID(versionID).SetWorkflowRevision(1).SetConfigurationRevision(1).
			SetSourceRevision(1).SetContextRevision(1).SetState(state).
			SetDueAt(at).SetCreatedAt(at).SetUpdatedAt(at)
		if selected != nil {
			candidate.SetSelected(*selected)
		}
		if failureCode != "" {
			candidate.SetFailureCode(failureCode)
		}
		if err := candidate.Exec(scope); err != nil {
			return err
		}
		if fixture.resultID != uuid.Nil {
			secret := processing.FieldValue{State: processing.FieldPresent,
				Value: &processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"private-evaluation-sentinel"`)}}
			return client.ProcessingResult.Create().SetID(fixture.resultID).
				SetCandidateID(fixture.candidateID).SetRevision(1).SetProducingFence(1).
				SetEvaluation(processing.Evaluation{Selected: true, Fields: map[string]processing.FieldValue{"secret": secret}}).
				SetCompletedAt(at).Exec(scope)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func createReadReviewer(t *testing.T, store *postgres.Store) (uuid.UUID, string, string) {
	t.Helper()
	username := "reader-" + uuid.NewString()
	usernameKey, err := identity.UsernameKey(username)
	if err != nil {
		t.Fatal(err)
	}
	password := "reviewer-password"
	passwordHash, err := identity.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	principalID, grantID := uuid.New(), uuid.New()
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Create:  map[string]bool{"Account": true, "Grant": true},
		})
		now := time.Now().UTC().Add(-time.Minute)
		if err := client.Account.Create().SetID(principalID).SetKind(account.KindHuman).
			SetUsername(username).SetUsernameKey(usernameKey).SetPasswordHash(passwordHash).
			SetCreatedAt(now).Exec(scope); err != nil {
			return err
		}
		return client.Grant.Create().SetID(grantID).SetSubjectID(principalID).
			SetSubjectKind(grant.SubjectKindPrincipal).SetActions([]string{"password.local", "rule.read"}).
			SetScope(authz.Scope{Kind: "all"}).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
	return principalID, username, password
}

func revokeReadReviewer(t *testing.T, store *postgres.Store, principalID uuid.UUID) {
	t.Helper()
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"Grant":           entql.FieldEQ("subject_id", principalID),
				"IdentityControl": entql.FieldEQ("id", 1),
			},
		})
		updated, err := client.Grant.Update().Where(grant.SubjectID(principalID)).
			SetActions([]string{"password.local"}).Save(scope)
		if err != nil {
			return err
		}
		if updated != 1 {
			return authz.ErrNotFound
		}
		return client.IdentityControl.UpdateOneID(1).AddAccessRevision(1).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessingStatusHTTPFiltersWorkflowPagesAndHidesPrivateEvidence(t *testing.T) {
	store := newManagementStore(t)
	runtime, err := composeProcessingAPIRuntime(t.Context(), store, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	originServer := httptest.NewUnstartedServer(nil)
	t.Cleanup(originServer.Close)
	origin := "http://" + originServer.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store, nil, runtime.routes)
	if err != nil {
		t.Fatal(err)
	}
	originServer.Config.Handler = handler
	originServer.Start()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := originServer.Client()
	client.Jar = jar
	login := managementRequest(t, client, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)

	createWorkflow := func(name, commandID string) uuid.UUID {
		t.Helper()
		body := `{"schemaVersion":1,"commandId":"` + commandID +
			`","name":"` + name + `","scope":{"kind":"all"},"enabled":true}`
		result := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
			body, login.CSRFToken, "", http.StatusCreated)
		id, _ := managementResultID(t, result)
		return id
	}
	workflowA := createWorkflow("Read A", "6e5adf3c-14bb-4bfc-a2b9-64e8171e08e1")
	workflowB := createWorkflow("Read B", "896cbd10-5d1a-4a46-8ee8-00197e34bb75")
	at := time.Now().UTC().Add(-time.Hour)
	pending := seedProcessingReadCandidate(t, store, workflowA, processingcandidate.StatePending, nil, "", at)
	completedSelected := true
	completed := seedProcessingReadCandidate(t, store, workflowA, processingcandidate.StateCompleted, &completedSelected, "", at.Add(time.Minute))
	failureCode := string(platform.CodeResourceLimit)
	failed := seedProcessingReadCandidate(t, store, workflowA, processingcandidate.StateFailed, nil, failureCode, at.Add(2*time.Minute))
	otherCompleted := seedProcessingReadCandidate(t, store, workflowB, processingcandidate.StateCompleted, &completedSelected, "", at)

	page := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates?limit=2", "", "", "", http.StatusOK)
	var firstPage struct {
		Items []struct {
			ID          uuid.UUID `json:"id"`
			Revision    int64     `json:"revision,string"`
			State       string    `json:"state"`
			Selected    *bool     `json:"selected"`
			FailureCode string    `json:"failureCode"`
			UpdatedAt   time.Time `json:"updatedAt"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(page.Data, &firstPage); err != nil {
		t.Fatal(err)
	}
	if firstPage.Total != 3 || len(firstPage.Items) != 2 {
		t.Fatalf("Workflow A first page total/items = %d/%d, want 3/2", firstPage.Total, len(firstPage.Items))
	}
	nextPage := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates?offset=2&limit=2", "", "", "", http.StatusOK)
	var lastPage struct {
		Items []struct {
			ID    uuid.UUID `json:"id"`
			State string    `json:"state"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(nextPage.Data, &lastPage); err != nil {
		t.Fatal(err)
	}
	if lastPage.Total != 3 || len(lastPage.Items) != 1 || lastPage.Items[0].ID != failed.candidateID ||
		lastPage.Items[0].State != "failed" {
		t.Fatalf("Workflow A final page = %+v, want only failed candidate", lastPage)
	}
	workflowBPage := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowB.String()+"/candidates", "", "", "", http.StatusOK)
	var otherPage struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(workflowBPage.Data, &otherPage); err != nil || otherPage.Total != 1 {
		t.Fatalf("Workflow B total = %d, decode error = %v; want 1", otherPage.Total, err)
	}

	status := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+completed.candidateID.String(),
		"", "", "", http.StatusOK)
	if strings.Contains(string(status.Data), pending.transmissionID.String()) ||
		strings.Contains(string(status.Data), "transmissionId") || strings.Contains(string(status.Data), "fencingToken") ||
		strings.Contains(string(status.Data), "workerId") || strings.Contains(string(status.Data), "attempts") {
		t.Fatalf("candidate status leaked private work fields: %s", status.Data)
	}
	result := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+completed.candidateID.String()+"/result",
		"", "", "", http.StatusOK)
	if strings.Contains(string(result.Data), "private-evaluation-sentinel") ||
		strings.Contains(string(result.Data), "evaluation") || strings.Contains(string(result.Data), "transmissionId") ||
		strings.Contains(string(result.Data), completed.transmissionID.String()) {
		t.Fatalf("private result metadata leaked Evaluation or source data: %s", result.Data)
	}
	var resultFields map[string]json.RawMessage
	if err := json.Unmarshal(result.Data, &resultFields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "revision", "completedAt"} {
		if _, ok := resultFields[field]; !ok {
			t.Fatalf("result metadata missing %q: %s", field, result.Data)
		}
	}
	if _, ok := resultFields["candidateId"]; ok {
		t.Fatalf("result metadata exposed json-hidden candidate binding: %s", result.Data)
	}

	invisible := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+otherCompleted.candidateID.String(),
		"", "", "", http.StatusNotFound)
	missing := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+uuid.NewString(),
		"", "", "", http.StatusNotFound)
	if invisible.Code != missing.Code {
		t.Fatalf("wrong-Workflow candidate code %q differs from missing candidate code %q", invisible.Code, missing.Code)
	}
	wrongResult := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+otherCompleted.candidateID.String()+"/result",
		"", "", "", http.StatusNotFound)
	if wrongResult.Code != missing.Code {
		t.Fatalf("wrong-Workflow result code %q differs from missing candidate code %q", wrongResult.Code, missing.Code)
	}

	reviewerID, reviewer, password := createReadReviewer(t, store)
	reviewerJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	reviewerClient := originServer.Client()
	reviewerClient.Jar = reviewerJar
	managementRequest(t, reviewerClient, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"`+reviewer+`","password":"`+password+`"}`, "", "", http.StatusOK)
	managementRequest(t, reviewerClient, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates", "", "", "", http.StatusOK)
	revokeReadReviewer(t, store, reviewerID)
	managementRequest(t, reviewerClient, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates", "", "", "", http.StatusForbidden)
}

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
			(id,gateway_id,gateway_network_identifier,network_id,is_default) VALUES($1,$2,'retry',$3,true)`,
			uuid.New(), gateway, network)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := composeProcessingAPIRuntime(t.Context(), store, "")
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
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store, nil, runtime.routes)
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
