package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

// pluginArtifact is a provisioned plugin as an instance names it.
type pluginArtifact struct {
	manifestPath string
	pluginID     string
	version      string
	capabilityID string
	digest       string
}

// provisionFirstParty builds the shipped plugin plugins/<name> into a plugin directory the way
// plugins/build.sh lays it out, and returns its artifact reference.
func provisionFirstParty(t *testing.T, directory, name string) pluginArtifact {
	t.Helper()
	const version = "1.0.0"
	binary := pluginfixture.FirstParty(t, name)
	digest := sha256Hex(binary)
	target := filepath.Join(directory, name, version)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, name), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	template, err := os.ReadFile("../../plugins/" + name + "/manifest.template.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.ReplaceAll(string(template), "@DIGEST@", digest)
	if err := os.WriteFile(filepath.Join(target, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return pluginArtifact{manifestPath: name + "/" + version + "/manifest.json", pluginID: name, version: version,
		capabilityID: name, digest: digest}
}

// provisionFixture places the supplied Delivery test plugin below fixture/ of a plugin directory
// and returns its artifact reference.
func provisionFixture(t *testing.T, directory string) pluginArtifact {
	t.Helper()
	binary := pluginfixture.Binary(t, "delivery")
	digest := sha256Hex(binary)
	target := filepath.Join(directory, "fixture")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "plugin"), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	template, err := os.ReadFile("../../internal/processing/testdata/delivery/manifest.template.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(template), "{{digest}}", digest)), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["artifact"].(map[string]any)["path"] = "fixture/plugin"
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "manifest.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return pluginArtifact{manifestPath: "fixture/manifest.json", pluginID: "test-delivery", version: "1",
		capabilityID: "send", digest: digest}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// pluginDeliveryScene is the production composition of the plugin host and the Delivery side over
// a provisioned plugin directory: HTTP API, fan-out and a runner whose placement follows the
// host. An administrator is signed in; subscribers are added with subscriber.
type pluginDeliveryScene struct {
	*deliveryFixture
	runtime   *processingAPIRuntime
	directory string
	dsn       string
	session   *http.Client
	csrf      string
	operator  uuid.UUID
}

// newPluginDeliveryScene composes the runtime over an empty plugin directory with a master key.
func newPluginDeliveryScene(t *testing.T) *pluginDeliveryScene {
	t.Helper()
	store, dsn := newManagementStoreAt(t, []string{"unit", "address"})
	shared := mustComposeCore(t, store)
	cipher, err := newTestCipher()
	if err != nil {
		t.Fatal(err)
	}
	s := &pluginDeliveryScene{directory: t.TempDir(), dsn: dsn}
	runtime, err := composeProcessingAPIRuntimeWith(t.Context(), shared, processingRuntimeSettings{
		ArtifactRoot: s.directory, Secrets: pluginSecretSettings{Cipher: cipher}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	if err := runtime.bindDelivery(shared); err != nil {
		t.Fatal(err)
	}
	s.runtime = runtime
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, nil, runtime.routes)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	runner, err := composeDeliveryRunner(shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	query := &queryFixture{t: t, store: store, origin: origin, client: backend.Client(), gateway: uuid.New(),
		network: uuid.New(), agency: seedAgency(t, store)}
	query.exec(`INSERT INTO gateways (id, name) VALUES ($1, 'gw')`, query.gateway)
	query.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, 'Net', 'pocsag', $2)`, query.network, query.agency)
	query.exec(`INSERT INTO gateway_networks (id, gateway_id, gateway_network_identifier, network_id, priority)
		VALUES ($1, $2, 'net', $3, 0)`, uuid.New(), query.gateway, query.network)
	admin := uuid.New()
	s.deliveryFixture = &deliveryFixture{queryFixture: query, core: shared, runner: runner,
		admin: authz.Authority{PrincipalID: admin.String(), Revision: 1, Active: true}}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	s.session = &http.Client{Jar: jar}
	login := managementRequest(t, s.session, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)
	s.csrf = login.CSRFToken
	s.operator = managementOperatorID(t, store)
	return s
}

// adminRequest sends a request as the signed-in administrator.
func (s *pluginDeliveryScene) adminRequest(method, path, body string, status int) managementResponse {
	s.t.Helper()
	csrf := ""
	if method != http.MethodGet {
		csrf = s.csrf
	}
	return managementRequest(s.t, s.session, s.origin, method, path, body, csrf, "", status)
}

// instanceBody builds a save body for an instance of the artifact. A nil secrets map keeps the
// stored Secrets of the previous revision.
func (s *pluginDeliveryScene) instanceBody(art pluginArtifact, expected string, enabled bool,
	configuration map[string]any, secrets map[string]any, targetUse any) string {
	body := map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "expectedRevision": expected, "enabled": enabled,
		"executionPrincipalId": s.operator, "targetUse": targetUse, "configuration": typedObject(configuration),
		"main": map[string]any{"manifestPath": art.manifestPath, "pin": map[string]any{
			"pluginId": art.pluginID, "version": art.version, "capabilityId": art.capabilityID, "digest": art.digest}}}
	if secrets != nil {
		body["secrets"] = secrets
	}
	return mustJSON(s.t, body)
}

// createInstance creates an enabled instance usable by everyone and returns its ID.
func (s *pluginDeliveryScene) createInstance(art pluginArtifact, configuration, secrets map[string]any) uuid.UUID {
	s.t.Helper()
	created := s.adminRequest(http.MethodPost, "/api/v1/instances",
		s.instanceBody(art, "0", true, configuration, secrets, everyone), http.StatusCreated)
	id, _ := managementResultID(s.t, created)
	return id
}

// updateInstance saves the next revision, keeping the stored Secrets.
func (s *pluginDeliveryScene) updateInstance(id uuid.UUID, art pluginArtifact, expected string, enabled bool,
	configuration map[string]any, targetUse any) {
	s.t.Helper()
	s.adminRequest(http.MethodPut, "/api/v1/instances/"+id.String(),
		s.instanceBody(art, expected, enabled, configuration, nil, targetUse), http.StatusOK)
}

// typedBoolean builds a typed wire value.
func typedBoolean(value bool) map[string]any {
	return map[string]any{"kind": "boolean", "value": value}
}

// plainSecret is a Secret write carrying a value.
func plainSecret(value string) map[string]any { return map[string]any{"value": value} }

// subscribeTarget creates an active message.created Subscription to the instance and returns its ID.
// The target needs no inputs; extra replaces body properties.
func (s *pluginDeliveryScene) subscribeTarget(bearer string, instance uuid.UUID, extra map[string]any) uuid.UUID {
	s.t.Helper()
	body := map[string]any{"targets": []any{map[string]any{"instanceId": instance, "inputs": map[string]any{}}}}
	for key, value := range extra {
		body[key] = value
	}
	return s.subCreated(bearer, subInput(instance, body)).ID
}

// deliverMessage records a message.created fact for a new Message, runs fan-out and returns the
// IDs of the Deliveries created for it.
func (s *pluginDeliveryScene) deliverMessage(text string) []uuid.UUID {
	s.t.Helper()
	unit := s.unit("Engine "+text, nil)
	message := s.message(s.now().Add(-time.Minute), text, unit)
	s.messageFact("message.created", message, nil)
	s.fanOut()
	return s.deliveryIDs(message)
}

// deliveryIDs lists the Deliveries of a Message in creation order.
func (s *pluginDeliveryScene) deliveryIDs(message uuid.UUID) []uuid.UUID {
	s.t.Helper()
	var ids []uuid.UUID
	err := s.store.WithinTransaction(s.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := s.store.Resolve(tx)
		if err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), `SELECT id FROM deliveries WHERE message_id = $1 ORDER BY created_at, id`, message)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return ids
}

// text runs a query returning one text value.
func (s *pluginDeliveryScene) text(query string, args ...any) string {
	s.t.Helper()
	var result string
	err := s.store.WithinTransaction(s.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := s.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), query, args...).Scan(&result)
	})
	if err != nil {
		s.t.Fatalf("%s: %v", query, err)
	}
	return result
}

// status renders a Delivery as "state/outcome-code/attempts".
func (s *pluginDeliveryScene) status(id uuid.UUID) string {
	s.t.Helper()
	return s.text(`SELECT state || '/' || coalesce(outcome_code, '-') || '/' || attempts FROM deliveries WHERE id = $1`, id)
}

// attemptStates renders the attempts of a Delivery as "number:state:code:revision" joined by spaces.
func (s *pluginDeliveryScene) attemptStates(id uuid.UUID) string {
	s.t.Helper()
	return s.text(`SELECT coalesce(string_agg(number || ':' || state || ':' || coalesce(outcome_code, '-') || ':' ||
		instance_revision, ' ' ORDER BY number), '') FROM delivery_attempts WHERE delivery_id = $1`, id)
}

// overdue moves the next attempt of an open Delivery past the placement grace.
func (s *pluginDeliveryScene) overdue(id uuid.UUID, grace time.Duration) {
	s.t.Helper()
	s.exec(`UPDATE deliveries SET next_attempt_at = $2 WHERE id = $1`, id, s.now().Add(-grace-time.Minute))
}
