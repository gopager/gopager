package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
)

// TestAgencyRoutesManageAgenciesAndUnitsRequireOne expects the Agency routes to create, list,
// rename and refuse to delete an Agency that owns a Unit, and Unit creation to require an
// Agency (400 without one, 201 with one, and the Unit reports its agencyId).
func TestAgencyRoutesManageAgenciesAndUnitsRequireOne(t *testing.T) {
	store := newManagementStore(t)
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), store, nil)
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

	created := managementRequest(t, client, origin, http.MethodPost, "/api/v1/agencies",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"County fire dispatch"}`,
		login.CSRFToken, "", http.StatusCreated)
	agencyID, revision := managementResultID(t, created)
	if revision != "1" {
		t.Fatalf("created Agency revision=%s want 1", revision)
	}
	listed := managementRequest(t, client, origin, http.MethodGet, "/api/v1/agencies?limit=10", "", "", "", http.StatusOK)
	var page struct {
		Total int `json:"total"`
		Items []struct {
			ID   uuid.UUID `json:"id"`
			Name string    `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listed.Data, &page); err != nil || page.Total != 1 || page.Items[0].Name != "County fire dispatch" {
		t.Fatalf("agency list=%s err=%v", listed.Data, err)
	}
	renamed := managementRequest(t, client, origin, http.MethodPut, "/api/v1/agencies/"+agencyID.String(),
		`{"schemaVersion":1,"expectedRevision":"1","name":"County fire dispatch centre"}`, login.CSRFToken, "", http.StatusOK)
	if _, revision := managementResultID(t, renamed); revision != "2" {
		t.Fatalf("renamed Agency revision=%s want 2", revision)
	}

	unitBody := func(agency string) string {
		return `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","name":"Station 1",` + agency +
			`"organizationId":null,"collectionIds":[],"settings":{"color":{"mode":"inherit"},"icon":{"mode":"inherit"},"functionMeanings":{"mode":"inherit"}}}`
	}
	if response := managementRequest(t, client, origin, http.MethodPost, "/api/v1/units", unitBody(""),
		login.CSRFToken, "", http.StatusBadRequest); response.Code != "INVALID_INPUT" {
		t.Fatalf("unit without Agency error=%s", response.Code)
	}
	unit := managementRequest(t, client, origin, http.MethodPost, "/api/v1/units", unitBody(`"agencyId":"`+agencyID.String()+`",`),
		login.CSRFToken, "", http.StatusCreated)
	unitID, _ := managementResultID(t, unit)
	read := managementRequest(t, client, origin, http.MethodGet, "/api/v1/units/"+unitID.String(), "", "", "", http.StatusOK)
	var unitView struct {
		AgencyID uuid.UUID `json:"agencyId"`
	}
	if err := json.Unmarshal(read.Data, &unitView); err != nil || unitView.AgencyID != agencyID {
		t.Fatalf("unit view=%s err=%v, want agencyId %s", read.Data, err, agencyID)
	}
	inUse := managementRequest(t, client, origin, http.MethodDelete, "/api/v1/agencies/"+agencyID.String(),
		`{"schemaVersion":1,"expectedRevision":"2"}`, login.CSRFToken, "", http.StatusConflict)
	if inUse.Code != "REVISION_CONFLICT" {
		t.Fatalf("deleting an Agency with a Unit error=%s want REVISION_CONFLICT", inUse.Code)
	}
}
