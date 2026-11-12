package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

// pluginConfigFixture is a composed API with a provisioned Delivery plugin, a signed-in
// administrator and helpers for personal users.
type pluginConfigFixture struct {
	t        *testing.T
	store    *postgres.Store
	runtime  *processingAPIRuntime
	origin   string
	admin    *http.Client
	csrf     string
	operator uuid.UUID
	query    *queryFixture
	// directory is the provisioned plugin directory.
	directory string
	digest    string
	// secretFiles is the Secret file root.
	secretFiles string
}

// newPluginConfigFixture composes the runtime over a Delivery plugin directory; withKey selects
// whether a master key is configured.
func newPluginConfigFixture(t *testing.T, withKey bool) *pluginConfigFixture {
	t.Helper()
	store := newManagementStore(t)
	shared := mustComposeCore(t, store)
	binary := pluginfixture.Binary(t, "delivery")
	sum := sha256.Sum256(binary)
	f := &pluginConfigFixture{t: t, store: store, directory: t.TempDir(), digest: hex.EncodeToString(sum[:]),
		secretFiles: t.TempDir()}
	if err := os.WriteFile(filepath.Join(f.directory, "plugin"), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	f.writeManifest("manifest.json", nil)
	settings := processingRuntimeSettings{ArtifactRoot: f.directory,
		Secrets: pluginSecretSettings{FileRoot: f.secretFiles}}
	if withKey {
		cipher, err := newTestCipher()
		if err != nil {
			t.Fatal(err)
		}
		settings.Secrets.Cipher = cipher
	}
	runtime, err := composeProcessingAPIRuntimeWith(t.Context(), shared, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	f.runtime = runtime
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	f.origin = "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(f.origin, identity.DefaultLoginRateLimits(), shared, nil, runtime.routes)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	f.admin = backend.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	f.admin.Jar = jar
	login := managementRequest(t, f.admin, f.origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)
	f.csrf = login.CSRFToken
	f.operator = managementOperatorID(t, store)
	f.query = &queryFixture{t: t, store: store, origin: f.origin, client: &http.Client{}}
	return f
}

// writeManifest renders the Delivery manifest template for the fixture's digest, applies mutate to
// its decoded form and writes it below the plugin directory.
func (f *pluginConfigFixture) writeManifest(name string, mutate func(map[string]any)) {
	f.t.Helper()
	template, err := os.ReadFile("../../internal/processing/testdata/delivery/manifest.template.json")
	if err != nil {
		f.t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(template), "{{digest}}", f.digest)), &manifest); err != nil {
		f.t.Fatal(err)
	}
	addPersonalNickname(manifest)
	if mutate != nil {
		mutate(manifest)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.directory, name), encoded, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// addPersonalNickname declares an optional personal string property "nick" next to the template's
// personal Secret.
func addPersonalNickname(manifest map[string]any) {
	schema := manifest["configurationSchema"].(map[string]any)["properties"].(map[string]any)
	schema["nick"] = map[string]any{"kind": "string", "optional": true}
	policy := manifest["configurationPolicy"].(map[string]any)["properties"].(map[string]any)
	policy["nick"] = map[string]any{"configurationScope": "user", "secret": false}
}

// main returns the artifact reference of a manifest file in the plugin directory.
func (f *pluginConfigFixture) main(manifest string) map[string]any {
	return map[string]any{"manifestPath": manifest, "pin": map[string]any{
		"pluginId": "test-delivery", "version": "1", "capabilityId": "send", "digest": f.digest}}
}

// typedString and typedInteger build typed wire values.
func typedString(text string) map[string]any { return map[string]any{"kind": "string", "value": text} }

func typedInteger(number string) map[string]any {
	return map[string]any{"kind": "integer", "value": number}
}

func typedObject(members map[string]any) map[string]any {
	return map[string]any{"kind": "object", "value": members}
}

// instanceBody builds a save body for the Delivery instance; secrets and targetUse may be nil.
func (f *pluginConfigFixture) instanceBody(expected string, enabled bool, secrets map[string]any, targetUse any) string {
	body := map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "expectedRevision": expected, "enabled": enabled,
		"executionPrincipalId": f.operator, "main": f.main("manifest.json"),
		"configuration": typedObject(map[string]any{"title": typedString("Title")})}
	if secrets != nil {
		body["secrets"] = secrets
	}
	if targetUse != nil {
		body["targetUse"] = targetUse
	}
	return mustJSON(f.t, body)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// everyone is a target use admitting every Subscriber.
var everyone = map[string]any{"everyone": true, "principalIds": []string{}}

// adminRequest sends an administrator request and returns the decoded response.
func (f *pluginConfigFixture) adminRequest(method, path, body string, status int) managementResponse {
	f.t.Helper()
	csrf := ""
	if method != http.MethodGet {
		csrf = f.csrf
	}
	return managementRequest(f.t, f.admin, f.origin, method, path, body, csrf, "", status)
}

// userRequest sends a request as a bearer user.
func (f *pluginConfigFixture) userRequest(bearer, method, path, body string, status int) managementResponse {
	f.t.Helper()
	return managementRequest(f.t, f.query.client, f.origin, method, path, body, "", bearer, status)
}

// createInstance creates a Delivery instance with a stored administrative token and returns its ID.
func (f *pluginConfigFixture) createInstance(targetUse any) uuid.UUID {
	f.t.Helper()
	created := f.adminRequest(http.MethodPost, "/api/v1/instances", f.instanceBody("0", true,
		map[string]any{"apiToken": map[string]any{"value": "admin-token-plaintext"}}, targetUse), http.StatusCreated)
	id, _ := managementResultID(f.t, created)
	return id
}

// personalUser seeds an account that may manage its own personal values.
func (f *pluginConfigFixture) personalUser() (string, uuid.UUID) {
	f.t.Helper()
	return f.query.caller([]string{"subscription.manage-own"}, authz.Scope{Kind: "all"}, []string{"payload"})
}

// scalar runs a query that returns one text value.
func (f *pluginConfigFixture) scalar(query string, args ...any) string {
	f.t.Helper()
	var result string
	err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), query, args...).Scan(&result)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

// rawRequest sends an administrator request and returns the status and the whole body.
func (f *pluginConfigFixture) rawRequest(method, path, body string) (int, map[string]any) {
	f.t.Helper()
	request, err := http.NewRequestWithContext(f.t.Context(), method, f.origin+path, strings.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", f.origin)
	request.Header.Set(identity.CSRFHeaderName, f.csrf)
	response, err := f.admin.Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		f.t.Fatal(err)
	}
	return response.StatusCode, decoded
}
