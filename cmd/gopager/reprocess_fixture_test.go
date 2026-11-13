package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/reprocess"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/testutil/attributeschemas"
)

// reprocessFixture runs the production composition end to end: HTTP management, Gateway intake,
// the Processing worker with its publisher and the Reprocess runner, on one PostgreSQL database.
type reprocessFixture struct {
	t          *testing.T
	store      *postgres.Store
	shared     *core
	runtime    *processingAPIRuntime
	repository *processingstore.Store
	worker     *worker.Service
	client     *http.Client
	origin     string
	csrf       string
	agency     uuid.UUID
	secret     string
	workflow   uuid.UUID
	revision   int64
	// schema is the Attribute schema key the Workflow's versions write; empty means "incident".
	schema string
}

// newReprocessFixture seeds one Agency, Network and Gateway, an Attribute schema "incident" with
// an integer field "level" and an enabled Workflow assigned to the Agency without a version.
func newReprocessFixture(t *testing.T) *reprocessFixture {
	t.Helper()
	store := newManagementStoreWithFields(t, []string{"unit", "address", "payload", "gateway", "derived"})
	network, gateway, agency := uuid.New(), uuid.New(), uuid.New()
	f := &reprocessFixture{t: t, store: store, agency: agency}
	f.exec(`WITH a AS (INSERT INTO agencies(id,name) VALUES($2,'Reprocess agency') RETURNING id)
		INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'Reprocess Network','pocsag',a.id FROM a`, network, agency)
	f.exec(`INSERT INTO gateways(id) VALUES($1)`, gateway)
	f.exec(`INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
		VALUES($1,$2,'reprocess',$3,0)`, uuid.New(), gateway, network)
	attributeschemas.Insert(t, store, "incident", map[string]attributes.Field{"level": {Kind: processing.IntegerValue}})
	f.shared = mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), f.shared, "")
	if err != nil {
		t.Fatal(err)
	}
	f.runtime = runtime
	t.Cleanup(func() {
		if err := runtime.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	f.origin = "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(f.origin, identity.DefaultLoginRateLimits(), f.shared, nil, runtime.routes)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	f.client = backend.Client()
	if f.client.Jar, err = cookiejar.New(nil); err != nil {
		t.Fatal(err)
	}
	login := managementRequest(t, f.client, f.origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)
	f.csrf = login.CSRFToken
	created := f.post("/api/v1/workflows", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+
		`","name":"Reprocess","scope":`+workflowScopeJSON(agency)+`,"enabled":true}`, http.StatusCreated)
	f.workflow, f.revision = f.reference(created)
	var credential gatewayCredentialResponse
	rawProcessingRequest(t, f.client, f.origin, "/api/v1/gateways/"+gateway.String()+"/credentials",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"1"}`,
		identity.CSRFHeaderName, f.csrf, http.StatusCreated, &credential)
	f.secret = credential.Secret
	f.repository, err = processingstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	f.worker, err = worker.NewWithPublisher(store, f.repository, runtime.workerSource, processing.CoreRegex{},
		uuid.New(), runtime.workerPublisher)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// schemaKey is the Attribute schema the Workflow writes.
func (f *reprocessFixture) schemaKey() string {
	if f.schema == "" {
		return "incident"
	}
	return f.schema
}

// otherWorkflow creates a second enabled Workflow on the same Agency that writes the integer
// field "level" of its own Attribute schema, and returns a fixture view that manages it. The view
// shares the database, HTTP session and worker with f.
func (f *reprocessFixture) otherWorkflow(name, schema string) *reprocessFixture {
	f.t.Helper()
	attributeschemas.Insert(f.t, f.store, schema, map[string]attributes.Field{"level": {Kind: processing.IntegerValue}})
	other := *f
	other.schema = schema
	created := f.post("/api/v1/workflows", `{"schemaVersion":1,"commandId":"`+uuid.NewString()+
		`","name":"`+name+`","scope":`+workflowScopeJSON(f.agency)+`,"enabled":true}`, http.StatusCreated)
	other.workflow, other.revision = f.reference(created)
	return &other
}

// replayModesOf lists the replay modes of the Workflow's processing.completed facts of the Transmission in order.
func (f *reprocessFixture) replayModesOf(transmission uuid.UUID) []string {
	f.t.Helper()
	return f.queryStrings(`SELECT envelope->>'replayMode' FROM domain_events
		WHERE type='processing.completed' AND envelope->'data'->>'transmissionId'=$1
			AND envelope->'data'->>'workflowId'=$2 ORDER BY seq`, transmission.String(), f.workflow.String())
}

// resultsOf counts the Workflow's results of the Transmission by publication state.
func (f *reprocessFixture) resultsOf(transmission uuid.UUID) (published, retracted int) {
	f.t.Helper()
	published = f.count(`SELECT count(*) FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
		WHERE c.transmission_id=$1 AND c.workflow_id=$2 AND r.published_at IS NOT NULL`, transmission, f.workflow)
	retracted = f.count(`SELECT count(*) FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
		WHERE c.transmission_id=$1 AND c.workflow_id=$2 AND r.retracted_at IS NOT NULL`, transmission, f.workflow)
	return published, retracted
}

// runWorkerFor executes the due candidates of this Workflow only, leaving others pending.
func (f *reprocessFixture) runWorkerFor() {
	f.t.Helper()
	var due []processing.Candidate
	if err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		page, err := f.repository.SelectDueWork(tx.Context(), tx, 50)
		due = page
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	for _, candidate := range due {
		if candidate.WorkflowID != f.workflow {
			continue
		}
		if err := f.worker.ExecuteWork(f.t.Context(), candidate); err != nil {
			f.t.Fatalf("candidate run failed: %s", causeChain(err))
		}
	}
}

// exec runs one statement in its own transaction.
func (f *reprocessFixture) exec(query string, args ...any) {
	f.t.Helper()
	if err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), query, args...)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
}

// queryStrings returns the first column of every row as text.
func (f *reprocessFixture) queryStrings(query string, args ...any) []string {
	f.t.Helper()
	var values []string
	if err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		values = nil
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	}); err != nil {
		f.t.Fatal(err)
	}
	return values
}

// count returns the single number a query yields.
func (f *reprocessFixture) count(query string, args ...any) int {
	f.t.Helper()
	values := f.queryStrings(query, args...)
	if len(values) != 1 {
		f.t.Fatalf("%q returned %d rows, want 1", query, len(values))
	}
	number, err := strconv.Atoi(values[0])
	if err != nil {
		f.t.Fatal(err)
	}
	return number
}

func (f *reprocessFixture) post(path, body string, status int) managementResponse {
	f.t.Helper()
	return managementRequest(f.t, f.client, f.origin, http.MethodPost, path, body, f.csrf, "", status)
}

func (f *reprocessFixture) get(path string, status int) managementResponse {
	f.t.Helper()
	return managementRequest(f.t, f.client, f.origin, http.MethodGet, path, "", "", "", status)
}

// reference reads the ID and the revision of a management command response.
func (f *reprocessFixture) reference(response managementResponse) (uuid.UUID, int64) {
	f.t.Helper()
	id, revision := managementResultID(f.t, response)
	parsed, err := strconv.ParseInt(revision, 10, 64)
	if err != nil {
		f.t.Fatal(err)
	}
	return id, parsed
}

// definition selects every POCSAG Transmission and writes incident.level as the given literal.
func (f *reprocessFixture) definition(level string) processing.Definition {
	f.t.Helper()
	return f.definitionFor("pocsag", level)
}

// definitionFor is definition selecting Transmissions of the given protocol.
func (f *reprocessFixture) definitionFor(selected, level string) processing.Definition {
	f.t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	protocol, err := processing.Literal(processing.StringValue, selected)
	if err != nil {
		f.t.Fatal(err)
	}
	literal, err := processing.Literal(processing.IntegerValue, level)
	if err != nil {
		f.t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &field, Value: &protocol},
		Outputs:  map[string]map[string]processing.Assignment{f.schemaKey(): {"level": {Literal: &literal}}}}
}

// saveVersion saves a version writing the level and returns its ID.
func (f *reprocessFixture) saveVersion(level string) uuid.UUID {
	f.t.Helper()
	return f.saveDefinition(f.definition(level))
}

// saveRejectingVersion saves a version that selects no POCSAG Transmission, so a run of it
// completes as not selected.
func (f *reprocessFixture) saveRejectingVersion() uuid.UUID {
	f.t.Helper()
	return f.saveDefinition(f.definitionFor("flex", "9"))
}

func (f *reprocessFixture) saveDefinition(definition processing.Definition) uuid.UUID {
	f.t.Helper()
	body, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1, CommandID: uuid.New(),
		ExpectedRevision: f.revision, Definition: definition})
	if err != nil {
		f.t.Fatal(err)
	}
	id, revision := f.reference(f.post("/api/v1/workflows/"+f.workflow.String()+"/versions", string(body), http.StatusCreated))
	f.revision = revision
	return id
}

// activate makes the version active.
func (f *reprocessFixture) activate(version uuid.UUID) {
	f.t.Helper()
	response := f.post("/api/v1/workflows/"+f.workflow.String()+"/versions/"+version.String()+"/activate",
		`{"schemaVersion":1,"expectedRevision":"`+strconv.FormatInt(f.revision, 10)+`"}`, http.StatusOK)
	_, f.revision = f.reference(response)
}

// updateWorkflow replaces the Workflow's metadata, which bumps its revision.
func (f *reprocessFixture) updateWorkflow(name string, agencies ...uuid.UUID) {
	f.t.Helper()
	response := managementRequest(f.t, f.client, f.origin, http.MethodPut, "/api/v1/workflows/"+f.workflow.String(),
		`{"schemaVersion":1,"expectedRevision":"`+strconv.FormatInt(f.revision, 10)+`","name":"`+name+
			`","scope":`+workflowScopeJSON(agencies...)+`,"enabled":true}`, f.csrf, "", http.StatusOK)
	_, f.revision = f.reference(response)
}

// submit accepts one Transmission with a distinct text, so that it forms its own Message.
func (f *reprocessFixture) submit(label string, observed time.Time) uuid.UUID {
	f.t.Helper()
	var submitted struct {
		Data struct {
			TransmissionID uuid.UUID `json:"transmissionId"`
		} `json:"data"`
	}
	rawProcessingRequest(f.t, f.client, f.origin, "/api/v1/transmissions",
		fmt.Sprintf(`{"schemaVersion":1,"submissionId":%q,"protocol":"pocsag","observationTime":%q,`+
			`"gatewayNetworkIdentifier":"reprocess","payload":{"form":"text","value":%q},"transmissionTargets":[]}`,
			label, observed.UTC().Format(time.RFC3339), label),
		"Authorization", "Bearer "+f.secret, http.StatusAccepted, &submitted)
	return submitted.Data.TransmissionID
}

// runWorker executes every due candidate through the production worker and publisher.
func (f *reprocessFixture) runWorker() {
	f.t.Helper()
	for range 20 {
		var due []processing.Candidate
		if err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
			page, err := f.repository.SelectDueWork(tx.Context(), tx, 50)
			due = page
			return err
		}); err != nil {
			f.t.Fatal(err)
		}
		if len(due) == 0 {
			return
		}
		for _, candidate := range due {
			if err := f.worker.ExecuteWork(f.t.Context(), candidate); err != nil {
				f.t.Fatalf("candidate run failed: %s", causeChain(err))
			}
		}
	}
	f.t.Fatal("worker left due work after 20 rounds")
}

// reprocessRunner builds a Reprocess runner re-admitting through the given port.
func (f *reprocessFixture) reprocessRunner(readmission reprocess.Readmission) *reprocess.Runner {
	f.t.Helper()
	runner, err := reprocess.New(f.store, f.store, f.repository, readmission, f.shared.facts)
	if err != nil {
		f.t.Fatal(err)
	}
	return runner
}

// runReprocess drains the Reprocess requests with the production source owner.
func (f *reprocessFixture) runReprocess() {
	f.t.Helper()
	if _, err := f.reprocessRunner(f.runtime.readmission()).RunOnce(f.t.Context()); err != nil {
		f.t.Fatal(err)
	}
}

// reprocessBody renders a Reprocess request body.
func (f *reprocessFixture) reprocessBody(version uuid.UUID, expected int64, from, to time.Time) string {
	return fmt.Sprintf(`{"schemaVersion":1,"versionId":%q,"expectedRevision":"%d","messageTimeFrom":%q,"messageTimeTo":%q}`,
		version, expected, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
}

// requestReprocess asks for a Reprocess of the Workflow's current revision and returns the request ID.
func (f *reprocessFixture) requestReprocess(version uuid.UUID, from, to time.Time) uuid.UUID {
	f.t.Helper()
	response := f.post("/api/v1/workflows/"+f.workflow.String()+"/reprocess",
		f.reprocessBody(version, f.revision, from, to), http.StatusAccepted)
	var view struct {
		ID    uuid.UUID `json:"id"`
		State string    `json:"state"`
	}
	if err := json.Unmarshal(response.Data, &view); err != nil {
		f.t.Fatal(err)
	}
	if view.State != processing.ReprocessPending {
		f.t.Fatalf("new request state = %q, want pending", view.State)
	}
	f.revision++
	return view.ID
}

// requestView reads a request through the progress route.
type requestView struct {
	State      string `json:"state"`
	Scanned    int64  `json:"scanned"`
	Readmitted int64  `json:"readmitted"`
	Retracted  int64  `json:"retracted"`
	// FailureCode and Attempts describe the last failed step of a request that is retrying.
	FailureCode string `json:"failureCode"`
	Attempts    int    `json:"attempts"`
}

func (f *reprocessFixture) request(id uuid.UUID) requestView {
	f.t.Helper()
	var view requestView
	response := f.get("/api/v1/workflows/"+f.workflow.String()+"/reprocesses/"+id.String(), http.StatusOK)
	if err := json.Unmarshal(response.Data, &view); err != nil {
		f.t.Fatal(err)
	}
	return view
}

// levels lists the level values the Transmission's published results project, ordered.
func (f *reprocessFixture) levels(transmission uuid.UUID) []string {
	f.t.Helper()
	return f.queryStrings(`SELECT integer_value::text FROM message_attribute_values
		WHERE transmission_id=$1 ORDER BY integer_value`, transmission)
}

// results counts the Transmission's results by publication state.
func (f *reprocessFixture) results(transmission uuid.UUID) (published, retracted int) {
	f.t.Helper()
	published = f.count(`SELECT count(*) FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
		WHERE c.transmission_id=$1 AND r.published_at IS NOT NULL`, transmission)
	retracted = f.count(`SELECT count(*) FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
		WHERE c.transmission_id=$1 AND r.retracted_at IS NOT NULL AND r.published_at IS NULL AND r.message_id IS NULL`, transmission)
	return published, retracted
}

// replayModes lists the replay mode of every processing.completed fact of the Transmission in order.
func (f *reprocessFixture) replayModes(transmission uuid.UUID) []string {
	f.t.Helper()
	return f.queryStrings(`SELECT envelope->>'replayMode' FROM domain_events
		WHERE type='processing.completed' AND envelope->'data'->>'transmissionId'=$1 ORDER BY seq`, transmission.String())
}

// retractions lists, for each processing.retracted fact of the Transmission in order, how many
// results it names.
func (f *reprocessFixture) retractions(transmission uuid.UUID) []string {
	f.t.Helper()
	return f.queryStrings(`SELECT jsonb_array_length(envelope->'data'->'resultIds')::text FROM domain_events
		WHERE type='processing.retracted' AND envelope->'data'->>'transmissionId'=$1 ORDER BY seq`, transmission.String())
}

// aggregateRevisions lists, in order, the type and aggregate revision of every fact about the result.
func (f *reprocessFixture) aggregateRevisions(result string) []string {
	f.t.Helper()
	return f.queryStrings(`SELECT type || ' ' || (envelope->>'aggregateRevision') FROM domain_events
		WHERE type IN ('processing.completed', 'processing.retracted') AND envelope->'aggregateRef'->>'id'=$1
			AND envelope->'aggregateRef'->>'type'='processingResult' ORDER BY seq`, result)
}

// marked counts the Transmission's candidates that carry a Reprocess mark.
func (f *reprocessFixture) marked(transmission uuid.UUID) int {
	f.t.Helper()
	return f.count(`SELECT count(*) FROM processing_candidates WHERE transmission_id=$1 AND reprocess_id IS NOT NULL`, transmission)
}
