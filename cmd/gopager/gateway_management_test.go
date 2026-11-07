package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// manage sends a management request through the fixture's Session and returns its data.
func (fixture gatewayHTTPFixture) manage(t *testing.T, method, path, body string, expected int) json.RawMessage {
	t.Helper()
	csrf := ""
	if method != http.MethodGet {
		csrf = fixture.csrf
	}
	return managementRequest(t, fixture.client, fixture.origin, method, path, body, csrf, "", expected).Data
}

// setScopedGrant replaces the administrative grant with actions in one explicit scope.
func (fixture gatewayHTTPFixture) setScopedGrant(t *testing.T, scope authz.Scope, actions ...string) {
	t.Helper()
	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		t.Fatal(err)
	}
	fixture.exec(t, `UPDATE identity_grants SET scope=$1,actions=$2`, string(scopeJSON), string(actionsJSON))
}

func (fixture gatewayHTTPFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if err := fixture.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := fixture.store.Resolve(tx)
		if err != nil {
			return err
		}
		result, err := executor.ExecContext(tx.Context(), query, args...)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil || count == 0 {
			return errors.Join(err, errors.New("statement changed no row"))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func (fixture gatewayHTTPFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var result int
	if err := fixture.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := fixture.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), query, args...).Scan(&result)
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

type listData struct {
	Items []struct {
		ID       uuid.UUID `json:"id"`
		Revision string    `json:"revision"`
	} `json:"items"`
	Total int `json:"total"`
}

func decodeData[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return value
}

func agencyScope(agencies ...uuid.UUID) authz.Scope {
	scope := authz.Scope{Kind: "agency"}
	for _, agency := range agencies {
		scope.References = append(scope.References, agency.String())
	}
	return scope
}

// TestNetworkManagementHTTP covers F-85 (Networks), F-86 and F-87.
func TestNetworkManagementHTTP(t *testing.T) {
	t.Parallel()
	fixture := newGatewayHTTPFixture(t)
	current, sibling, proposed := fixture.agencies[fixture.current], fixture.agencies[fixture.sibling],
		fixture.agencies[fixture.proposed]
	create := func(agency uuid.UUID, expected int) json.RawMessage {
		return fixture.manage(t, http.MethodPost, "/api/v1/networks", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+
			`","name":"East","protocol":"pocsag","agencyId":"`+agency.String()+`"}`, expected)
	}
	created := decodeData[struct {
		ID       uuid.UUID `json:"id"`
		Revision string    `json:"revision"`
	}](t, create(proposed, http.StatusCreated))
	network := decodeData[struct {
		Name     string    `json:"name"`
		Protocol string    `json:"protocol"`
		AgencyID uuid.UUID `json:"agencyId"`
	}](t, fixture.manage(t, http.MethodGet, "/api/v1/networks/"+created.ID.String(), "", http.StatusOK))
	if network.Name != "East" || network.Protocol != "pocsag" || network.AgencyID != proposed {
		t.Fatalf("created Network %+v", network)
	}
	fixture.manage(t, http.MethodPut, "/api/v1/networks/"+created.ID.String(),
		`{"schemaVersion":1,"expectedRevision":"1","name":"East 2"}`, http.StatusOK)
	fixture.manage(t, http.MethodPut, "/api/v1/networks/"+created.ID.String(),
		`{"schemaVersion":1,"expectedRevision":"1","name":"East 3"}`, http.StatusConflict)
	create(uuid.New(), http.StatusBadRequest)
	if all := decodeData[listData](t, fixture.manage(t, http.MethodGet, "/api/v1/networks", "", http.StatusOK)); all.Total != 4 {
		t.Fatalf("installation-wide list total %d, want 4", all.Total)
	}

	// F-85: a grant on one Agency neither creates elsewhere nor reveals other Networks.
	fixture.setScopedGrant(t, agencyScope(current), "network.read", "network.create", "network.update", "network.delete")
	create(sibling, http.StatusForbidden)
	fixture.manage(t, http.MethodGet, "/api/v1/networks/"+fixture.sibling.String(), "", http.StatusNotFound)
	fixture.manage(t, http.MethodPut, "/api/v1/networks/"+fixture.sibling.String(),
		`{"schemaVersion":1,"expectedRevision":"1","name":"Taken"}`, http.StatusNotFound)
	scoped := decodeData[listData](t, fixture.manage(t, http.MethodGet, "/api/v1/networks", "", http.StatusOK))
	if scoped.Total != 1 || len(scoped.Items) != 1 || scoped.Items[0].ID != fixture.current {
		t.Fatalf("scoped Network list %+v", scoped)
	}

	// D-140: a caller with a read right on the Network is told the change is forbidden.
	fixture.setScopedGrant(t, agencyScope(current), "network.read")
	fixture.manage(t, http.MethodPut, "/api/v1/networks/"+fixture.current.String(),
		`{"schemaVersion":1,"expectedRevision":"1","name":"Renamed"}`, http.StatusForbidden)
	fixture.manage(t, http.MethodDelete, "/api/v1/networks/"+fixture.sibling.String(),
		`{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusNotFound)
	fixture.setScopedGrant(t, agencyScope(current), "network.read", "network.create", "network.update", "network.delete")

	// F-86: the in-use conflict names no Gateway, rule, Workflow or Agency the caller cannot see.
	conflict := managementRequest(t, fixture.client, fixture.origin, http.MethodDelete,
		"/api/v1/networks/"+fixture.current.String(), `{"schemaVersion":1,"expectedRevision":"1"}`,
		fixture.csrf, "", http.StatusConflict)
	raw, err := json.Marshal(conflict)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []uuid.UUID{fixture.gateway, fixture.sibling, sibling} {
		if strings.Contains(string(raw), hidden.String()) {
			t.Fatalf("in-use conflict reveals %s: %s", hidden, raw)
		}
	}
	if conflict.Code != "REVISION_CONFLICT" {
		t.Fatalf("in-use conflict code %s", conflict.Code)
	}

	// F-87: after the assignment is removed the Network is deleted softly and stays referenced.
	fixture.setScopedGrant(t, authz.Scope{Kind: "all"}, authz.Actions()...)
	fixture.request(t, http.MethodPut, "/api/v1/gateways/"+fixture.gateway.String()+"/networks",
		`{"schemaVersion":1,"expectedRevision":"1","networks":[{"gatewayNetworkIdentifier":"sibling","networkId":"`+
			fixture.sibling.String()+`","priority":0}]}`, fixture.csrf, "", http.StatusOK)
	fixture.manage(t, http.MethodDelete, "/api/v1/networks/"+fixture.current.String(),
		`{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusOK)
	fixture.manage(t, http.MethodGet, "/api/v1/networks/"+fixture.current.String(), "", http.StatusNotFound)
	fixture.request(t, http.MethodPut, "/api/v1/gateways/"+fixture.gateway.String()+"/networks",
		`{"schemaVersion":1,"expectedRevision":"2","networks":[{"gatewayNetworkIdentifier":"current","networkId":"`+
			fixture.current.String()+`","priority":0}]}`, fixture.csrf, "", http.StatusBadRequest)
	if rows := fixture.count(t, `SELECT count(*) FROM networks WHERE id=$1 AND deleted AND revision=2`, fixture.current); rows != 1 {
		t.Fatalf("soft-deleted Network rows %d", rows)
	}
	if rows := fixture.count(t, `SELECT count(*) FROM identity_audit WHERE action LIKE 'network.%'`); rows != 3 {
		t.Fatalf("Network audit records %d, want create, rename and delete", rows)
	}
}

type gatewayView struct {
	ID            uuid.UUID `json:"id"`
	Revision      string    `json:"revision"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	State         string    `json:"state"`
	LastContactAt *string   `json:"lastContactAt"`
	Networks      []struct {
		NetworkID uuid.UUID `json:"networkId"`
	} `json:"networks"`
}

type historyView struct {
	Items []struct {
		OldState string `json:"oldState"`
		NewState string `json:"newState"`
		Cause    string `json:"cause"`
	} `json:"items"`
	Total int `json:"total"`
}

// TestGatewayManagementHTTP covers F-85 (Gateways), F-88 and F-90 over HTTP, including
// first contact through a Submission.
func TestGatewayManagementHTTP(t *testing.T) {
	t.Parallel()
	fixture := newGatewayHTTPFixture(t)
	current := fixture.agencies[fixture.current]
	createBody := func(networks ...uuid.UUID) string {
		entries := make([]string, len(networks))
		for index, network := range networks {
			entries[index] = `{"gatewayNetworkIdentifier":"n` + string(rune('a'+index)) + `","networkId":"` +
				network.String() + `","priority":` + string(rune('0'+index)) + `}`
		}
		return `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","name":"Roof","networks":[` +
			strings.Join(entries, ",") + `]}`
	}
	created := decodeData[gatewayView](t, fixture.manage(t, http.MethodPost, "/api/v1/gateways",
		createBody(fixture.proposed), http.StatusCreated))
	gatewayPath := "/api/v1/gateways/" + created.ID.String()
	view := decodeData[gatewayView](t, fixture.manage(t, http.MethodGet, gatewayPath, "", http.StatusOK))
	if view.Name != "Roof" || !view.Enabled || view.State != "never-seen" || view.LastContactAt != nil ||
		len(view.Networks) != 1 || view.Networks[0].NetworkID != fixture.proposed {
		t.Fatalf("created Gateway %+v", view)
	}
	diagnostics := fixture.manage(t, http.MethodGet, gatewayPath+"/diagnostics", "", http.StatusOK)
	if !strings.Contains(string(diagnostics), `"state":"never-seen"`) || !strings.Contains(string(diagnostics), `"lastContactAt":null`) ||
		!strings.Contains(string(diagnostics), `"historyRemovedBefore":null`) || strings.Contains(string(diagnostics), "networks") {
		t.Fatalf("never-seen diagnostics %s", diagnostics)
	}
	if history := decodeData[historyView](t, fixture.manage(t, http.MethodGet, gatewayPath+"/history", "", http.StatusOK)); history.Total != 0 {
		t.Fatalf("never-seen history %+v", history)
	}
	fixture.manage(t, http.MethodPost, "/api/v1/gateways", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+
		`","name":" ","networks":[]}`, http.StatusBadRequest)

	// F-85: a grant on one Agency cannot reach a Gateway that also uses another Agency.
	fixture.setScopedGrant(t, agencyScope(current), "gateway.read", "gateway.create", "gateway.update", "gateway.delete")
	fixturePath := "/api/v1/gateways/" + fixture.gateway.String()
	fixture.manage(t, http.MethodGet, fixturePath, "", http.StatusNotFound)
	fixture.manage(t, http.MethodGet, fixturePath+"/diagnostics", "", http.StatusNotFound)
	fixture.manage(t, http.MethodPut, fixturePath, `{"schemaVersion":1,"expectedRevision":"1","name":"X","enabled":false}`,
		http.StatusNotFound)
	fixture.manage(t, http.MethodDelete, fixturePath, `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusNotFound)
	fixture.manage(t, http.MethodPost, "/api/v1/gateways", createBody(fixture.current, fixture.sibling), http.StatusForbidden)
	fixture.manage(t, http.MethodPost, "/api/v1/gateways", createBody(), http.StatusForbidden)
	own := decodeData[gatewayView](t, fixture.manage(t, http.MethodPost, "/api/v1/gateways", createBody(fixture.current), http.StatusCreated))
	scoped := decodeData[listData](t, fixture.manage(t, http.MethodGet, "/api/v1/gateways", "", http.StatusOK))
	if scoped.Total != 1 || len(scoped.Items) != 1 || scoped.Items[0].ID != own.ID {
		t.Fatalf("scoped Gateway list %+v", scoped)
	}

	// D-140: a caller that may read every Agency of the Gateway is told a change is forbidden.
	fixture.setScopedGrant(t, agencyScope(current, fixture.agencies[fixture.sibling]), "gateway.read")
	fixture.manage(t, http.MethodPut, fixturePath, `{"schemaVersion":1,"expectedRevision":"1","name":"X","enabled":false}`,
		http.StatusForbidden)
	fixture.manage(t, http.MethodDelete, fixturePath, `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusForbidden)

	// F-88: credentials never appear in management views; disabling and deletion block them.
	fixture.setScopedGrant(t, authz.Scope{Kind: "all"}, authz.Actions()...)
	if all := decodeData[listData](t, fixture.manage(t, http.MethodGet, "/api/v1/gateways", "", http.StatusOK)); all.Total != 3 {
		t.Fatalf("installation-wide Gateway list total %d, want 3", all.Total)
	}
	issued := fixture.request(t, http.MethodPost, gatewayPath+"/credentials",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"1"}`, fixture.csrf, "", http.StatusCreated)
	var credential gatewayCredentialResponse
	if err := json.Unmarshal(issued, &credential); err != nil || credential.Secret == "" {
		t.Fatalf("issue %s: %v", issued, err)
	}
	bearer := "Bearer " + credential.Secret
	for _, path := range []string{gatewayPath, gatewayPath + "/diagnostics", gatewayPath + "/history", "/api/v1/gateways"} {
		if body := fixture.manage(t, http.MethodGet, path, "", http.StatusOK); strings.Contains(string(body), credential.Secret) ||
			strings.Contains(string(body), credential.CredentialID) {
			t.Fatalf("%s exposes credential data: %s", path, body)
		}
	}
	heartbeat := `{"schemaVersion":1,"operation":"heartbeat"}`
	fixture.manage(t, http.MethodPut, gatewayPath, `{"schemaVersion":1,"expectedRevision":"2","name":"Roof","enabled":false}`,
		http.StatusOK)
	fixture.request(t, http.MethodPost, "/api/v1/gateway/state", heartbeat, "", bearer, http.StatusUnauthorized)
	fixture.manage(t, http.MethodPut, gatewayPath, `{"schemaVersion":1,"expectedRevision":"3","name":"Roof","enabled":true}`,
		http.StatusOK)
	submission := `{"schemaVersion":1,"submissionId":"first","protocol":"pocsag",` +
		`"observationTime":"2026-11-04T12:00:00Z","payload":{"form":"tone-only"},"transmissionTargets":[]}`
	fixture.request(t, http.MethodPost, "/api/v1/transmissions", submission, "", bearer, http.StatusOK)
	history := decodeData[historyView](t, fixture.manage(t, http.MethodGet, gatewayPath+"/history", "", http.StatusOK))
	if history.Total != 1 || history.Items[0].OldState != "never-seen" || history.Items[0].NewState != "active" ||
		history.Items[0].Cause != "submission" {
		t.Fatalf("first contact history %+v", history)
	}
	fixture.manage(t, http.MethodDelete, gatewayPath, `{"schemaVersion":1,"expectedRevision":"5"}`, http.StatusOK)
	fixture.request(t, http.MethodPost, "/api/v1/gateway/state", heartbeat, "", bearer, http.StatusUnauthorized)
	fixture.manage(t, http.MethodGet, gatewayPath, "", http.StatusNotFound)
	fixture.request(t, http.MethodGet, gatewayPath+"/credentials", "", "", "", http.StatusNotFound)
	if rows := fixture.count(t, `SELECT count(*) FROM gateway_credentials WHERE gateway_id=$1 AND revoked_at IS NULL`, created.ID); rows != 0 {
		t.Fatalf("unrevoked credentials after deletion %d", rows)
	}
	if facts := fixture.count(t, `SELECT count(*) FROM domain_events WHERE envelope->'aggregateRef'->>'id'=$1
		AND envelope->'data'->>'changeKind'='credential-revoked'`, credential.CredentialID); facts != 1 {
		t.Fatalf("credential revocation facts %d", facts)
	}
	if rows := fixture.count(t, `SELECT count(*) FROM transmissions WHERE gateway_id=$1`, created.ID); rows != 1 {
		t.Fatalf("accepted Transmissions after deletion %d", rows)
	}
}

// TestGatewayDelegationHTTP covers F-89: single-Gateway diagnostics and rotation grants reach
// only their Gateway and only their own operations.
func TestGatewayDelegationHTTP(t *testing.T) {
	t.Parallel()
	fixture := newGatewayHTTPFixture(t)
	other := decodeData[gatewayView](t, fixture.manage(t, http.MethodPost, "/api/v1/gateways",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Other","networks":[]}`, http.StatusCreated))
	g1, g2 := "/api/v1/gateways/"+fixture.gateway.String(), "/api/v1/gateways/"+other.ID.String()
	gatewayScope := authz.Scope{Kind: "gateway", References: []string{fixture.gateway.String()}}
	issue := func(path, revision string, expected int) []byte {
		return fixture.request(t, http.MethodPost, path+"/credentials",
			`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"`+revision+`"}`,
			fixture.csrf, "", expected)
	}

	fixture.setScopedGrant(t, gatewayScope, "gateway.diagnostics.read")
	fixture.manage(t, http.MethodGet, g1+"/diagnostics", "", http.StatusOK)
	fixture.manage(t, http.MethodGet, g1+"/history", "", http.StatusOK)
	fixture.manage(t, http.MethodGet, g2+"/diagnostics", "", http.StatusNotFound)
	// D-140: the delegate knows G1, so other G1 operations are forbidden rather than hidden.
	fixture.manage(t, http.MethodGet, g1, "", http.StatusForbidden)
	fixture.request(t, http.MethodGet, g1+"/credentials", "", "", "", http.StatusForbidden)
	issue(g1, "1", http.StatusForbidden)
	fixture.manage(t, http.MethodGet, "/api/v1/gateways", "", http.StatusForbidden)

	fixture.setScopedGrant(t, gatewayScope, "gateway.credential.rotate")
	fixture.request(t, http.MethodGet, g1+"/credentials", "", "", "", http.StatusOK)
	var credential gatewayCredentialResponse
	if err := json.Unmarshal(issue(g1, "1", http.StatusCreated), &credential); err != nil {
		t.Fatal(err)
	}
	fixture.request(t, http.MethodDelete, g1+"/credentials/"+credential.CredentialID,
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"2"}`, fixture.csrf, "", http.StatusOK)
	issue(g2, "1", http.StatusNotFound)
	fixture.manage(t, http.MethodGet, g1+"/diagnostics", "", http.StatusForbidden)
	fixture.request(t, http.MethodPut, g1+"/networks", `{"schemaVersion":1,"expectedRevision":"3","networks":[]}`,
		fixture.csrf, "", http.StatusBadRequest)
	fixture.request(t, http.MethodPut, g1+"/networks", `{"schemaVersion":1,"expectedRevision":"3","networks":[`+
		`{"gatewayNetworkIdentifier":"current","networkId":"`+fixture.current.String()+`","priority":0}]}`,
		fixture.csrf, "", http.StatusForbidden)

	// An Agency grant on gateway.read reads every Gateway of its Agencies, not a single one.
	fixture.setScopedGrant(t, agencyScope(fixture.agencies[fixture.current], fixture.agencies[fixture.sibling]), "gateway.read")
	fixture.manage(t, http.MethodGet, g1+"/diagnostics", "", http.StatusOK)
	fixture.manage(t, http.MethodGet, g2+"/diagnostics", "", http.StatusNotFound)
}
