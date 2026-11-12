package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// presentationEnvelope is the response envelope of the presentation routes.
type presentationEnvelope struct {
	Data       json.RawMessage `json:"data"`
	Code       string          `json:"code"`
	Parameters map[string]any  `json:"parameters"`
}

// presentationCall sends a request, expects the status and decodes the envelope.
func (f *queryFixture) presentationCall(who credentials, method, path, body string, status int) presentationEnvelope {
	f.t.Helper()
	got, _, payload := f.call(who, method, path, body)
	if got != status {
		f.t.Fatalf("%s %s = %d, want %d: %s", method, path, got, status, payload)
	}
	var envelope presentationEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		f.t.Fatalf("%s %s: %v: %s", method, path, err, payload)
	}
	return envelope
}

// textLayoutRow stores a text Layout of the Agencies and returns its ID.
func (f *queryFixture) textLayoutRow(kind string, agencies ...uuid.UUID) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	f.exec(`INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at)
		VALUES ($1, 1, 'Layout', $2, $3, now(), now())`, id, kind, uuid.New())
	f.exec(`INSERT INTO layout_definition_versions (layout_id, version, definition, created_by, created_at)
		VALUES ($1, 1, $2::jsonb, $3, now())`, id,
		fmt.Sprintf(`{"schemaVersion":1,"outputKind":%q,"blocks":[{"kind":"text","text":"x"}]}`, kind), uuid.New())
	for _, agency := range agencies {
		f.exec(`INSERT INTO layout_definition_agencies (layout_id, agency_id) VALUES ($1, $2)`, id, agency)
	}
	return id
}

func TestInstallationRoutes(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	// No credential at all: the route is public.
	read := f.presentationCall(credentials{}, http.MethodGet, "/api/v1/installation", "", http.StatusOK)
	var settings struct {
		Title           string  `json:"title"`
		Banner          *string `json:"banner"`
		DefaultLayoutID *string `json:"defaultLayoutId"`
		Revision        string  `json:"revision"`
	}
	if err := json.Unmarshal(read.Data, &settings); err != nil || settings.Title != "GoPager" || settings.Revision != "1" || settings.Banner != nil {
		t.Fatalf("seed = %s, %v", read.Data, err)
	}
	manager := f.session(f.human([]string{"notice.manage"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	layout := f.textLayoutRow("text", f.agency)
	body := fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":"1","title":"Control room","banner":"Planned downtime","severity":"warning","defaultLayoutId":%q}`, layout)
	f.presentationCall(manager, http.MethodPut, "/api/v1/installation", body, http.StatusOK)
	read = f.presentationCall(credentials{}, http.MethodGet, "/api/v1/installation", "", http.StatusOK)
	if err := json.Unmarshal(read.Data, &settings); err != nil || settings.Title != "Control room" || settings.Revision != "2" ||
		*settings.Banner != "Planned downtime" || *settings.DefaultLayoutID != layout.String() {
		t.Fatalf("updated = %s, %v", read.Data, err)
	}
	if got := f.presentationCall(manager, http.MethodPut, "/api/v1/installation", body, http.StatusConflict); got.Code != "REVISION_CONFLICT" {
		t.Fatalf("stale = %+v", got)
	}
	missing := fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":"2","title":"t","defaultLayoutId":%q}`, uuid.New())
	if got := f.presentationCall(manager, http.MethodPut, "/api/v1/installation", missing, http.StatusBadRequest); got.Parameters["field"] != "defaultLayoutId" {
		t.Fatalf("missing default Layout = %+v", got)
	}
	noBanner := `{"schemaVersion":1,"expectedRevision":"2","title":"t","banner":"b"}`
	if got := f.presentationCall(manager, http.MethodPut, "/api/v1/installation", noBanner, http.StatusBadRequest); got.Parameters["field"] != "severity" {
		t.Fatalf("banner without severity = %+v", got)
	}
	reader := f.session(f.human([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	f.presentationCall(reader, http.MethodPut, "/api/v1/installation", `{"schemaVersion":1,"expectedRevision":"2","title":"t"}`, http.StatusForbidden)
	f.presentationCall(credentials{}, http.MethodPut, "/api/v1/installation", `{"schemaVersion":1,"expectedRevision":"2","title":"t"}`, http.StatusUnauthorized)
}

func TestPreferenceRoutes(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	principal := f.human([]string{"preference.manage-own"}, authz.Scope{Kind: "all"}, []string{"payload"})
	session := f.session(principal)
	read := f.presentationCall(session, http.MethodGet, "/api/v1/me/preferences", "", http.StatusOK)
	if string(read.Data) != `{"locale":"en","timeZone":"UTC","timeGapMinutes":0,"revision":"0"}` {
		t.Fatalf("defaults = %s", read.Data)
	}
	first := `{"schemaVersion":1,"expectedRevision":"0","locale":"de-AT","timeZone":"Europe/Vienna","timeGapMinutes":30}`
	saved := f.presentationCall(session, http.MethodPut, "/api/v1/me/preferences", first, http.StatusOK)
	if string(saved.Data) != `{"locale":"de-AT","timeZone":"Europe/Vienna","timeGapMinutes":30,"revision":"1"}` {
		t.Fatalf("saved = %s", saved.Data)
	}
	if got := f.presentationCall(session, http.MethodPut, "/api/v1/me/preferences", first, http.StatusConflict); got.Code != "REVISION_CONFLICT" {
		t.Fatalf("second first-save = %+v", got)
	}
	for field, body := range map[string]string{
		"locale":         `{"schemaVersion":1,"expectedRevision":"1","locale":"x y","timeZone":"UTC","timeGapMinutes":0}`,
		"timeZone":       `{"schemaVersion":1,"expectedRevision":"1","locale":"en","timeZone":"Local","timeGapMinutes":0}`,
		"timeGapMinutes": `{"schemaVersion":1,"expectedRevision":"1","locale":"en","timeZone":"UTC","timeGapMinutes":1441}`,
	} {
		if got := f.presentationCall(session, http.MethodPut, "/api/v1/me/preferences", body, http.StatusBadRequest); got.Parameters["field"] != field {
			t.Fatalf("%s: %+v", field, got)
		}
	}
	// Keys, sessions without the action and the guest do not own preferences.
	f.presentationCall(f.key(principal, []string{"preference.manage-own"}, []string{"payload"}), http.MethodGet, "/api/v1/me/preferences", "", http.StatusForbidden)
	f.presentationCall(f.key(principal, []string{"preference.manage-own"}, []string{"payload"}), http.MethodPut, "/api/v1/me/preferences", first, http.StatusForbidden)
	other := f.session(f.human([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	f.presentationCall(other, http.MethodGet, "/api/v1/me/preferences", "", http.StatusForbidden)
	f.presentationCall(credentials{}, http.MethodGet, "/api/v1/me/preferences", "", http.StatusUnauthorized)
}

func TestLayoutAssignmentRoutes(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	otherAgency := seedAgency(t, f.store)
	manager := f.session(f.human([]string{"presentation.manage"}, authz.Scope{Kind: "agency", References: []string{f.agency.String()}}, []string{"payload"}))
	unit := f.unit("Engine 1", nil)
	organization := uuid.New()
	f.exec(`INSERT INTO organizations (id, name, defaults) VALUES ($1, 'Fire brigade', '{}'::jsonb)`, organization)
	layout := f.textLayoutRow("text", f.agency)
	foreign := f.textLayoutRow("text", otherAgency)
	both := f.textLayoutRow("text", f.agency, otherAgency)
	path := func(kind string, scope uuid.UUID, output string) string {
		return fmt.Sprintf("/api/v1/layout-assignments/%s/%s/%s", kind, scope, output)
	}

	created := f.presentationCall(manager, http.MethodPut, path("unit", unit, "text"),
		fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":"0","layoutId":%q}`, layout), http.StatusOK)
	var assignment struct {
		LayoutID string `json:"layoutId"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(created.Data, &assignment); err != nil || assignment.LayoutID != layout.String() || assignment.Revision != "1" {
		t.Fatalf("created = %s, %v", created.Data, err)
	}
	f.presentationCall(manager, http.MethodPut, path("unit", unit, "text"),
		fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":"0","layoutId":%q}`, layout), http.StatusConflict)
	f.presentationCall(manager, http.MethodPut, path("unit", unit, "markdown"),
		fmt.Sprintf(`{"schemaVersion":1,"layoutId":%q}`, layout), http.StatusBadRequest)
	if got := f.presentationCall(manager, http.MethodPut, path("unit", unit, "text"),
		fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":"1","layoutId":%q}`, foreign), http.StatusBadRequest); got.Parameters["field"] != "layoutId" {
		t.Fatalf("layout of another Agency = %+v", got)
	}
	f.presentationCall(manager, http.MethodPut, path("unit", uuid.New(), "text"),
		fmt.Sprintf(`{"schemaVersion":1,"layoutId":%q}`, layout), http.StatusNotFound)
	f.presentationCall(manager, http.MethodPut, path("organization", uuid.New(), "text"),
		fmt.Sprintf(`{"schemaVersion":1,"layoutId":%q}`, layout), http.StatusNotFound)
	// An Organization's Layout needs the right in every Agency of the Layout.
	f.presentationCall(manager, http.MethodPut, path("organization", organization, "text"),
		fmt.Sprintf(`{"schemaVersion":1,"layoutId":%q}`, both), http.StatusForbidden)
	f.presentationCall(manager, http.MethodPut, path("organization", organization, "text"),
		fmt.Sprintf(`{"schemaVersion":1,"layoutId":%q}`, layout), http.StatusOK)

	list := f.presentationCall(manager, http.MethodGet, "/api/v1/layout-assignments?scopeKind=unit&scopeId="+unit.String(), "", http.StatusOK)
	var page struct {
		Items []struct {
			ScopeKind string `json:"scopeKind"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(list.Data, &page); err != nil || page.Total != 1 || page.Items[0].ScopeKind != "unit" {
		t.Fatalf("list = %s, %v", list.Data, err)
	}
	reader := f.session(f.human([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	f.presentationCall(reader, http.MethodGet, "/api/v1/layout-assignments", "", http.StatusForbidden)

	// A referenced Layout stays until its assignment is gone.
	f.presentationCall(manager, http.MethodDelete, "/api/v1/layouts/"+layout.String(), `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusConflict)
	f.presentationCall(manager, http.MethodDelete, path("unit", unit, "text"), `{"schemaVersion":1,"expectedRevision":"7"}`, http.StatusConflict)
	f.presentationCall(manager, http.MethodDelete, path("unit", unit, "text"), `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusOK)
	f.presentationCall(manager, http.MethodDelete, path("organization", organization, "text"), `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusOK)
	f.presentationCall(manager, http.MethodDelete, "/api/v1/layouts/"+layout.String(), `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusOK)
}

func TestInstallationDefaultLayoutCannotBeDeleted(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	manager := f.session(f.human([]string{"presentation.manage", "notice.manage"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	layout := f.textLayoutRow("text", f.agency)
	f.presentationCall(manager, http.MethodPut, "/api/v1/installation",
		fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":"1","title":"t","defaultLayoutId":%q}`, layout), http.StatusOK)
	f.presentationCall(manager, http.MethodDelete, "/api/v1/layouts/"+layout.String(), `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusConflict)
	f.presentationCall(manager, http.MethodPut, "/api/v1/installation", `{"schemaVersion":1,"expectedRevision":"2","title":"t"}`, http.StatusOK)
	f.presentationCall(manager, http.MethodDelete, "/api/v1/layouts/"+layout.String(), `{"schemaVersion":1,"expectedRevision":"1"}`, http.StatusOK)
}

func TestLayoutEditorMetadataRoute(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	manager := f.session(f.human([]string{"presentation.manage"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	got := f.presentationCall(manager, http.MethodGet, "/api/v1/layouts/editor-metadata", "", http.StatusOK)
	var metadata struct {
		BlockKinds []struct {
			Kind string `json:"kind"`
		} `json:"blockKinds"`
		OutputKinds []string       `json:"outputKinds"`
		Bounds      map[string]int `json:"bounds"`
		Fields      []struct {
			Label string `json:"label"`
			Group string `json:"group"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(got.Data, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.BlockKinds) != 7 || len(metadata.OutputKinds) != 3 || metadata.Bounds["outputBytes"] != processing.MaxLayoutOutputBytes {
		t.Fatalf("metadata = %s", got.Data)
	}
	for _, field := range metadata.Fields {
		if field.Group != "" && field.Group != "payload" {
			t.Fatalf("field %+v of an unreadable group", field)
		}
	}
	subscriber := f.session(f.human([]string{"subscription.manage-own"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	f.presentationCall(subscriber, http.MethodGet, "/api/v1/layouts/editor-metadata", "", http.StatusOK)
	reader := f.session(f.human([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"}))
	f.presentationCall(reader, http.MethodGet, "/api/v1/layouts/editor-metadata", "", http.StatusForbidden)
}

// TestLayeredResolverIsComposed expects the composed Delivery side to resolve Layouts through the
// layers, with the production store.
func TestLayeredResolverIsComposed(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	runtime, err := composeDelivery(mustComposeCore(t, f.store))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.deps.Layouts != runtime.layoutResolver {
		t.Fatal("Delivery does not use the layered resolver")
	}
	unit := f.unit("Engine 1", nil)
	assigned, installed := f.textLayoutRow("text", f.agency), f.textLayoutRow("text", f.agency)
	f.exec(`INSERT INTO layout_assignments (scope_kind, scope_id, output_kind, layout_id, revision, updated_at)
		VALUES ('unit', $1, 'text', $2, 1, now())`, unit, assigned)
	f.exec(`UPDATE installation_settings SET default_layout_id = $1`, installed)
	resolve := func(units ...uuid.UUID) processing.ResolvedLayout {
		var resolved processing.ResolvedLayout
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			resolved, err = runtime.layoutResolver.ResolveLayout(tx.Context(), tx, processing.LayoutRequest{OutputKind: "text", UnitIDs: units})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return resolved
	}
	if got := resolve(unit); got.ID != assigned || got.Version != 1 || got.Conflict {
		t.Fatalf("unit layer = %+v", got)
	}
	if got := resolve(); got.ID != installed {
		t.Fatalf("installation default = %+v", got)
	}
	other := f.unit("Engine 2", nil)
	f.exec(`INSERT INTO layout_assignments (scope_kind, scope_id, output_kind, layout_id, revision, updated_at)
		VALUES ('unit', $1, 'text', $2, 1, now())`, other, installed)
	if got := resolve(unit, other); got.ID != installed || !got.Conflict {
		t.Fatalf("conflict = %+v", got)
	}
}
