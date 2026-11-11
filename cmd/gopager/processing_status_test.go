package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

type processingReadFixture struct {
	candidateID    uuid.UUID
	transmissionID uuid.UUID
	resultID       uuid.UUID
}

func seedProcessingReadCandidate(t *testing.T, store *postgres.Store, workflowID uuid.UUID,
	state processingcandidate.State, selected *bool, failureCode string, at time.Time,
) processingReadFixture {
	t.Helper()
	fixture := processingReadFixture{candidateID: uuid.New(), transmissionID: uuid.New()}
	if state == processingcandidate.StateCompleted && selected != nil && *selected {
		fixture.resultID = uuid.New()
	}
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"WorkflowVersion":     entql.FieldEQ("workflow_id", workflowID),
				"ProcessingCandidate": entql.FieldEQ("workflow_id", workflowID),
				"ProcessingResult":    entql.FieldNotNil("candidate_id"),
			},
			Create: map[string]bool{
				"WorkflowVersion": true, "ProcessingCandidate": true, "ProcessingResult": true,
			},
		})
		versionID := uuid.New()
		if err := client.WorkflowVersion.Create().SetID(versionID).SetWorkflowID(workflowID).
			SetDefinition(processing.Definition{}).SetCreatedAt(at).SetCreatedBy(uuid.New()).Exec(scope); err != nil {
			return err
		}
		candidate := client.ProcessingCandidate.Create().SetID(fixture.candidateID).
			SetTransmissionID(fixture.transmissionID).SetWorkflowID(workflowID).
			SetDefinitionVersionID(versionID).SetWorkflowRevision(1).SetConfigurationRevision(1).
			SetSourceRevision(1).SetContextRevision(1).SetState(state).
			SetDueAt(at).SetCreatedAt(at).SetUpdatedAt(at)
		if selected != nil {
			candidate.SetSelected(*selected)
		}
		if failureCode != "" {
			candidate.SetFailureCode(failureCode)
		}
		if err := candidate.Exec(scope); err != nil {
			return err
		}
		if fixture.resultID != uuid.Nil {
			secret := processing.FieldValue{State: processing.FieldPresent,
				Value: &processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"private-evaluation-sentinel"`)}}
			return client.ProcessingResult.Create().SetID(fixture.resultID).
				SetCandidateID(fixture.candidateID).SetRevision(1).SetProducingFence(1).SetFieldGroups([]string{"derived"}).
				SetEvaluation(processing.Evaluation{Selected: true, Fields: map[string]map[string]processing.FieldValue{"private": {"secret": secret}}}).
				SetCompletedAt(at).Exec(scope)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func createReadReviewer(t *testing.T, store *postgres.Store) (uuid.UUID, string, string) {
	t.Helper()
	username := "reader-" + uuid.NewString()
	usernameKey, err := identity.UsernameKey(username)
	if err != nil {
		t.Fatal(err)
	}
	password := "reviewer-password"
	passwordHash, err := identity.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	principalID, grantID := uuid.New(), uuid.New()
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Create:  map[string]bool{"Account": true, "Grant": true},
		})
		now := time.Now().UTC().Add(-time.Minute)
		if err := client.Account.Create().SetID(principalID).SetKind(account.KindHuman).
			SetUsername(username).SetUsernameKey(usernameKey).SetPasswordHash(passwordHash).
			SetCreatedAt(now).Exec(scope); err != nil {
			return err
		}
		return client.Grant.Create().SetID(grantID).SetSubjectID(principalID).
			SetSubjectKind(grant.SubjectKindPrincipal).SetActions([]string{"password.local", "workflow.read"}).
			SetScope(authz.Scope{Kind: "all"}).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
	return principalID, username, password
}

func revokeReadReviewer(t *testing.T, store *postgres.Store, principalID uuid.UUID) {
	t.Helper()
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"Grant":           entql.FieldEQ("subject_id", principalID),
				"IdentityControl": entql.FieldEQ("id", 1),
			},
		})
		updated, err := client.Grant.Update().Where(grant.SubjectID(principalID)).
			SetActions([]string{"password.local"}).Save(scope)
		if err != nil {
			return err
		}
		if updated != 1 {
			return authz.ErrNotFound
		}
		return client.IdentityControl.UpdateOneID(1).AddAccessRevision(1).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessingStatusHTTPFiltersWorkflowPagesAndHidesPrivateEvidence(t *testing.T) {
	t.Parallel()
	store := newManagementStore(t)
	agency := seedAgency(t, store)
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
	originServer := httptest.NewUnstartedServer(nil)
	t.Cleanup(originServer.Close)
	origin := "http://" + originServer.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, nil, runtime.routes)
	if err != nil {
		t.Fatal(err)
	}
	originServer.Config.Handler = handler
	originServer.Start()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := originServer.Client()
	client.Jar = jar
	login := managementRequest(t, client, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)

	createWorkflow := func(name, commandID string) uuid.UUID {
		t.Helper()
		body := `{"schemaVersion":1,"commandId":"` + commandID +
			`","name":"` + name + `","scope":` + workflowScopeJSON(agency) + `,"enabled":true}`
		result := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
			body, login.CSRFToken, "", http.StatusCreated)
		id, _ := managementResultID(t, result)
		return id
	}
	workflowA := createWorkflow("Read A", "6e5adf3c-14bb-4bfc-a2b9-64e8171e08e1")
	workflowB := createWorkflow("Read B", "896cbd10-5d1a-4a46-8ee8-00197e34bb75")
	at := time.Now().UTC().Add(-time.Hour)
	pending := seedProcessingReadCandidate(t, store, workflowA, processingcandidate.StatePending, nil, "", at)
	completedSelected := true
	completed := seedProcessingReadCandidate(t, store, workflowA, processingcandidate.StateCompleted, &completedSelected, "", at.Add(time.Minute))
	failureCode := string(platform.CodeResourceLimit)
	failed := seedProcessingReadCandidate(t, store, workflowA, processingcandidate.StateFailed, nil, failureCode, at.Add(2*time.Minute))
	otherCompleted := seedProcessingReadCandidate(t, store, workflowB, processingcandidate.StateCompleted, &completedSelected, "", at)

	page := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates?limit=2", "", "", "", http.StatusOK)
	var firstPage struct {
		Items []struct {
			ID          uuid.UUID `json:"id"`
			Revision    int64     `json:"revision,string"`
			State       string    `json:"state"`
			Selected    *bool     `json:"selected"`
			FailureCode string    `json:"failureCode"`
			UpdatedAt   time.Time `json:"updatedAt"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(page.Data, &firstPage); err != nil {
		t.Fatal(err)
	}
	if firstPage.Total != 3 || len(firstPage.Items) != 2 {
		t.Fatalf("Workflow A first page total/items = %d/%d, want 3/2", firstPage.Total, len(firstPage.Items))
	}
	nextPage := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates?offset=2&limit=2", "", "", "", http.StatusOK)
	var lastPage struct {
		Items []struct {
			ID    uuid.UUID `json:"id"`
			State string    `json:"state"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(nextPage.Data, &lastPage); err != nil {
		t.Fatal(err)
	}
	if lastPage.Total != 3 || len(lastPage.Items) != 1 || lastPage.Items[0].ID != failed.candidateID ||
		lastPage.Items[0].State != "failed" {
		t.Fatalf("Workflow A final page = %+v, want only failed candidate", lastPage)
	}
	workflowBPage := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowB.String()+"/candidates", "", "", "", http.StatusOK)
	var otherPage struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(workflowBPage.Data, &otherPage); err != nil || otherPage.Total != 1 {
		t.Fatalf("Workflow B total = %d, decode error = %v; want 1", otherPage.Total, err)
	}

	status := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+completed.candidateID.String(),
		"", "", "", http.StatusOK)
	if strings.Contains(string(status.Data), pending.transmissionID.String()) ||
		strings.Contains(string(status.Data), "transmissionId") || strings.Contains(string(status.Data), "fencingToken") ||
		strings.Contains(string(status.Data), "workerId") || strings.Contains(string(status.Data), "attempts") {
		t.Fatalf("candidate status leaked private work fields: %s", status.Data)
	}
	result := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+completed.candidateID.String()+"/result",
		"", "", "", http.StatusOK)
	if strings.Contains(string(result.Data), "private-evaluation-sentinel") ||
		strings.Contains(string(result.Data), "evaluation") || strings.Contains(string(result.Data), "transmissionId") ||
		strings.Contains(string(result.Data), completed.transmissionID.String()) {
		t.Fatalf("private result metadata leaked Evaluation or source data: %s", result.Data)
	}
	var resultFields map[string]json.RawMessage
	if err := json.Unmarshal(result.Data, &resultFields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "revision", "completedAt"} {
		if _, ok := resultFields[field]; !ok {
			t.Fatalf("result metadata missing %q: %s", field, result.Data)
		}
	}
	if _, ok := resultFields["candidateId"]; ok {
		t.Fatalf("result metadata exposed json-hidden candidate binding: %s", result.Data)
	}

	invisible := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+otherCompleted.candidateID.String(),
		"", "", "", http.StatusNotFound)
	missing := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+uuid.NewString(),
		"", "", "", http.StatusNotFound)
	if invisible.Code != missing.Code {
		t.Fatalf("wrong-Workflow candidate code %q differs from missing candidate code %q", invisible.Code, missing.Code)
	}
	wrongResult := managementRequest(t, client, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates/"+otherCompleted.candidateID.String()+"/result",
		"", "", "", http.StatusNotFound)
	if wrongResult.Code != missing.Code {
		t.Fatalf("wrong-Workflow result code %q differs from missing candidate code %q", wrongResult.Code, missing.Code)
	}

	reviewerID, reviewer, password := createReadReviewer(t, store)
	reviewerJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	reviewerClient := originServer.Client()
	reviewerClient.Jar = reviewerJar
	managementRequest(t, reviewerClient, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"`+reviewer+`","password":"`+password+`"}`, "", "", http.StatusOK)
	managementRequest(t, reviewerClient, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates", "", "", "", http.StatusOK)
	revokeReadReviewer(t, store, reviewerID)
	managementRequest(t, reviewerClient, origin, http.MethodGet,
		"/api/v1/workflows/"+workflowA.String()+"/candidates", "", "", "", http.StatusForbidden)
}
