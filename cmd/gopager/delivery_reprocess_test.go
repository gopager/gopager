package main

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/processing"
)

// TestReprocessFactCreatesNoDelivery expects the fan-out consumer to ignore a processing.completed
// fact with replay mode reprocess, however well an active Subscription matches its Message, while
// a normal fact for an equal Message creates its Delivery. Redelivery stays a separate operation.
func TestReprocessFactCreatesNoDelivery(t *testing.T) {
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
	version, revision := api.workflowVersion(author, workflow, revision, levelOutput(t, "3"))
	activated := api.subCall(author, http.MethodPost, "/api/v1/workflows/"+workflow.String()+"/versions/"+version.String()+"/activate",
		map[string]any{"schemaVersion": 1, "expectedRevision": strconv.FormatInt(revision, 10)})
	if activated.status != http.StatusOK {
		t.Fatalf("activation returned %d/%s", activated.status, activated.Code)
	}
	created2 := f.subCreated(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.completed"},
		"filter": levelAtLeast("2")}))
	unit := f.unit("Engine 1", nil)
	if response := f.subCreate(bearer, subInput(instance, map[string]any{"eventTypes": []string{"processing.retracted"}})); response.status == http.StatusCreated {
		t.Fatal("a Subscription must not be able to select the non-deliverable retraction fact")
	}

	complete := func(label, replay string) uuid.UUID {
		t.Helper()
		message := f.message(f.minuteAgo(), label, unit)
		result := f.publishLevel(workflow, message, "3")
		f.fact("processing.completed", map[string]any{"messageId": message, "messageTime": f.now().Format(time.RFC3339Nano),
			"resultId": result}, factOptions{replay: replay, aggregate: "processingResult"})
		return message
	}
	complete("REPROCESSED", events.ReplayReprocess)
	f.fanOutAll()
	if listed := f.deliveries(bearer, "?subscriptionId="+created2.ID.String()); len(listed) != 0 {
		t.Fatalf("Deliveries after a reprocess fact = %+v, want none", listed)
	}

	retracted := f.message(f.minuteAgo(), "RETRACTED", unit)
	f.publishLevel(workflow, retracted, "3")
	f.fact("processing.retracted", map[string]any{"messageId": retracted, "messageTime": f.now().Format(time.RFC3339Nano),
		"transmissionId": uuid.New(), "workflowId": workflow, "resultIds": []uuid.UUID{uuid.New()}},
		factOptions{replay: events.ReplayReprocess, aggregate: "processingResult"})
	f.fanOutAll()
	if listed := f.deliveries(bearer, "?subscriptionId="+created2.ID.String()); len(listed) != 0 {
		t.Fatalf("Deliveries after a retraction fact = %+v, want none", listed)
	}

	complete("NORMAL", events.ReplayNormal)
	f.fanOutAll()
	listed := f.deliveries(bearer, "?subscriptionId="+created2.ID.String())
	if len(listed) != 1 {
		t.Fatalf("Deliveries after a normal fact = %+v, want exactly the normal Message's", listed)
	}
}
