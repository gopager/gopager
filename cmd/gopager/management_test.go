package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type managementNetworkFixture struct{ id uuid.UUID }

// ReadNetworkProtocol supplies only this declared fixture Network, not a production substitute.
func (f managementNetworkFixture) ReadNetworkProtocol(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, error) {
	if id != f.id {
		return "", authz.ErrNotFound
	}
	return protocols.POCSAG, ctx.Err()
}

func (f managementNetworkFixture) ScopeExists(ctx context.Context, tx platform.TransactionContext, scope authz.Scope) (bool, error) {
	if scope.Kind != "network" || !authz.ValidScope(scope) {
		return false, authz.ErrInvalidInput
	}
	for _, reference := range scope.References {
		if reference != f.id.String() {
			return false, nil
		}
	}
	return true, ctx.Err()
}

// newManagementStore isolates actual PostgreSQL HTTP tests even with an externally supplied database.
func newManagementStore(t *testing.T) *postgres.Store {
	return newManagementStoreWithFields(t, []string{"unit", "address"})
}

func newManagementStoreWithFields(t *testing.T, fields []string) *postgres.Store {
	t.Helper()
	dsn := os.Getenv("GOPAGER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration disabled; run with GOPAGER_INTEGRATION=1")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*config)
	name := "ap08_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA "`+name+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), `DROP SCHEMA "`+name+`" CASCADE`); err != nil {
			t.Error(err)
		}
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	scopedDSN := dsn + " search_path=" + name
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		address, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		query := address.Query()
		query.Set("search_path", name)
		address.RawQuery = query.Encode()
		scopedDSN = address.String()
	}
	store, _, err := postgres.NewStore(scopedDSN, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := store.Migrate(t.Context(), migrations.Files, platform.MigrationApply); err != nil {
		t.Fatal(err)
	}
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Bootstrap(t.Context(), store, repository, store, facts, "Operator", "initial-password",
		identity.BootstrapPermissions{Actions: authz.Actions(), Fields: fields}); err != nil {
		t.Fatal(err)
	}
	return store
}

type managementResponse struct {
	SchemaVersion int             `json:"schemaVersion"`
	Data          json.RawMessage `json:"data"`
	Code          string          `json:"code"`
	CSRFToken     string          `json:"csrfToken"`
}

// managementRequest exercises the actual HTTP JSON adapter and consumes/closes each response.
func managementRequest(t *testing.T, client *http.Client, origin, method, path, body, csrf, bearer string, expected int) managementResponse {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, origin+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	if csrf != "" {
		request.Header.Set(identity.CSRFHeaderName, csrf)
	}
	if bearer != "" {
		request.Header.Set("Authorization", bearer)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result managementResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected || result.SchemaVersion != 1 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%s %s returned %d/%s, expected %d; response=%s",
			method, path, response.StatusCode, result.Code, expected, result.Data)
	}
	return result
}

// managementResultID reads only the reference/revision DTO, not an internal persistence entity.
func managementResultID(t *testing.T, result managementResponse) (uuid.UUID, string) {
	t.Helper()
	var value struct {
		ID       uuid.UUID `json:"id"`
		Revision string    `json:"revision"`
	}
	if err := json.Unmarshal(result.Data, &value); err != nil || value.ID == uuid.Nil || value.Revision == "" {
		t.Fatalf("invalid result DTO: %v", err)
	}
	return value.ID, value.Revision
}

func TestManagementHTTPAndTLSReverseProxyWithHTTPUpstream(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "plain-http"
		if secure {
			name = "https-proxy-http-upstream"
		}
		t.Run(name, func(t *testing.T) {
			store := newManagementStore(t)
			backend := httptest.NewUnstartedServer(nil)
			t.Cleanup(backend.Close)
			origin := "http://" + backend.Listener.Addr().String()
			var proxy *httptest.Server
			if secure {
				target, err := url.Parse(origin)
				if err != nil {
					t.Fatal(err)
				}
				proxy = httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
				t.Cleanup(proxy.Close)
				origin = "https://" + proxy.Listener.Addr().String()
			}
			network := managementNetworkFixture{id: uuid.New()}
			handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store, network, network)
			if err != nil {
				t.Fatal(err)
			}
			backend.Config.Handler = handler
			backend.Start()
			client := backend.Client()
			if secure {
				proxy.StartTLS()
				client = proxy.Client()
			}
			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			client.Jar = jar
			login := managementRequest(t, client, origin, "POST", "/api/v1/auth/login",
				`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)
			if login.CSRFToken == "" {
				t.Fatal("login did not provide Session-bound CSRF")
			}
			managementRequest(t, client, origin, "POST", "/api/v1/unit-collections",
				`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Denied"}`, "", "", http.StatusForbidden)
			collection := managementRequest(t, client, origin, "POST", "/api/v1/unit-collections",
				`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Stations"}`, login.CSRFToken, "", http.StatusCreated)
			collectionID, _ := managementResultID(t, collection)
			unitInput := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","name":"Station","collectionIds":["` + collectionID.String() + `"]}`
			created := managementRequest(t, client, origin, "POST", "/api/v1/units", unitInput, login.CSRFToken, "", http.StatusCreated)
			unitID, _ := managementResultID(t, created)
			retry := managementRequest(t, client, origin, "POST", "/api/v1/units", unitInput, login.CSRFToken, "", http.StatusCreated)
			if retryID, _ := managementResultID(t, retry); retryID != unitID {
				t.Fatal("HTTP creation retry created a second Unit")
			}
			for _, membership := range []string{"", `,"collectionIds":null`} {
				managementRequest(t, client, origin, "PUT", "/api/v1/units/"+unitID.String(),
					`{"schemaVersion":1,"expectedRevision":"1","name":"Silent clear"`+membership+`}`, login.CSRFToken, "", http.StatusBadRequest)
			}
			managementRequest(t, client, origin, "PUT", "/api/v1/units/"+unitID.String(),
				`{"schemaVersion":1,"expectedRevision":"1","name":"Station renamed","collectionIds":["`+collectionID.String()+`"]}`, login.CSRFToken, "", http.StatusOK)
			managementRequest(t, client, origin, "GET", "/api/v1/units?offset=0&limit=1", "", "", "", http.StatusOK)
			managementRequest(t, client, origin, "GET", "/api/v1/units?networkId=ignored", "", "", "", http.StatusBadRequest)
			managementRequest(t, client, origin, "GET", "/api/v1/units", "", "", "Bearer malformed", http.StatusUnauthorized)
			ruleValues := `"networkId":"` + network.id.String() + `","unitId":"` + unitID.String() +
				`","protocol":"pocsag","selector":{"from":10,"to":20,"mode":"function-bit","functions":[1,3]},"enabled":true`
			managementRequest(t, client, origin, "POST", "/api/v1/address-rules/preview", `{"schemaVersion":1,`+ruleValues+`}`, "", "", http.StatusOK)
			rule := managementRequest(t, client, origin, "POST", "/api/v1/address-rules",
				`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`",`+ruleValues+`}`, login.CSRFToken, "", http.StatusCreated)
			ruleID, _ := managementResultID(t, rule)
			managementRequest(t, client, origin, "GET", "/api/v1/address-rules/"+ruleID.String(), "", "", "", http.StatusOK)
			managementRequest(t, client, origin, "GET", "/api/v1/address-rules?limit=1", "", "", "", http.StatusOK)
			managementRequest(t, client, origin, "PUT", "/api/v1/address-rules/"+ruleID.String(),
				`{"schemaVersion":1,"expectedRevision":"1",`+ruleValues+`}`, login.CSRFToken, "", http.StatusOK)
			managementRequest(t, client, origin, "DELETE", "/api/v1/address-rules/"+ruleID.String(),
				`{"schemaVersion":1,"expectedRevision":"1"}`, login.CSRFToken, "", http.StatusConflict)
			deleted := managementRequest(t, client, origin, "DELETE", "/api/v1/address-rules/"+ruleID.String(),
				`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusAccepted)
			if !strings.Contains(string(deleted.Data), `"status":"pending"`) {
				t.Fatal("rule deletion falsely claimed historical completion")
			}
			managementRequest(t, client, origin, "PUT", "/api/v1/unit-collections/"+collectionID.String(),
				`{"schemaVersion":1,"expectedRevision":"1","name":"Stations renamed"}`, login.CSRFToken, "", http.StatusOK)
			managementRequest(t, client, origin, "GET", "/api/v1/unit-collections/"+collectionID.String(), "", "", "", http.StatusOK)
			managementRequest(t, client, origin, "GET", "/api/v1/unit-collections", "", "", "", http.StatusOK)
			managementRequest(t, client, origin, "DELETE", "/api/v1/unit-collections/"+collectionID.String(),
				`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusOK)
			managementRequest(t, client, origin, "DELETE", "/api/v1/units/"+unitID.String(),
				`{"schemaVersion":1,"expectedRevision":"3"}`, login.CSRFToken, "", http.StatusAccepted)
		})
	}
}

func TestManagementRejectsInvalidShapes(t *testing.T) {
	for _, body := range []string{
		`{"schemaVersion":1,"unexpected":true}`, `{} {}`, `null`,
		`{"name":"` + strings.Repeat("x", 64*1024) + `"}`,
	} {
		request := httptest.NewRequest("POST", "/api/v1/units", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		var input unitMutation
		err := decodeManagementJSON(request, &input)
		if body == "null" {
			if err != nil || input.SchemaVersion != 0 {
				t.Fatal("null input lost its invalid schema marker")
			}
		} else if err == nil {
			t.Fatal("invalid/oversized JSON accepted")
		}
	}

}

func TestManagementUnwiredProductionNetworkOwnerFailsExplicitly(t *testing.T) {
	store := newManagementStore(t)
	handler, err := composeApplicationAPI("http://example.test", identity.DefaultLoginRateLimits(), store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := func(path, body, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", "http://example.test"+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://example.test")
		if csrf != "" {
			request.Header.Set(identity.CSRFHeaderName, csrf)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	login := call("/api/v1/auth/login", `{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", nil)
	var authentication managementResponse
	if err := json.Unmarshal(login.Body.Bytes(), &authentication); err != nil || login.Code != http.StatusOK {
		t.Fatalf("standalone production login failed: %v", err)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("login did not set the Session cookie")
	}
	created := call("/api/v1/units", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Station","collectionIds":[]}`,
		authentication.CSRFToken, cookies[0])
	var unit managementResponse
	if err := json.Unmarshal(created.Body.Bytes(), &unit); err != nil || created.Code != http.StatusCreated {
		t.Fatalf("standalone production Unit creation failed: %v", err)
	}
	unitID, _ := managementResultID(t, unit)
	rule := call("/api/v1/address-rules/preview", `{"schemaVersion":1,"networkId":"`+uuid.NewString()+`","unitId":"`+unitID.String()+
		`","protocol":"pocsag","selector":{"from":10,"to":20,"mode":"function-bit"},"enabled":true}`, "", cookies[0])
	var failure managementResponse
	if err := json.Unmarshal(rule.Body.Bytes(), &failure); err != nil || rule.Code != http.StatusServiceUnavailable || failure.Code != "AUTHORITY_UNAVAILABLE" {
		t.Fatalf("unwired production Network owner returned success/default: %d %v", rule.Code, err)
	}
}

func TestManagementErrorMapping(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{authz.ErrForbidden, http.StatusForbidden, "FORBIDDEN"},
		{authz.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{authz.ErrRevisionConflict, http.StatusConflict, "REVISION_CONFLICT"},
		{platform.NewFailure(platform.CodeResourceLimit, errors.New("private infrastructure detail")), http.StatusRequestEntityTooLarge, "RESOURCE_LIMIT"},
	} {
		recorder := httptest.NewRecorder()
		actual := test.err
		if test.code != "RESOURCE_LIMIT" {
			actual = platform.NewFailure(platform.CodeStorageUnavailable, actual)
		}
		writeManagementError(recorder, actual)
		var result managementResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != test.status || result.Code != test.code {
			t.Fatalf("domain cause hidden by storage wrapper: %d", recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "private infrastructure detail") {
			t.Fatal("private wrapped error escaped HTTP classification")
		}
	}
}
