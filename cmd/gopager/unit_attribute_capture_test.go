package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
)

// TestUnitAttributesAreCapturedWithTheRunsSource proves on the production wiring that a
// target-scoped Workflow reads the effective Unit attribute of the target's Unit, that editing the
// attribute afterwards neither re-admits processing nor changes the stored result, and that a
// later Transmission captures the new value.
func TestUnitAttributesAreCapturedWithTheRunsSource(t *testing.T) {
	t.Parallel()
	store := newManagementStoreWithFields(t, []string{"unit", "address", "payload", "gateway", "derived"})
	network, gateway, agency := uuid.New(), uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES($2,'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'Attribute Network','pocsag',a.id FROM a`, network, agency); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id) VALUES($1)`, gateway); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,priority) VALUES($1,$2,'attributes',$3,0)`,
			uuid.New(), gateway, network)
		return err
	}); err != nil {
		t.Fatal(err)
	}
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
	send := func(method, path, body string, status int) managementResponse {
		t.Helper()
		return managementRequest(t, client, origin, method, path, body, login.CSRFToken, "", status)
	}

	for _, schema := range []struct{ key, subject string }{{"station", "unit"}, {"routing", "message"}} {
		send(http.MethodPost, "/api/v1/attribute-schemas", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+
			`","key":"`+schema.key+`","label":"`+schema.key+`","subject":"`+schema.subject+`","agencies":["`+agency.String()+
			`"],"fields":{"town":{"kind":"string"}}}`, http.StatusCreated)
	}
	unitBody := func(town string) string {
		return `"agencyId":"` + agency.String() + `","name":"Engine","collectionIds":[],"organizationId":null,` +
			`"settings":{"attributes":{"station":{"town":{"mode":"value","value":{"kind":"string","value":"` + town + `"}}}}}}`
	}
	unit, _ := managementResultID(t, send(http.MethodPost, "/api/v1/units",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`",`+unitBody("A"), http.StatusCreated))
	send(http.MethodPost, "/api/v1/address-rules", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","networkId":"`+
		network.String()+`","unitId":"`+unit.String()+`","protocol":"pocsag",`+
		`"selector":{"from":10,"to":20,"mode":"function-bit","functions":[1]},"enabled":true}`, http.StatusCreated)

	workflow, _ := managementResultID(t, send(http.MethodPost, "/api/v1/workflows", `{"schemaVersion":1,"commandId":"`+
		uuid.NewString()+`","name":"Town","scope":`+workflowScopeJSON(agency)+`,"enabled":true}`, http.StatusCreated))
	protocol := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	pocsag, err := processing.Literal(processing.StringValue, "pocsag")
	if err != nil {
		t.Fatal(err)
	}
	town := processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", "town"}}
	definition := processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocol, Value: &pocsag},
		Outputs:  map[string]map[string]processing.Assignment{"routing": {"town": {Field: &town}}}}
	versionInput, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1,
		CommandID: uuid.New(), ExpectedRevision: 1, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	version, _ := managementResultID(t, send(http.MethodPost, "/api/v1/workflows/"+workflow.String()+"/versions",
		string(versionInput), http.StatusCreated))
	send(http.MethodPost, "/api/v1/workflows/"+workflow.String()+"/versions/"+version.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"2"}`, http.StatusOK)

	var credential gatewayCredentialResponse
	rawProcessingRequest(t, client, origin, "/api/v1/gateways/"+gateway.String()+"/credentials",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"1"}`,
		identity.CSRFHeaderName, login.CSRFToken, http.StatusCreated, &credential)
	submit := func(id string) uuid.UUID {
		t.Helper()
		var submitted struct {
			Data struct {
				TransmissionID uuid.UUID `json:"transmissionId"`
			} `json:"data"`
		}
		rawProcessingRequest(t, client, origin, "/api/v1/transmissions",
			`{"schemaVersion":1,"submissionId":"`+id+`","protocol":"pocsag","observationTime":"2026-11-04T12:00:00Z",`+
				`"gatewayNetworkIdentifier":"attributes","payload":{"form":"text","value":"`+id+`"},`+
				`"transmissionTargets":[{"address":15,"function":1}]}`,
			"Authorization", "Bearer "+credential.Secret, http.StatusAccepted, &submitted)
		return submitted.Data.TransmissionID
	}
	repository, err := processingstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	service, err := worker.NewWithPublisher(store, repository, runtime.workerSource, processing.CoreRegex{},
		uuid.New(), runtime.workerPublisher)
	if err != nil {
		t.Fatal(err)
	}
	runDue := func() {
		t.Helper()
		var due []processing.Candidate
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			page, err := repository.SelectDueWork(tx.Context(), tx, 10)
			due = page
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for _, candidate := range due {
			if err := service.ExecuteWork(t.Context(), candidate); err != nil {
				t.Fatalf("run failed: %s", causeChain(err))
			}
		}
	}
	publishedTown := func(transmission uuid.UUID) string {
		t.Helper()
		var value string
		if err := queryOne(t, store, `SELECT COALESCE(r.evaluation #>> '{targetResults,0,fields,routing,town,value,value}','')
			FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
			WHERE c.transmission_id=$1 AND r.published_at IS NOT NULL`, transmission).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	counts := func() [3]int {
		t.Helper()
		var result [3]int
		if err := queryOne(t, store, `SELECT (SELECT count(*) FROM processing_candidates),
			(SELECT count(*) FROM processing_results), (SELECT count(*) FROM assignment_pending_scopes)`).
			Scan(&result[0], &result[1], &result[2]); err != nil {
			t.Fatal(err)
		}
		return result
	}

	first := submit("first")
	runDue()
	if got := publishedTown(first); got != "A" {
		t.Fatalf("first result town = %q, want A", got)
	}
	before := counts()

	send(http.MethodPut, "/api/v1/units/"+unit.String(), `{"schemaVersion":1,"expectedRevision":"1",`+unitBody("C"), http.StatusOK)
	runDue()
	if after := counts(); after != before {
		t.Fatalf("candidates, results and pending scopes = %v after the edit, were %v", after, before)
	}
	if got := publishedTown(first); got != "A" {
		t.Fatalf("result town after the edit = %q, want A", got)
	}

	second := submit("second")
	runDue()
	if got := publishedTown(second); got != "C" {
		t.Fatalf("second result town = %q, want C", got)
	}
}

// queryOne runs a one-row query in a read transaction of the store.
func queryOne(t *testing.T, store *postgres.Store, statement string, args ...any) rowScanner {
	t.Helper()
	return rowScanner{t: t, store: store, statement: statement, args: args}
}

// rowScanner scans the single row of a statement inside a transaction of the store.
type rowScanner struct {
	t         *testing.T
	store     *postgres.Store
	statement string
	args      []any
}

func (r rowScanner) Scan(dest ...any) error {
	return r.store.WithinTransaction(r.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := r.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), r.statement, r.args...).Scan(dest...)
	})
}
