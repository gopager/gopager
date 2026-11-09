package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
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
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

type managementNetworkFixture struct{ id, agency uuid.UUID }

// ReadNetwork supplies only this declared fixture Network, not a production substitute.
func (f managementNetworkFixture) ReadNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	if id != f.id {
		return "", uuid.Nil, authz.ErrNotFound
	}
	return protocols.POCSAG, f.agency, ctx.Err()
}

// seedAgency inserts an Agency row for fixtures that create Networks and Units with raw SQL.
func seedAgency(t *testing.T, store *postgres.Store) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO agencies(id,name) VALUES($1,'Fixture agency')`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// migratedTemplate is the database with the release migrations applied, built once per process.
var migratedTemplate = testpostgres.NewTemplate("migrated", func(ctx context.Context, dsn string) error {
	store, _, err := postgres.NewStore(dsn, true)
	if err != nil {
		return err
	}
	migrateErr := store.Migrate(ctx, migrations.Files, platform.MigrationApply)
	if err := store.Close(); err != nil && migrateErr == nil {
		return err
	}
	return migrateErr
})

// newManagementStore isolates actual PostgreSQL HTTP tests even with an externally supplied database.
func newManagementStore(t *testing.T) *postgres.Store {
	return newManagementStoreWithFields(t, []string{"unit", "address"})
}

func newManagementStoreWithFields(t *testing.T, fields []string) *postgres.Store {
	t.Helper()
	scopedDSN := migratedTemplate.Database(t)
	store, _, err := postgres.NewStore(scopedDSN, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
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

// managementRequestWithoutOrigin sends a Cookie-authenticated POST that omits
// the Origin header, so only Session state and the optional CSRF token are
// presented, and asserts the response status and error envelope.
func managementRequestWithoutOrigin(t *testing.T, client *http.Client, origin, path, body, csrf string, expected int) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, origin+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		request.Header.Set(identity.CSRFHeaderName, csrf)
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
	if response.StatusCode != expected || result.SchemaVersion != 1 {
		t.Fatalf("POST %s without Origin returned %d/%s, expected %d", path, response.StatusCode, result.Code, expected)
	}
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

// TestManagementHTTPAPI walks the management routes over plain HTTP. The TLS reverse proxy path
// has one dedicated test, TestTLSReverseProxyServesLoginAndLogoutOverProductionWiring.
func TestManagementHTTPAPI(t *testing.T) {
	t.Parallel()
	store := newManagementStore(t)
	agency := `"agencyId":"` + seedAgency(t, store).String() + `",`
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	network := managementNetworkFixture{id: uuid.New(), agency: uuid.New()}
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), mustComposeCore(t, store), network)
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
	unitInput := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `",` + agency + `"name":"Station","collectionIds":["` + collectionID.String() + `"],"organizationId":null,"settings":{}}`
	created := managementRequest(t, client, origin, "POST", "/api/v1/units", unitInput, login.CSRFToken, "", http.StatusCreated)
	unitID, _ := managementResultID(t, created)
	retry := managementRequest(t, client, origin, "POST", "/api/v1/units", unitInput, login.CSRFToken, "", http.StatusCreated)
	if retryID, _ := managementResultID(t, retry); retryID != unitID {
		t.Fatal("HTTP creation retry created a second Unit")
	}
	for _, membership := range []string{"", `,"collectionIds":null`} {
		managementRequest(t, client, origin, "PUT", "/api/v1/units/"+unitID.String(),
			`{"schemaVersion":1,"expectedRevision":"1",`+agency+`"name":"Silent clear","organizationId":null,"settings":{}`+membership+`}`, login.CSRFToken, "", http.StatusBadRequest)
	}
	managementRequest(t, client, origin, "PUT", "/api/v1/units/"+unitID.String(),
		`{"schemaVersion":1,"expectedRevision":"1",`+agency+`"name":"Station renamed","collectionIds":["`+collectionID.String()+`"],"organizationId":null,"settings":{}}`, login.CSRFToken, "", http.StatusOK)
	defaults := `{"color":{"mode":"value","value":"#AABBCC"},"icon":{"mode":"unset"},"functionMeanings":{"mode":"unset"}}`
	organization := managementRequest(t, client, origin, "POST", "/api/v1/organizations",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Radio Operations","defaults":`+defaults+`}`,
		login.CSRFToken, "", http.StatusCreated)
	organizationID, _ := managementResultID(t, organization)
	templateSettings := `{"color":{"mode":"inherit"},"icon":{"mode":"inherit"},"functionMeanings":{"mode":"inherit"}}`
	template := managementRequest(t, client, origin, "POST", "/api/v1/unit-templates",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Radio","organizationId":"`+
			organizationID.String()+`","settings":`+templateSettings+`}`, login.CSRFToken, "", http.StatusCreated)
	templateID, _ := managementResultID(t, template)
	copied := managementRequest(t, client, origin, "POST", "/api/v1/units/from-template",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`",`+agency+`"templateId":"`+templateID.String()+
			`","name":"Copied Radio","collectionIds":[]}`, login.CSRFToken, "", http.StatusCreated)
	copiedID, _ := managementResultID(t, copied)
	bulkSettings := `{"color":{"mode":"value","value":"#123456"},"icon":{"mode":"unset"},"functionMeanings":{"mode":"unset"}}`
	bulkItems := `"operation":"update","items":[{"id":"` + copiedID.String() +
		`","expectedRevision":"1","settings":` + bulkSettings + `}]}`
	bulk := `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `",` + bulkItems
	bulkPreview := `{"schemaVersion":1,` + bulkItems
	managementRequest(t, client, origin, "POST", "/api/v1/units/bulk/preview",
		bulkPreview, "", "", http.StatusForbidden)
	managementRequestWithoutOrigin(t, client, origin, "/api/v1/units/bulk/preview",
		bulkPreview, login.CSRFToken, http.StatusForbidden)
	preview := managementRequest(t, client, origin, "POST", "/api/v1/units/bulk/preview",
		bulkPreview, login.CSRFToken, "", http.StatusOK)
	var previewData struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(preview.Data, &previewData); err != nil || !previewData.Valid {
		t.Fatalf("bulk preview was invalid: %v", err)
	}
	managementRequest(t, client, origin, "POST", "/api/v1/units/bulk",
		bulk, login.CSRFToken, "", http.StatusOK)
	managementRequest(t, client, origin, "DELETE", "/api/v1/organizations/"+organizationID.String(),
		`{"schemaVersion":1,"expectedRevision":"1"}`, login.CSRFToken, "", http.StatusOK)
	managementRequest(t, client, origin, "GET", "/api/v1/unit-templates/"+templateID.String(),
		"", "", "", http.StatusOK)
	// Suppression/correction routes return only the safe version-1 error shape.
	managementRequest(t, client, origin, "GET", "/api/v1/suppressions", "", "", "", http.StatusOK)
	unknownTransmission := uuid.NewString()
	missingTarget := managementRequest(t, client, origin, "POST", "/api/v1/suppressions",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","transmissionId":"`+unknownTransmission+
			`","targetPosition":0}`, login.CSRFToken, "", http.StatusNotFound)
	if missingTarget.Code != "NOT_FOUND" || strings.Contains(string(missingTarget.Data), unknownTransmission) {
		t.Fatalf("unknown suppression target was not a safe NOT_FOUND: %+v", missingTarget)
	}
	unknownField := managementRequest(t, client, origin, "POST", "/api/v1/suppressions",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","transmissionId":"`+unknownTransmission+
			`","targetPosition":0,"selector":"*"}`,
		login.CSRFToken, "", http.StatusBadRequest)
	if unknownField.Code != "INVALID_INPUT" {
		t.Fatalf("broad suppression selector was not rejected: %+v", unknownField)
	}
	unknownCorrection := uuid.NewString()
	for _, request := range []struct{ method, path, body, csrf string }{
		{"GET", "/api/v1/assignment-corrections/" + unknownCorrection, "", ""},
		{"POST", "/api/v1/assignment-corrections/" + unknownCorrection + "/retry",
			`{"schemaVersion":1,"commandId":"` + uuid.NewString() + `"}`, login.CSRFToken},
	} {
		missing := managementRequest(t, client, origin, request.method, request.path, request.body,
			request.csrf, "", http.StatusNotFound)
		if missing.Code != "NOT_FOUND" || strings.Contains(string(missing.Data), unknownCorrection) {
			t.Fatalf("%s %s was not a safe NOT_FOUND: %+v", request.method, request.path, missing)
		}
	}
	managementRequest(t, client, origin, "GET", "/api/v1/units/"+copiedID.String(),
		"", "", "", http.StatusOK)
	managementRequest(t, client, origin, "GET", "/api/v1/units?offset=0&limit=1", "", "", "", http.StatusOK)
	managementRequest(t, client, origin, "GET", "/api/v1/units?networkId=ignored", "", "", "", http.StatusBadRequest)
	managementRequest(t, client, origin, "GET", "/api/v1/units", "", "", "Bearer malformed", http.StatusUnauthorized)
	ruleValues := `"networkId":"` + network.id.String() + `","unitId":"` + unitID.String() +
		`","protocol":"pocsag","selector":{"from":10,"to":20,"mode":"function-bit","functions":[1,3]},"enabled":true`
	managementRequest(t, client, origin, "POST", "/api/v1/address-rules/preview", `{"schemaVersion":1,`+ruleValues+`}`, "", "", http.StatusForbidden)
	managementRequestWithoutOrigin(t, client, origin, "/api/v1/address-rules/preview",
		`{"schemaVersion":1,`+ruleValues+`}`, login.CSRFToken, http.StatusForbidden)
	managementRequest(t, client, origin, "POST", "/api/v1/address-rules/preview", `{"schemaVersion":1,`+ruleValues+`}`, login.CSRFToken, "", http.StatusOK)
	rule := managementRequest(t, client, origin, "POST", "/api/v1/address-rules",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`",`+ruleValues+`}`, login.CSRFToken, "", http.StatusCreated)
	ruleID, _ := managementResultID(t, rule)
	readRule := managementRequest(t, client, origin, "GET", "/api/v1/address-rules/"+ruleID.String(), "", "", "", http.StatusOK)
	if !strings.Contains(string(readRule.Data), `"encrypted":false`) {
		t.Fatalf("rule created without encrypted is not returned as false: %s", readRule.Data)
	}
	managementRequest(t, client, origin, "GET", "/api/v1/address-rules?limit=1", "", "", "", http.StatusOK)
	managementRequest(t, client, origin, "PUT", "/api/v1/address-rules/"+ruleID.String(),
		`{"schemaVersion":1,"expectedRevision":"1","encrypted":true,`+ruleValues+`}`, login.CSRFToken, "", http.StatusOK)
	readRule = managementRequest(t, client, origin, "GET", "/api/v1/address-rules/"+ruleID.String(), "", "", "", http.StatusOK)
	if !strings.Contains(string(readRule.Data), `"encrypted":true`) || !strings.Contains(string(readRule.Data), `"revision":"2"`) {
		t.Fatalf("encrypted update not returned with bumped revision: %s", readRule.Data)
	}
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
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `UPDATE identity_grants
			SET actions=actions-'unit-template.read'-'organization.create'
			WHERE subject_kind='principal'`); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE identity_control
			SET access_revision=access_revision+1 WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var unitsBefore int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM units`).Scan(&unitsBefore)
	}); err != nil {
		t.Fatal(err)
	}
	deniedTemplateCopy := managementRequest(t, client, origin, "POST", "/api/v1/units/from-template",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`",`+agency+`"templateId":"`+templateID.String()+
			`","name":"Denied Copy","collectionIds":[]}`, login.CSRFToken, "", http.StatusForbidden)
	if deniedTemplateCopy.Code != "FORBIDDEN" || strings.Contains(string(deniedTemplateCopy.Data), templateID.String()) ||
		strings.Contains(string(deniedTemplateCopy.Data), organizationID.String()) {
		t.Fatalf("template denial exposed cross-scope data: %+v", deniedTemplateCopy)
	}
	var unitsAfter int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM units`).Scan(&unitsAfter)
	}); err != nil {
		t.Fatal(err)
	}
	if unitsAfter != unitsBefore {
		t.Fatalf("denied template copy created a Unit: before=%d after=%d", unitsBefore, unitsAfter)
	}
	var organizationsBefore int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM organizations`).
			Scan(&organizationsBefore)
	}); err != nil {
		t.Fatal(err)
	}
	inlineOrganization := `{"name":"Denied Inline","defaults":{"color":{"mode":"unset"},` +
		`"icon":{"mode":"unset"},"functionMeanings":{"mode":"unset"}}}`
	deniedInline := managementRequest(t, client, origin, "POST", "/api/v1/units",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`",`+agency+`"name":"Denied Inline Unit",`+
			`"collectionIds":[],"settings":{},"newOrganization":`+inlineOrganization+`}`,
		login.CSRFToken, "", http.StatusForbidden)
	if deniedInline.Code != "FORBIDDEN" {
		t.Fatalf("inline Organization denial had unsafe HTTP code: %+v", deniedInline)
	}
	var organizationsAfter int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM organizations`).
			Scan(&organizationsAfter)
	}); err != nil {
		t.Fatal(err)
	}
	if organizationsAfter != organizationsBefore {
		t.Fatalf("denied inline creation persisted an Organization: before=%d after=%d",
			organizationsBefore, organizationsAfter)
	}
}

func TestManagementProductionNetworkOwnerHidesUnknownNetwork(t *testing.T) {
	t.Parallel()
	store := newManagementStore(t)
	handler, err := composeApplicationAPI("http://example.test", identity.DefaultLoginRateLimits(), mustComposeCore(t, store), nil)
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
	created := call("/api/v1/units", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","agencyId":"`+seedAgency(t, store).String()+
		`","name":"Station","collectionIds":[],"organizationId":null,"settings":{}}`, authentication.CSRFToken, cookies[0])
	var unit managementResponse
	if err := json.Unmarshal(created.Body.Bytes(), &unit); err != nil || created.Code != http.StatusCreated {
		t.Fatalf("standalone production Unit creation failed: %v", err)
	}
	unitID, _ := managementResultID(t, unit)
	rule := call("/api/v1/address-rules/preview", `{"schemaVersion":1,"networkId":"`+uuid.NewString()+`","unitId":"`+unitID.String()+
		`","protocol":"pocsag","selector":{"from":10,"to":20,"mode":"function-bit"},"enabled":true}`, authentication.CSRFToken, cookies[0])
	var failure managementResponse
	if err := json.Unmarshal(rule.Body.Bytes(), &failure); err != nil || rule.Code != http.StatusForbidden || failure.Code != "FORBIDDEN" {
		t.Fatalf("production Network owner exposed an unauthorized Network: %d %v", rule.Code, err)
	}
}
