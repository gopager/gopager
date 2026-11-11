package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
)

// workflowRoutes serves the Workflow management routes of the fixture's core on a second server
// and returns a fixture view whose HTTP calls reach it. The Delivery fixture mounts no Processing
// routes, and the Workflows of this test must be created and activated through them.
func (f *deliveryFixture) workflowRoutes() *deliveryFixture {
	f.t.Helper()
	runtime, err := composeProcessingAPIRuntime(f.t.Context(), f.core, "")
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() {
		if err := runtime.Close(f.t.Context()); err != nil {
			f.t.Error(err)
		}
	})
	backend := httptest.NewUnstartedServer(nil)
	f.t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), f.core, nil, runtime.routes)
	if err != nil {
		f.t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	query := *f.queryFixture
	query.origin, query.client = origin, backend.Client()
	view := *f
	view.queryFixture = &query
	return &view
}

// workflowVersion saves a Workflow version writing the given outputs and returns its ID and the
// Workflow revision after the save.
func (f *deliveryFixture) workflowVersion(bearer string, workflow uuid.UUID, expected int64,
	outputs map[string]map[string]processing.Assignment) (uuid.UUID, int64) {
	f.t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	protocol, err := processing.Literal(processing.StringValue, "pocsag")
	if err != nil {
		f.t.Fatal(err)
	}
	response := f.subCall(bearer, http.MethodPost, "/api/v1/workflows/"+workflow.String()+"/versions",
		workflowVersionMutation{SchemaVersion: 1, CommandID: uuid.New(), ExpectedRevision: expected,
			Definition: processing.Definition{SchemaVersion: 1,
				Selector: processing.Predicate{Op: "eq", Field: &field, Value: &protocol}, Outputs: outputs}})
	if response.status != http.StatusCreated {
		f.t.Fatalf("version save returned %d/%s", response.status, response.Code)
	}
	return f.resultReference(response)
}

// resultReference reads the ID and revision of a management response.
func (f *deliveryFixture) resultReference(response subResponse) (uuid.UUID, int64) {
	f.t.Helper()
	id, revision := managementResultID(f.t, managementResponse{Data: response.Data})
	parsed, err := strconv.ParseInt(revision, 10, 64)
	if err != nil {
		f.t.Fatal(err)
	}
	return id, parsed
}

// levelOutput is the output writing incident.level as the integer literal.
func levelOutput(t *testing.T, level string) map[string]map[string]processing.Assignment {
	t.Helper()
	literal, err := processing.Literal(processing.IntegerValue, level)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]map[string]processing.Assignment{"incident": {"level": {Literal: &literal}}}
}

// TestSubscriptionStaysValidWhenTheProducingWorkflowChangesVersion proves that a filter
// references the Attribute schema field and not the Workflow: activating a new version of the
// Workflow that produced the field neither invalidates the Subscription nor stops later matching
// results from creating Deliveries.
func TestSubscriptionStaysValidWhenTheProducingWorkflowChangesVersion(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.subscriber(fanOutGroups...)
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	api := f.workflowRoutes()
	author, _ := f.caller([]string{"workflow.read", "workflow.create", "workflow.update", "workflow.activate"},
		authz.Scope{Kind: "all"}, fanOutGroups)

	created := api.subCall(author, http.MethodPost, "/api/v1/workflows", map[string]any{"schemaVersion": 1,
		"commandId": uuid.New(), "name": "Level producer", "enabled": true, "scope": processing.WorkflowScope{Agencies: []uuid.UUID{f.agency}}})
	if created.status != http.StatusCreated {
		t.Fatalf("workflow create returned %d/%s", created.status, created.Code)
	}
	workflow, revision := api.resultReference(created)
	activate := func(version uuid.UUID, expected int64) int64 {
		t.Helper()
		response := api.subCall(author, http.MethodPost, "/api/v1/workflows/"+workflow.String()+"/versions/"+version.String()+"/activate",
			map[string]any{"schemaVersion": 1, "expectedRevision": strconv.FormatInt(expected, 10)})
		if response.status != http.StatusOK {
			t.Fatalf("activation returned %d/%s", response.status, response.Code)
		}
		_, activated := api.resultReference(response)
		return activated
	}
	first, revision := api.workflowVersion(author, workflow, revision, levelOutput(t, "3"))
	revision = activate(first, revision)

	created2 := f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed"},
		"filter": levelAtLeast("2")}))
	unit := f.unit("Engine 1", nil)
	before := f.message(f.minuteAgo(), "BEFORE", unit)
	result := f.publishLevel(workflow, before, "3")
	f.messageFact("processing.completed", before, map[string]any{"resultId": result})
	f.fanOutAll()
	if listed := f.deliveries(bearer, ""); len(listed) != 1 {
		t.Fatalf("Deliveries before the version change = %+v, want one", listed)
	}

	// The second version writes the same field from another literal, as a rewritten rule would.
	second, revision := api.workflowVersion(author, workflow, revision, levelOutput(t, "4"))
	activate(second, revision)
	f.requireHealth(bearer, created2.ID, delivery.HealthValid, "", 1)

	after := f.message(f.minuteAgo(), "AFTER", unit)
	result = f.publishLevel(workflow, after, "4")
	f.messageFact("processing.completed", after, map[string]any{"resultId": result})
	f.fanOutAll()
	f.requireHealth(bearer, created2.ID, delivery.HealthValid, "", 1)
	if listed := f.deliveries(bearer, "?subscriptionId="+created2.ID.String()); len(listed) != 2 {
		t.Fatalf("Deliveries after the version change = %+v, want two", listed)
	}
}
