package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

var storedReference = regexp.MustCompile(`secret:[0-9a-f-]{36}`)

// instanceReference returns the stored Secret reference of an instance revision's token.
func (f *pluginConfigFixture) instanceReference(id uuid.UUID, revision int) string {
	f.t.Helper()
	return f.scalar(`SELECT configuration::text FROM processing_instance_versions WHERE instance_id = $1 AND revision = $2`, id, revision)
}

func TestPluginInstanceSecretsAreStoredAndNeverShown(t *testing.T) {
	t.Parallel()
	f := newPluginConfigFixture(t, true)
	id := f.createInstance(everyone)
	path := "/api/v1/instances/" + id.String()

	view := f.adminRequest(http.MethodGet, path, "", http.StatusOK)
	text := string(view.Data)
	for _, leaked := range []string{"admin-token-plaintext", "secret:", "manifestPath", "file:"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("view leaked %q: %s", leaked, text)
		}
	}
	var decoded struct {
		Capability string `json:"capability"`
		Lifecycle  string `json:"lifecycle"`
		TargetUse  struct {
			Everyone bool `json:"everyone"`
		} `json:"targetUse"`
		Status        struct{ Running bool } `json:"status"`
		Configuration struct {
			Fields map[string]struct {
				Source   string `json:"source"`
				Redacted bool   `json:"redacted"`
			} `json:"fields"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(view.Data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Capability != "delivery" || decoded.Lifecycle != "active" || !decoded.TargetUse.Everyone ||
		decoded.Configuration.Fields["apiToken"].Source != "stored" || !decoded.Configuration.Fields["apiToken"].Redacted {
		t.Fatalf("unexpected view: %s", text)
	}
	if rows := f.scalar(`SELECT count(*)::text FROM configuration_secrets`); rows != "1" {
		t.Fatalf("secret rows = %s", rows)
	}
	first := storedReference.FindString(f.instanceReference(id, 1))
	if first == "" {
		t.Fatal("revision 1 holds no stored reference")
	}

	f.adminRequest(http.MethodPut, path, f.instanceBody("1", true, nil, everyone), http.StatusOK)
	if kept := storedReference.FindString(f.instanceReference(id, 2)); kept != first {
		t.Fatalf("an update without secrets did not keep the reference: %q vs %q", kept, first)
	}
	f.adminRequest(http.MethodPut, path, f.instanceBody("2", true,
		map[string]any{"apiToken": map[string]any{"value": "second-plaintext"}}, everyone), http.StatusOK)
	if changed := storedReference.FindString(f.instanceReference(id, 3)); changed == "" || changed == first {
		t.Fatalf("a new value did not create a new row: %q", changed)
	}
	if rows := f.scalar(`SELECT count(*)::text FROM configuration_secrets`); rows != "2" {
		t.Fatalf("secret rows = %s, want 2 (old rows stay with their revision)", rows)
	}
	f.adminRequest(http.MethodPut, path, f.instanceBody("3", true,
		map[string]any{"apiToken": map[string]any{"reference": "file:test-delivery/tokens/api"}}, everyone), http.StatusOK)
	viewed := f.adminRequest(http.MethodGet, path, "", http.StatusOK)
	if !strings.Contains(string(viewed.Data), `"source":"file"`) || strings.Contains(string(viewed.Data), "tokens/api") {
		t.Fatalf("file reference view wrong: %s", viewed.Data)
	}

	for name, secrets := range map[string]map[string]any{
		"stored reference":          {"apiToken": map[string]any{"reference": "secret:" + uuid.NewString()}},
		"both":                      {"apiToken": map[string]any{"value": "x", "reference": "file:a"}},
		"neither":                   {"apiToken": map[string]any{}},
		"unknown path":              {"nothing": map[string]any{"value": "x"}},
		"personal path":             {"userKey": map[string]any{"value": "x"}},
		"remove required":           {"apiToken": nil},
		"bad environment":           {"apiToken": map[string]any{"reference": "env:HOME"}},
		"parent file path":          {"apiToken": map[string]any{"reference": "file:../x"}},
		"another plugin's variable": {"apiToken": map[string]any{"reference": "env:GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN"}},
		"another plugin's file":     {"apiToken": map[string]any{"reference": "file:pushover/token"}},
	} {
		if response := f.adminRequest(http.MethodPut, path, f.instanceBody("4", true, secrets, everyone),
			http.StatusBadRequest); response.Code != "INVALID_INPUT" {
			t.Errorf("%s: code = %s", name, response.Code)
		}
	}
	inline := mustJSON(t, map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "expectedRevision": "4", "enabled": true,
		"executionPrincipalId": f.operator, "main": f.main("manifest.json"), "targetUse": everyone,
		"configuration": typedObject(map[string]any{"title": typedString("T"), "apiToken": typedString("file:smuggled")})})
	if response := f.adminRequest(http.MethodPut, path, inline, http.StatusBadRequest); response.Code != "INVALID_INPUT" {
		t.Fatalf("a Secret inside the configuration: %s", response.Code)
	}
	if response := f.adminRequest(http.MethodPut, path, f.instanceBody("4", true, nil, nil), http.StatusBadRequest); response.Code != "INVALID_INPUT" {
		t.Fatalf("a Delivery instance without target use: %s", response.Code)
	}
}

func TestPluginInstanceWithoutMasterKey(t *testing.T) {
	t.Parallel()
	f := newPluginConfigFixture(t, false)
	response := f.adminRequest(http.MethodPost, "/api/v1/instances", f.instanceBody("0", true,
		map[string]any{"apiToken": map[string]any{"value": "plain"}}, everyone), http.StatusServiceUnavailable)
	if response.Code != "AUTHORITY_UNAVAILABLE" {
		t.Fatalf("code = %s", response.Code)
	}
	if rows := f.scalar(`SELECT count(*)::text FROM configuration_secrets`); rows != "0" {
		t.Fatalf("a Secret row was stored without a key: %s", rows)
	}
	f.adminRequest(http.MethodPost, "/api/v1/instances", f.instanceBody("0", true,
		map[string]any{"apiToken": map[string]any{"reference": "file:test-delivery/tokens/api"}}, everyone), http.StatusCreated)
}

func TestPluginPersonalConfigurationStaysPerOwner(t *testing.T) {
	t.Parallel()
	f := newPluginConfigFixture(t, true)
	alice, aliceID := f.personalUser()
	bob, _ := f.personalUser()
	carol, _ := f.personalUser()
	id := f.createInstance(map[string]any{"everyone": false, "principalIds": []uuid.UUID{aliceID}})
	path := "/api/v1/instances/" + id.String() + "/personal-configuration"
	write := func(expected string, configuration, secrets map[string]any) string {
		body := map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "expectedRevision": expected,
			"configuration": typedObject(configuration)}
		if secrets != nil {
			body["secrets"] = secrets
		}
		return mustJSON(t, body)
	}

	f.userRequest(carol, http.MethodGet, path, "", http.StatusNotFound)
	f.userRequest(carol, http.MethodPut, path, write("0", map[string]any{"nick": typedString("c")}, nil), http.StatusNotFound)
	f.userRequest(bob, http.MethodPut, path, write("0", map[string]any{"nick": typedString("b")}, nil), http.StatusNotFound)
	f.userRequest(alice, http.MethodGet, path, "", http.StatusNotFound)

	for name, body := range map[string]string{
		"administrative property": write("0", map[string]any{"title": typedString("hijack")}, nil),
		"unknown property":        write("0", map[string]any{"other": typedString("x")}, nil),
		"Secret in configuration": write("0", map[string]any{"userKey": typedString("file:x")}, nil),
		"administrative secret":   write("0", map[string]any{"nick": typedString("a")}, map[string]any{"apiToken": map[string]any{"value": "x"}}),
		"file reference":          write("0", map[string]any{"nick": typedString("a")}, map[string]any{"userKey": map[string]any{"reference": "file:keys/a"}}),
		"environment reference":   write("0", map[string]any{"nick": typedString("a")}, map[string]any{"userKey": map[string]any{"reference": "env:GOPAGER_PLUGIN_SECRET_A"}}),
	} {
		if response := f.userRequest(alice, http.MethodPut, path, body, http.StatusBadRequest); response.Code != "INVALID_INPUT" {
			t.Errorf("%s: code = %s", name, response.Code)
		}
	}
	if rows := f.scalar(`SELECT count(*)::text FROM plugin_personal_configurations`); rows != "0" {
		t.Fatalf("a rejected write stored a row: %s", rows)
	}

	saved := f.userRequest(alice, http.MethodPut, path, write("0", map[string]any{"nick": typedString("alice")},
		map[string]any{"userKey": map[string]any{"value": "alice-key-plaintext"}}), http.StatusOK)
	if _, revision := managementResultID(t, saved); revision != "1" {
		t.Fatalf("revision = %s", revision)
	}
	view := f.userRequest(alice, http.MethodGet, path, "", http.StatusOK)
	text := string(view.Data)
	if strings.Contains(text, "alice-key-plaintext") || strings.Contains(text, "secret:") || strings.Contains(text, "Title") ||
		!strings.Contains(text, `"source":"stored"`) || !strings.Contains(text, "alice") {
		t.Fatalf("personal view wrong: %s", text)
	}
	if owner := f.scalar(`SELECT owner_id::text FROM configuration_secrets WHERE owner_id IS NOT NULL`); owner != aliceID.String() {
		t.Fatalf("personal Secret owner = %s", owner)
	}
	f.userRequest(alice, http.MethodPut, path, write("0", map[string]any{"nick": typedString("again")}, nil), http.StatusConflict)
	keptKey := f.scalar(`SELECT configuration::text FROM plugin_personal_configurations`)
	f.userRequest(alice, http.MethodPut, path, write("1", map[string]any{"nick": typedString("renamed")}, nil), http.StatusOK)
	if after := f.scalar(`SELECT configuration::text FROM plugin_personal_configurations`); !strings.Contains(after, "renamed") ||
		storedReference.FindString(after) != storedReference.FindString(keptKey) {
		t.Fatalf("an update without secrets did not keep the Secret: %s", after)
	}

	f.adminRequest(http.MethodPut, "/api/v1/instances/"+id.String(), f.instanceBody("1", true, nil,
		map[string]any{"everyone": true, "principalIds": []string{}}), http.StatusOK)
	f.userRequest(bob, http.MethodPut, path, write("0", map[string]any{"nick": typedString("bob")}, nil), http.StatusOK)
	if got := string(f.userRequest(alice, http.MethodGet, path, "", http.StatusOK).Data); strings.Contains(got, "bob") {
		t.Fatalf("another owner's value is visible: %s", got)
	}
	if got := string(f.userRequest(bob, http.MethodGet, path, "", http.StatusOK).Data); strings.Contains(got, "renamed") {
		t.Fatalf("another owner's value is visible: %s", got)
	}
	if administrative := string(f.adminRequest(http.MethodGet, "/api/v1/instances/"+id.String(), "", http.StatusOK).Data); !strings.Contains(administrative, `"title"`) {
		t.Fatalf("administrative view changed: %s", administrative)
	}

	deleteBody := mustJSON(t, map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "expectedRevision": "2"})
	deleted := f.userRequest(alice, http.MethodDelete, path, deleteBody, http.StatusOK)
	if _, revision := managementResultID(t, deleted); revision != "3" {
		t.Fatalf("delete revision = %s, want the tombstone revision 3", revision)
	}
	f.userRequest(alice, http.MethodGet, path, "", http.StatusNotFound)
	if rows := f.scalar(`SELECT count(*)::text FROM configuration_secrets WHERE owner_id = $1`, aliceID); rows != "0" {
		t.Fatalf("the owner's Secret rows stayed after delete: %s", rows)
	}
	f.userRequest(bob, http.MethodGet, path, "", http.StatusOK)

	// Revisions never repeat: a client holding revision 1 or 2 cannot touch the re-created values.
	f.userRequest(alice, http.MethodDelete, path, mustJSON(t, map[string]any{"schemaVersion": 1, "commandId": uuid.New(),
		"expectedRevision": "2"}), http.StatusConflict)
	recreated := f.userRequest(alice, http.MethodPut, path, write("0", map[string]any{"nick": typedString("again")}, nil), http.StatusOK)
	if _, revision := managementResultID(t, recreated); revision != "4" {
		t.Fatalf("re-created revision = %s, want 4", revision)
	}
	f.userRequest(alice, http.MethodPut, path, write("1", map[string]any{"nick": typedString("stale")}, nil), http.StatusConflict)
	f.userRequest(alice, http.MethodDelete, path, mustJSON(t, map[string]any{"schemaVersion": 1, "commandId": uuid.New(),
		"expectedRevision": "1"}), http.StatusConflict)
	f.userRequest(alice, http.MethodPut, path, write("4", map[string]any{"nick": typedString("current")}, nil), http.StatusOK)

	disabled := f.instanceBody("2", false, nil, everyone)
	f.adminRequest(http.MethodPut, "/api/v1/instances/"+id.String(), disabled, http.StatusOK)
	f.userRequest(bob, http.MethodGet, path, "", http.StatusNotFound)
}

func TestPluginUnitParameters(t *testing.T) {
	t.Parallel()
	f := newPluginConfigFixture(t, true)
	id := f.createInstance(everyone)
	unit, agency := uuid.New(), uuid.New()
	f.query.exec(`INSERT INTO agencies (id, name) VALUES ($1, 'A')`, agency)
	f.query.exec(`INSERT INTO units (id, name, agency_id) VALUES ($1, 'U', $2)`, unit, agency)
	base := "/api/v1/instances/" + id.String() + "/unit-parameters"
	put := func(unit uuid.UUID, values map[string]any, status int) managementResponse {
		return f.adminRequest(http.MethodPut, base+"/"+unit.String(),
			mustJSON(t, map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "values": values}), status)
	}
	put(unit, map[string]any{"sound": typedString("bell"), "priority": typedInteger("2")}, http.StatusOK)
	for name, values := range map[string]map[string]any{
		"undeclared key": {"volume": typedInteger("1")},
		"wrong kind":     {"priority": typedString("high")},
		"too large":      {"sound": typedString(strings.Repeat("x", 80))},
	} {
		if response := put(unit, values, http.StatusBadRequest); response.Code != "INVALID_INPUT" {
			t.Errorf("%s: %s", name, response.Code)
		}
	}
	put(uuid.New(), map[string]any{"sound": typedString("bell")}, http.StatusNotFound)
	list := f.adminRequest(http.MethodGet, base, "", http.StatusOK)
	if !strings.Contains(string(list.Data), unit.String()) || !strings.Contains(string(list.Data), `"total":1`) {
		t.Fatalf("list = %s", list.Data)
	}
	other := f.adminRequest(http.MethodGet, base+"?unitId="+uuid.NewString(), "", http.StatusOK)
	if !strings.Contains(string(other.Data), `"total":0`) {
		t.Fatalf("filtered list = %s", other.Data)
	}
	f.adminRequest(http.MethodDelete, base+"/"+unit.String(),
		mustJSON(t, map[string]any{"schemaVersion": 1, "commandId": uuid.New()}), http.StatusOK)
	if empty := f.adminRequest(http.MethodGet, base, "", http.StatusOK); !strings.Contains(string(empty.Data), `"total":0`) {
		t.Fatalf("list after delete = %s", empty.Data)
	}
	f.adminRequest(http.MethodGet, "/api/v1/instances/"+uuid.NewString()+"/unit-parameters", "", http.StatusNotFound)
}

func TestPluginInspectionAndCapabilityGuard(t *testing.T) {
	t.Parallel()
	f := newPluginConfigFixture(t, true)
	f.writeManifest("overreach.json", func(manifest map[string]any) {
		manifest["requiredOperations"] = []string{"evaluate"}
	})
	inspect := func(manifest string) (int, map[string]any) {
		return f.rawRequest(http.MethodPost, "/api/v1/instances/inspection",
			mustJSON(t, map[string]any{"schemaVersion": 1, "main": f.main(manifest)}))
	}
	// A session POST without the CSRF token is refused like any other state-changing request.
	managementRequest(t, f.admin, f.origin, http.MethodPost, "/api/v1/instances/inspection",
		mustJSON(t, map[string]any{"schemaVersion": 1, "main": f.main("manifest.json")}), "", "", http.StatusForbidden)
	status, body := inspect("manifest.json")
	if status != http.StatusOK {
		t.Fatalf("inspection = %d %v", status, body)
	}
	data := mustJSON(t, body["data"])
	for _, want := range []string{`"pluginId":"test-delivery"`, `"capability":"delivery"`, `"outputKinds"`, `"resources"`,
		`"configurationPolicy"`, `"supportsPreview":false`} {
		if !strings.Contains(data, want) {
			t.Errorf("inspection lacks %s: %s", want, data)
		}
	}
	if strings.Contains(data, f.digest) || strings.Contains(data, "manifest.json") {
		t.Fatalf("inspection leaked the artifact identity: %s", data)
	}
	for _, manifest := range []string{"overreach.json"} {
		status, body = inspect(manifest)
		parameters, _ := body["parameters"].(map[string]any)
		if status != http.StatusForbidden || body["code"] != "CAPABILITY_UNDECLARED" || parameters["capability"] != "delivery" {
			t.Fatalf("overreaching manifest = %d %v", status, body)
		}
	}
	save := mustJSON(t, map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "expectedRevision": "0", "enabled": true,
		"executionPrincipalId": f.operator, "main": f.main("overreach.json"), "targetUse": everyone,
		"configuration": typedObject(map[string]any{"title": typedString("T")}),
		"secrets":       map[string]any{"apiToken": map[string]any{"reference": "file:test-delivery/t"}}})
	if status, body := f.rawRequest(http.MethodPost, "/api/v1/instances", save); status != http.StatusForbidden ||
		body["code"] != "CAPABILITY_UNDECLARED" {
		t.Fatalf("saving an overreaching manifest = %d %v", status, body)
	}
}

func TestPluginInstanceLifecycle(t *testing.T) {
	t.Parallel()
	f := newPluginConfigFixture(t, true)
	id := f.createInstance(everyone)
	path := "/api/v1/instances/" + id.String()
	lifecycle := func() (string, string) {
		view := f.adminRequest(http.MethodGet, path, "", http.StatusOK)
		var decoded struct {
			Lifecycle     string          `json:"lifecycle"`
			Configuration json.RawMessage `json:"configuration"`
		}
		if err := json.Unmarshal(view.Data, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded.Lifecycle, string(decoded.Configuration)
	}
	if state, configuration := lifecycle(); state != "active" || configuration == "" {
		t.Fatalf("active instance: %s %s", state, configuration)
	}
	f.adminRequest(http.MethodPut, path, f.instanceBody("1", false, nil, everyone), http.StatusOK)
	if state, configuration := lifecycle(); state != "disabled" || configuration == "" {
		t.Fatalf("disabled instance: %s %s", state, configuration)
	}
	listed := f.adminRequest(http.MethodGet, "/api/v1/instances", "", http.StatusOK)
	if !strings.Contains(string(listed.Data), `"capability":"delivery"`) {
		t.Fatalf("summary lacks the capability: %s", listed.Data)
	}
	f.writeManifest("manifest.json", func(manifest map[string]any) { manifest["sdkMajor"] = 3 })
	if state, configuration := lifecycle(); state != "incompatible" || configuration != "" {
		t.Fatalf("incompatible instance: %s %q", state, configuration)
	}
	f.writeManifest("manifest.json", func(manifest map[string]any) {
		manifest["configurationSchema"].(map[string]any)["properties"].(map[string]any)["title"] = map[string]any{"kind": "string", "optional": true}
		manifest["configurationPolicy"].(map[string]any)["properties"].(map[string]any)["title"] =
			map[string]any{"configurationScope": "user", "secret": false}
	})
	if state, configuration := lifecycle(); state != "failed" || configuration != "" {
		t.Fatalf("instance whose stored configuration the host would reject: %s %q", state, configuration)
	}
	f.writeManifest("manifest.json", nil)
	if err := removePlugin(f.directory); err != nil {
		t.Fatal(err)
	}
	if state, configuration := lifecycle(); state != "failed" || configuration != "" {
		t.Fatalf("failed instance: %s %q", state, configuration)
	}
}
