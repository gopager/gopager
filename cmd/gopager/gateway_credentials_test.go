package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

type gatewayHTTPFixture struct {
	store    *postgres.Store
	client   *http.Client
	origin   string
	csrf     string
	gateway  uuid.UUID
	current  uuid.UUID
	sibling  uuid.UUID
	proposed uuid.UUID
	// agencies maps each fixture Network to the Agency that owns it.
	agencies map[uuid.UUID]uuid.UUID
}

type gatewayCredentialResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	CredentialID  string `json:"credentialId"`
	Secret        string `json:"secret,omitempty"`
	Status        string `json:"status"`
	Revision      string `json:"revision"`
	Code          string `json:"code"`
}

func newGatewayHTTPFixture(t *testing.T) gatewayHTTPFixture {
	t.Helper()
	store := newManagementStore(t)
	fixture := gatewayHTTPFixture{
		store: store, gateway: uuid.New(), current: uuid.New(), sibling: uuid.New(), proposed: uuid.New(),
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		// Every fixture Network has its own Agency, so grants on one Network stay distinguishable.
		fixture.agencies = make(map[uuid.UUID]uuid.UUID)
		for _, network := range []struct {
			id   uuid.UUID
			name string
		}{{fixture.current, "Current"}, {fixture.sibling, "Sibling"}, {fixture.proposed, "Proposed"}} {
			agency := uuid.New()
			fixture.agencies[network.id] = agency
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO agencies(id,name) VALUES($1,$2)`,
				agency, network.name+" agency"); err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO networks(id,name,protocol,agency_id)
				VALUES($1,$2,'pocsag',$3)`, network.id, network.name, agency); err != nil {
				return err
			}
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state) VALUES($1,'active')`, fixture.gateway); err != nil {
			return err
		}
		for _, assignment := range []struct {
			id         uuid.UUID
			identifier string
			network    uuid.UUID
			priority   int
		}{
			{uuid.New(), "current", fixture.current, 0},
			{uuid.New(), "sibling", fixture.sibling, 1},
		} {
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
				(id,gateway_id,gateway_network_identifier,network_id,priority)
				VALUES($1,$2,$3,$4,$5)`, assignment.id, fixture.gateway, assignment.identifier,
				assignment.network, assignment.priority); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	fixture.origin = "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(fixture.origin, identity.DefaultLoginRateLimits(), mustComposeCore(t, store), nil)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	fixture.client = backend.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	fixture.client.Jar = jar
	login := managementRequest(t, fixture.client, fixture.origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)
	if login.CSRFToken == "" {
		t.Fatal("login did not provide Session-bound CSRF")
	}
	fixture.csrf = login.CSRFToken
	return fixture
}

func (fixture gatewayHTTPFixture) request(t *testing.T, method, path, body, csrf, bearer string, expected int) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, fixture.origin+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", fixture.origin)
	if csrf != "" {
		request.Header.Set(identity.CSRFHeaderName, csrf)
	}
	if bearer != "" {
		request.Header.Set("Authorization", bearer)
	}
	response, err := fixture.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%s %s returned %d, expected %d; response=%s", method, path,
			response.StatusCode, expected, responseBody)
	}
	if response.StatusCode != http.StatusNoContent {
		var result struct {
			SchemaVersion int `json:"schemaVersion"`
		}
		if err := json.Unmarshal(responseBody, &result); err != nil || result.SchemaVersion != 1 {
			t.Fatalf("%s %s returned invalid schema envelope: %v; response=%s", method, path, err, responseBody)
		}
	}
	return responseBody
}

func (fixture gatewayHTTPFixture) setGrant(t *testing.T, networks []uuid.UUID, actions ...string) {
	t.Helper()
	scope := authz.Scope{Kind: "agency"}
	for _, network := range networks {
		scope.References = append(scope.References, fixture.agencies[network].String())
	}
	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := fixture.store.Resolve(tx)
		if err != nil {
			return err
		}
		result, err := executor.ExecContext(tx.Context(), `UPDATE identity_grants SET scope=$1,actions=$2`,
			string(scopeJSON), string(actionsJSON))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("test fixture has no administrative grant")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayNetworkReplacementHTTPAuthorizationAndValidation(t *testing.T) {
	t.Parallel()
	fixture := newGatewayHTTPFixture(t)
	path := "/api/v1/gateways/" + fixture.gateway.String() + "/networks"
	replace := func(revision string, networks string, expected int) []byte {
		body := `{"schemaVersion":1,"expectedRevision":"` + revision + `","networks":` + networks + `}`
		return fixture.request(t, http.MethodPut, path, body, fixture.csrf, "", expected)
	}

	fixture.request(t, http.MethodPut, path,
		`{"schemaVersion":1,"expectedRevision":"1","networks":[{"gatewayNetworkIdentifier":"proposed","networkId":"`+
			fixture.proposed.String()+`","priority":0}]}`, fixture.csrf, "Bearer invalid", http.StatusUnauthorized)
	replace("1", `[{"gatewayNetworkIdentifier":"proposed","networkId":"`+fixture.proposed.String()+`","priority":1}]`,
		http.StatusBadRequest)
	replace("1", `[]`, http.StatusBadRequest)
	replace("1", `[{"gatewayNetworkIdentifier":"first","networkId":"`+fixture.proposed.String()+`","priority":0},{"gatewayNetworkIdentifier":"second","networkId":"`+fixture.current.String()+`","priority":0}]`,
		http.StatusBadRequest)
	replace("1", `[{"gatewayNetworkIdentifier":"first","networkId":"`+fixture.proposed.String()+`","priority":0},{"gatewayNetworkIdentifier":"second","networkId":"`+fixture.current.String()+`","priority":2}]`,
		http.StatusBadRequest)
	replace("1", `[{"gatewayNetworkIdentifier":"same","networkId":"`+fixture.proposed.String()+`","priority":0},{"gatewayNetworkIdentifier":"same","networkId":"`+fixture.current.String()+`","priority":1}]`,
		http.StatusBadRequest)

	// D-140: a grant covering every current Network but not the proposed one is 403; a grant
	// that does not cover the Gateway's current Networks gives no right on it, so it is 404.
	fixture.setGrant(t, []uuid.UUID{fixture.current, fixture.sibling}, "gateway.update")
	replace("1", `[{"gatewayNetworkIdentifier":"proposed","networkId":"`+fixture.proposed.String()+`","priority":0}]`,
		http.StatusForbidden)
	fixture.setGrant(t, []uuid.UUID{fixture.current}, "gateway.update")
	replace("1", `[{"gatewayNetworkIdentifier":"proposed","networkId":"`+fixture.proposed.String()+`","priority":0}]`,
		http.StatusNotFound)
	fixture.setGrant(t, []uuid.UUID{fixture.proposed}, "gateway.update")
	replace("1", `[{"gatewayNetworkIdentifier":"proposed","networkId":"`+fixture.proposed.String()+`","priority":0}]`,
		http.StatusNotFound)

	fixture.setGrant(t, []uuid.UUID{fixture.current, fixture.sibling, fixture.proposed}, "gateway.update")
	var success struct {
		SchemaVersion int      `json:"schemaVersion"`
		Revision      string   `json:"revision"`
		CorrectionIDs []string `json:"correctionIds"`
	}
	response := replace("1", `[{"gatewayNetworkIdentifier":"proposed","networkId":"`+fixture.proposed.String()+`","priority":0}]`,
		http.StatusOK)
	// The candidate Networks changed, so one re-resolution correction is queued.
	if err := json.Unmarshal(response, &success); err != nil || success.Revision != "2" || len(success.CorrectionIDs) != 1 {
		t.Fatalf("replacement response = %s, error=%v", response, err)
	}
	replace("1", `[{"gatewayNetworkIdentifier":"proposed","networkId":"`+fixture.proposed.String()+`","priority":0}]`,
		http.StatusConflict)

	var count int
	var network uuid.UUID
	var priority int
	if err := fixture.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := fixture.store.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*), min(network_id::text)::uuid, min(priority)
			FROM gateway_networks WHERE gateway_id=$1`, fixture.gateway).Scan(&count, &network, &priority); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 || network != fixture.proposed || priority != 0 {
		t.Fatalf("replacement persisted count=%d network=%s priority=%d", count, network, priority)
	}
}

func TestGatewayCredentialManagementHTTPSecretLifecycleAndAuthorization(t *testing.T) {
	t.Parallel()
	fixture := newGatewayHTTPFixture(t)
	path := "/api/v1/gateways/" + fixture.gateway.String() + "/credentials"
	fixture.setGrant(t, []uuid.UUID{fixture.current}, "gateway.update", "gateway.read")

	// D-140: rights on only one of the Gateway's two Agencies give no right on it (404).
	firstCommand := uuid.New()
	firstBody := `{"schemaVersion":1,"commandId":"` + firstCommand.String() + `","expectedRevision":"1"}`
	fixture.request(t, http.MethodPost, path, firstBody, fixture.csrf, "", http.StatusNotFound)
	fixture.setGrant(t, []uuid.UUID{fixture.current, fixture.sibling}, "gateway.read")
	fixture.request(t, http.MethodPost, path, firstBody, fixture.csrf, "", http.StatusForbidden)
	fixture.setGrant(t, []uuid.UUID{fixture.current, fixture.sibling}, "gateway.update")
	firstResponse := fixture.request(t, http.MethodPost, path, firstBody, fixture.csrf, "", http.StatusCreated)
	var first gatewayCredentialResponse
	if err := json.Unmarshal(firstResponse, &first); err != nil || first.CredentialID == "" || first.Secret == "" ||
		first.Status != "current" || first.Revision != "2" {
		t.Fatalf("initial issue response = %s, error=%v", firstResponse, err)
	}

	replay := fixture.request(t, http.MethodPost, path, firstBody, fixture.csrf, "", http.StatusOK)
	if strings.Contains(string(replay), first.Secret) || strings.Contains(string(replay), `"secret"`) {
		t.Fatalf("credential replay disclosed one-time secret: %s", replay)
	}
	var replayed gatewayCredentialResponse
	if err := json.Unmarshal(replay, &replayed); err != nil ||
		replayed.CredentialID != first.CredentialID || replayed.Status != "current" || replayed.Secret != "" {
		t.Fatalf("credential replay response = %s, error=%v", replay, err)
	}
	statusPath := path
	status := fixture.request(t, http.MethodGet, statusPath, "", "", "", http.StatusForbidden)
	if strings.Contains(string(status), first.Secret) {
		t.Fatalf("forbidden status response disclosed secret: %s", status)
	}

	fixture.setGrant(t, []uuid.UUID{fixture.current, fixture.sibling}, "gateway.update", "gateway.read")
	status = fixture.request(t, http.MethodGet, statusPath, "", "", "", http.StatusOK)
	if strings.Contains(string(status), first.Secret) || strings.Contains(string(status), "secretHash") ||
		!strings.Contains(string(status), first.CredentialID) || !strings.Contains(string(status), `"current"`) {
		t.Fatalf("status response did not expose only nonsecret credential state: %s", status)
	}

	replacementCommand := uuid.New()
	replacementBody := `{"schemaVersion":1,"commandId":"` + replacementCommand.String() + `","expectedRevision":"2"}`
	replacementResponse := fixture.request(t, http.MethodPost, path, replacementBody, fixture.csrf, "", http.StatusCreated)
	var replacement gatewayCredentialResponse
	if err := json.Unmarshal(replacementResponse, &replacement); err != nil || replacement.CredentialID == "" ||
		replacement.Secret == "" || replacement.Secret == first.Secret || replacement.Status != "pending" ||
		replacement.Revision != "3" {
		t.Fatalf("replacement issue response = %s, error=%v", replacementResponse, err)
	}
	secondPending := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","expectedRevision":"3"}`
	fixture.request(t, http.MethodPost, path, secondPending, fixture.csrf, "", http.StatusConflict)
	changedReplay := `{"schemaVersion":1,"commandId":"` + replacementCommand.String() + `","expectedRevision":"3"}`
	changedConflict := fixture.request(t, http.MethodPost, path, changedReplay, fixture.csrf, "", http.StatusConflict)
	if !strings.Contains(string(changedConflict), `"code":"IDEMPOTENCY_CONFLICT"`) ||
		!strings.Contains(string(changedConflict), `"retryable":false`) {
		t.Fatalf("changed command reuse was not a non-retryable idempotency conflict: %s", changedConflict)
	}
	fixture.setGrant(t, []uuid.UUID{fixture.current}, "gateway.update")
	partialRevoke := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","expectedRevision":"3"}`
	fixture.request(t, http.MethodDelete, path+"/"+replacement.CredentialID, partialRevoke,
		fixture.csrf, "", http.StatusNotFound)
	fixture.setGrant(t, []uuid.UUID{fixture.current, fixture.sibling}, "gateway.read")
	fixture.request(t, http.MethodDelete, path+"/"+replacement.CredentialID, partialRevoke,
		fixture.csrf, "", http.StatusForbidden)
	fixture.setGrant(t, []uuid.UUID{fixture.current, fixture.sibling}, "gateway.update", "gateway.read")

	fixture.request(t, http.MethodPost, "/api/v1/gateway/state",
		`{"schemaVersion":1,"operation":"heartbeat"}`, "", "Bearer "+replacement.Secret, http.StatusNoContent)
	fixture.request(t, http.MethodPost, "/api/v1/gateway/state",
		`{"schemaVersion":1,"operation":"heartbeat"}`, "", "Bearer "+first.Secret, http.StatusUnauthorized)
	status = fixture.request(t, http.MethodGet, statusPath, "", "", "", http.StatusOK)
	if strings.Contains(string(status), replacement.Secret) || !strings.Contains(string(status), `"revoked"`) ||
		!strings.Contains(string(status), `"current"`) {
		t.Fatalf("cutover status did not show nonsecret predecessor and replacement states: %s", status)
	}

	var revision int64
	if err := fixture.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := fixture.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT revision FROM gateways WHERE id=$1`,
			fixture.gateway).Scan(&revision)
	}); err != nil {
		t.Fatal(err)
	}
	revokeBody := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","expectedRevision":"` +
		strconv.FormatInt(revision, 10) + `"}`
	revokeResponse := fixture.request(t, http.MethodDelete, path+"/"+replacement.CredentialID, revokeBody,
		fixture.csrf, "", http.StatusOK)
	if strings.Contains(string(revokeResponse), replacement.Secret) {
		t.Fatalf("revoke response disclosed secret: %s", revokeResponse)
	}
	status = fixture.request(t, http.MethodGet, statusPath, "", "", "", http.StatusOK)
	if strings.Contains(string(status), replacement.Secret) || !strings.Contains(string(status), `"revoked"`) {
		t.Fatalf("revoked credential status disclosed secret or omitted revoked state: %s", status)
	}
	// Each credential aggregate is issued at revision 1 and revoked at revision 2,
	// whether by rotation cutover (first) or explicit revocation (replacement).
	for _, credential := range []string{first.CredentialID, replacement.CredentialID} {
		var kinds []string
		if err := fixture.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := fixture.store.Resolve(tx)
			if err != nil {
				return err
			}
			rows, err := executor.QueryContext(tx.Context(), `SELECT envelope->'data'->>'changeKind',
				(envelope->>'aggregateRevision')::bigint FROM domain_events
				WHERE envelope->'aggregateRef'->>'type'='credential' AND envelope->'aggregateRef'->>'id'=$1
				ORDER BY (envelope->>'aggregateRevision')::bigint`, credential)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var kind string
				var revision int64
				if err := rows.Scan(&kind, &revision); err != nil {
					return err
				}
				kinds = append(kinds, kind+"@"+strconv.FormatInt(revision, 10))
			}
			return rows.Err()
		}); err != nil {
			t.Fatal(err)
		}
		if strings.Join(kinds, ",") != "credential-issued@1,credential-revoked@2" {
			t.Fatalf("credential %s facts are not strictly ordered issue@1, revoke@2: %v", credential, kinds)
		}
	}
}
