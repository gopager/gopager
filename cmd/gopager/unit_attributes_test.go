package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
)

// attributeUnitsFixture is a composed application with a manager who maintains Attribute
// schemas, Organizations, templates and Units, and two Agencies.
type attributeUnitsFixture struct {
	*deliveryFixture
	bearer string
	north  uuid.UUID
	south  uuid.UUID
}

func newAttributeUnitsFixture(t *testing.T) *attributeUnitsFixture {
	t.Helper()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	bearer, _ := f.caller([]string{"attribute-schema.manage", "unit.create", "unit.read", "unit.update", "unit.delete",
		"organization.create", "organization.read", "organization.update", "organization.delete",
		"unit-template.create", "unit-template.read", "unit-template.update", "unit-template.delete"},
		authz.Scope{Kind: "all"}, []string{"unit", "payload"})
	return &attributeUnitsFixture{deliveryFixture: f, bearer: bearer, north: f.agency, south: seedAgency(f.t, f.store)}
}

// do sends an authenticated request as the manager.
func (f *attributeUnitsFixture) do(method, path string, body any) subResponse {
	f.t.Helper()
	return f.subCall(f.bearer, method, path, body)
}

// expect fails the test unless the response has the status, and returns it.
func (f *attributeUnitsFixture) expect(response subResponse, status int) subResponse {
	f.t.Helper()
	if response.status != status {
		f.t.Fatalf("status = %d/%s (%s), want %d", response.status, response.Code, response.Data, status)
	}
	return response
}

// pick returns the value at the path of a JSON object, or nil.
func pick(raw json.RawMessage, path ...string) any {
	var current any
	if err := json.Unmarshal(raw, &current); err != nil {
		return nil
	}
	for _, name := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[name]
	}
	return current
}

func (f *attributeUnitsFixture) schema(key, subject string, agencies []uuid.UUID, fields map[string]any) {
	f.t.Helper()
	f.expect(f.do(http.MethodPost, "/api/v1/attribute-schemas", map[string]any{"schemaVersion": 1, "commandId": uuid.New(),
		"key": key, "label": key, "subject": subject, "agencies": agencies, "fields": fields}), http.StatusCreated)
}

// stationSchema creates the `unit` schema "station" for the north Agency.
func (f *attributeUnitsFixture) stationSchema() {
	f.schema("station", "unit", []uuid.UUID{f.north},
		map[string]any{"town": map[string]any{"kind": "string"}, "count": map[string]any{"kind": "integer"}})
}

func text(value string) map[string]any { return map[string]any{"kind": "string", "value": value} }

func valueEntry(value map[string]any) map[string]any {
	return map[string]any{"mode": "value", "value": value}
}

func (f *attributeUnitsFixture) unitBody(agency uuid.UUID, organization *uuid.UUID, attributes map[string]any) map[string]any {
	return map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "agencyId": agency, "name": "Unit",
		"collectionIds": []uuid.UUID{}, "organizationId": organization, "settings": map[string]any{"attributes": attributes}}
}

// createUnit creates a Unit and returns the response.
func (f *attributeUnitsFixture) createUnit(agency uuid.UUID, organization *uuid.UUID, attributes map[string]any) subResponse {
	f.t.Helper()
	return f.do(http.MethodPost, "/api/v1/units", f.unitBody(agency, organization, attributes))
}

// created creates a Unit and returns its stored representation.
func (f *attributeUnitsFixture) created(agency uuid.UUID, organization *uuid.UUID, attributes map[string]any) subResponse {
	f.t.Helper()
	return f.read("/api/v1/units/", f.id(f.expect(f.createUnit(agency, organization, attributes), http.StatusCreated)))
}

// fromTemplate creates a Unit from the template and returns its stored representation.
func (f *attributeUnitsFixture) fromTemplate(agency, template uuid.UUID) subResponse {
	f.t.Helper()
	response := f.expect(f.do(http.MethodPost, "/api/v1/units/from-template", map[string]any{"schemaVersion": 1,
		"commandId": uuid.New(), "agencyId": agency, "templateId": template, "name": "Copy", "collectionIds": []uuid.UUID{}}),
		http.StatusCreated)
	return f.read("/api/v1/units/", f.id(response))
}

func (f *attributeUnitsFixture) id(response subResponse) uuid.UUID {
	f.t.Helper()
	id, err := uuid.Parse(pick(response.Data, "id").(string))
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

// organization creates an Organization whose defaults carry the attributes.
func (f *attributeUnitsFixture) organization(attributes map[string]any) uuid.UUID {
	f.t.Helper()
	return f.id(f.expect(f.do(http.MethodPost, "/api/v1/organizations", map[string]any{"schemaVersion": 1,
		"commandId": uuid.New(), "name": "Org", "defaults": organizationDefaults(attributes)}), http.StatusCreated))
}

func organizationDefaults(attributes map[string]any) map[string]any {
	return map[string]any{"color": map[string]any{"mode": "unset"}, "icon": map[string]any{"mode": "unset"},
		"functionMeanings": map[string]any{"mode": "unset"}, "attributes": attributes}
}

func (f *attributeUnitsFixture) template(organization *uuid.UUID, attributes map[string]any) subResponse {
	f.t.Helper()
	return f.do(http.MethodPost, "/api/v1/unit-templates", map[string]any{"schemaVersion": 1, "commandId": uuid.New(),
		"name": "Template", "organizationId": organization, "settings": map[string]any{"attributes": attributes}})
}

func (f *attributeUnitsFixture) read(path string, id uuid.UUID) subResponse {
	f.t.Helper()
	return f.expect(f.do(http.MethodGet, path+id.String(), nil), http.StatusOK)
}

func (f *attributeUnitsFixture) deleteSchema(key string, revision string) subResponse {
	return f.do(http.MethodDelete, "/api/v1/attribute-schemas/"+key, map[string]any{"schemaVersion": 1, "expectedRevision": revision})
}

func TestUnitAttributesAreValidatedAgainstAssignedUnitSchemas(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	f.schema("alarm", "message", []uuid.UUID{f.north}, map[string]any{"code": map[string]any{"kind": "string"}})
	f.schema("radio", "unit", []uuid.UUID{f.south}, map[string]any{"channel": map[string]any{"kind": "integer"}})

	created := f.created(f.north, nil, map[string]any{"station": map[string]any{
		"town": valueEntry(text("Northville")), "count": map[string]any{"mode": "unset"}}})
	if got := pick(created.Data, "effectiveSettings", "attributes", "station", "town", "value", "value"); got != "Northville" {
		t.Fatalf("effective town = %v", got)
	}
	if got := pick(created.Data, "effectiveSettings", "attributes", "station", "count"); got != nil {
		t.Fatalf("unset count has effective value %v", got)
	}
	invalid := map[string]map[string]any{
		"schema of another Agency": {"radio": map[string]any{"channel": valueEntry(map[string]any{"kind": "integer", "value": "7"})}},
		"message schema":           {"alarm": map[string]any{"code": valueEntry(text("x"))}},
		"unknown schema":           {"missing": map[string]any{"town": valueEntry(text("x"))}},
		"unknown field":            {"station": map[string]any{"mayor": valueEntry(text("x"))}},
		"wrong type":               {"station": map[string]any{"count": valueEntry(text("many"))}},
		"undeclared property":      {"station": map[string]any{"town": map[string]any{"mode": "unset", "extra": true}}},
		"unset of other Agency":    {"radio": map[string]any{"channel": map[string]any{"mode": "unset"}}},
	}
	for name, attributes := range invalid {
		if got := f.createUnit(f.north, nil, attributes); got.status != http.StatusBadRequest || got.Code != "INVALID_INPUT" {
			t.Errorf("%s: create = %d/%s, want 400 INVALID_INPUT", name, got.status, got.Code)
		}
	}
	f.expect(f.createUnit(f.south, nil, map[string]any{"radio": map[string]any{
		"channel": valueEntry(map[string]any{"kind": "integer", "value": "7"})}}), http.StatusCreated)
	if got := f.template(nil, map[string]any{"missing": map[string]any{"town": valueEntry(text("x"))}}); got.status != http.StatusBadRequest {
		t.Fatalf("template with unknown schema = %d/%s, want 400", got.status, got.Code)
	}
	f.expect(f.template(nil, map[string]any{"radio": map[string]any{"channel": valueEntry(map[string]any{"kind": "integer", "value": "1"})}}),
		http.StatusCreated)
}

func TestUnitAttributesInheritFromOrganizationForAssignedAgenciesOnly(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	organization := f.organization(map[string]any{"station": map[string]any{"town": valueEntry(text("Default"))}})

	inheriting := f.created(f.north, &organization, nil)
	if got := pick(inheriting.Data, "effectiveSettings", "attributes", "station", "town", "source"); got != "organization" {
		t.Fatalf("inherited source = %v", got)
	}
	own := f.created(f.north, &organization, map[string]any{"station": map[string]any{"town": valueEntry(text("Own"))}})
	if got := pick(own.Data, "effectiveSettings", "attributes", "station", "town", "source"); got != "unit" {
		t.Fatalf("own source = %v", got)
	}
	unset := f.created(f.north, &organization, map[string]any{"station": map[string]any{"town": map[string]any{"mode": "unset"}}})
	if got := pick(unset.Data, "effectiveSettings", "attributes", "station", "town"); got != nil {
		t.Fatalf("unset town resolves to %v", got)
	}
	foreign := f.created(f.south, &organization, nil)
	if got := pick(foreign.Data, "effectiveSettings", "attributes", "station"); got != nil {
		t.Fatalf("Unit of an Agency without the schema resolves %v", got)
	}
}

func TestOrganizationDeletionMaterializesInheritedAttributes(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	organization := f.organization(map[string]any{"station": map[string]any{"town": valueEntry(text("Default"))}})
	inheriting := f.id(f.expect(f.createUnit(f.north, &organization, nil), http.StatusCreated))
	foreign := f.id(f.expect(f.createUnit(f.south, &organization, nil), http.StatusCreated))

	f.expect(f.do(http.MethodDelete, "/api/v1/organizations/"+organization.String(),
		map[string]any{"schemaVersion": 1, "expectedRevision": "1"}), http.StatusOK)

	north := f.read("/api/v1/units/", inheriting)
	if got := pick(north.Data, "settings", "attributes", "station", "town", "value", "value"); got != "Default" {
		t.Fatalf("materialized settings = %s", north.Data)
	}
	if got := pick(north.Data, "effectiveSettings", "attributes", "station", "town", "source"); got != "unit" {
		t.Fatalf("materialized source = %v", got)
	}
	south := f.read("/api/v1/units/", foreign)
	if got := pick(south.Data, "settings", "attributes", "station"); got != nil {
		t.Fatalf("Unit of an Agency without the schema stored %v", got)
	}
}

func TestUnitFromTemplateCopiesEffectiveValuesOfAssignedSchemas(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	f.schema("radio", "unit", []uuid.UUID{f.south}, map[string]any{"channel": map[string]any{"kind": "integer"}})
	organization := f.organization(map[string]any{"station": map[string]any{"count": valueEntry(map[string]any{"kind": "integer", "value": "3"})}})
	template := f.id(f.expect(f.template(&organization, map[string]any{
		"station": map[string]any{"town": valueEntry(text("Templated"))},
		"radio":   map[string]any{"channel": valueEntry(map[string]any{"kind": "integer", "value": "9"})},
	}), http.StatusCreated))

	copied := f.fromTemplate(f.north, template)
	if got := pick(copied.Data, "settings", "attributes", "station", "town", "value", "value"); got != "Templated" {
		t.Fatalf("copied town = %s", copied.Data)
	}
	if got := pick(copied.Data, "settings", "attributes", "station", "count", "value", "value"); got != "3" {
		t.Fatalf("copied inherited count = %s", copied.Data)
	}
	if got := pick(copied.Data, "settings", "attributes", "radio"); got != nil {
		t.Fatalf("copied a schema of another Agency: %v", got)
	}
	southCopy := f.fromTemplate(f.south, template)
	if got := pick(southCopy.Data, "settings", "attributes", "radio", "channel", "value", "value"); got != "9" {
		t.Fatalf("copied channel = %s", southCopy.Data)
	}
	if got := pick(southCopy.Data, "settings", "attributes", "station"); got != nil {
		t.Fatalf("copied a schema of another Agency: %v", got)
	}
}

func TestBulkUnitAttributeUpdateIsAllOrNothing(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	good := f.id(f.expect(f.createUnit(f.north, nil, nil), http.StatusCreated))
	bad := f.id(f.expect(f.createUnit(f.north, nil, nil), http.StatusCreated))
	item := func(id uuid.UUID, attributes map[string]any) map[string]any {
		return map[string]any{"id": id, "expectedRevision": "1", "settings": map[string]any{"attributes": attributes}}
	}
	items := []any{
		item(good, map[string]any{"station": map[string]any{"town": valueEntry(text("Bulk"))}}),
		item(bad, map[string]any{"station": map[string]any{"count": valueEntry(text("many"))}}),
	}

	preview := f.expect(f.do(http.MethodPost, "/api/v1/units/bulk/preview",
		map[string]any{"schemaVersion": 1, "operation": "update", "items": items}), http.StatusOK)
	if pick(preview.Data, "valid") != false {
		t.Fatalf("preview = %s", preview.Data)
	}
	codes := map[string]any{}
	for _, entry := range pick(preview.Data, "items").([]any) {
		object := entry.(map[string]any)
		codes[object["id"].(string)] = object["code"]
	}
	if codes[bad.String()] != "INVALID_INPUT" || codes[good.String()] != nil {
		t.Fatalf("preview codes = %v", codes)
	}
	if got := f.do(http.MethodPost, "/api/v1/units/bulk", map[string]any{"schemaVersion": 1, "commandId": uuid.New(),
		"operation": "update", "items": items}); got.status != http.StatusBadRequest {
		t.Fatalf("bulk = %d/%s, want 400", got.status, got.Code)
	}
	untouched := f.read("/api/v1/units/", good)
	if pick(untouched.Data, "revision") != "1" || pick(untouched.Data, "settings", "attributes", "station") != nil {
		t.Fatalf("the valid item was applied: %s", untouched.Data)
	}
	f.expect(f.do(http.MethodPost, "/api/v1/units/bulk", map[string]any{"schemaVersion": 1, "commandId": uuid.New(),
		"operation": "update", "items": items[:1]}), http.StatusOK)
	updated := f.read("/api/v1/units/", good)
	if got := pick(updated.Data, "settings", "attributes", "station", "town", "value", "value"); got != "Bulk" {
		t.Fatalf("bulk result = %s", updated.Data)
	}
}

func TestUnitAgencyChangeRequiresAttributesOfTheNewAgency(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	organization := f.organization(map[string]any{"station": map[string]any{"town": valueEntry(text("Default"))}})
	unit := f.id(f.expect(f.createUnit(f.north, &organization, map[string]any{
		"station": map[string]any{"count": valueEntry(map[string]any{"kind": "integer", "value": "2"})}}), http.StatusCreated))

	move := func(attributes map[string]any) subResponse {
		body := f.unitBody(f.south, &organization, attributes)
		delete(body, "commandId")
		body["expectedRevision"] = "1"
		return f.do(http.MethodPut, "/api/v1/units/"+unit.String(), body)
	}
	if got := move(map[string]any{"station": map[string]any{"count": valueEntry(map[string]any{"kind": "integer", "value": "2"})}}); got.status != http.StatusBadRequest {
		t.Fatalf("move with entries of the old Agency = %d/%s, want 400", got.status, got.Code)
	}
	moved := f.expect(move(nil), http.StatusOK)
	if got := pick(moved.Data, "effectiveSettings", "attributes", "station"); got != nil {
		t.Fatalf("moved Unit resolves %v", got)
	}
}

func TestAttributeSchemaChangesConflictWithStoredUnitAttributes(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	f.schema("badge", "unit", []uuid.UUID{f.north}, map[string]any{"label": map[string]any{"kind": "string"}})
	f.schema("shared", "unit", []uuid.UUID{f.north}, map[string]any{"label": map[string]any{"kind": "string"}})
	f.schema("defaulted", "unit", []uuid.UUID{f.north}, map[string]any{"label": map[string]any{"kind": "string"}})
	unit := f.id(f.expect(f.createUnit(f.north, nil, map[string]any{"station": map[string]any{"town": valueEntry(text("A"))}}), http.StatusCreated))
	f.expect(f.template(nil, map[string]any{"badge": map[string]any{"label": valueEntry(text("T"))}}), http.StatusCreated)
	f.organization(map[string]any{"defaulted": map[string]any{"label": valueEntry(text("O"))}})

	for _, key := range []string{"station", "badge", "defaulted"} {
		if got := f.deleteSchema(key, "1"); got.status != http.StatusConflict || got.Code != "REVISION_CONFLICT" {
			t.Errorf("delete %s = %d/%s, want 409 REVISION_CONFLICT", key, got.status, got.Code)
		}
	}
	f.expect(f.deleteSchema("shared", "1"), http.StatusOK)

	update := func(agencies []uuid.UUID) subResponse {
		return f.do(http.MethodPut, "/api/v1/attribute-schemas/station", map[string]any{"schemaVersion": 1, "expectedRevision": "1",
			"label": "station", "subject": "unit", "agencies": agencies,
			"fields": map[string]any{"town": map[string]any{"kind": "string"}, "count": map[string]any{"kind": "integer"}}})
	}
	if got := update([]uuid.UUID{f.south}); got.status != http.StatusConflict || got.Code != "REVISION_CONFLICT" {
		t.Fatalf("removing the Agency of a stored entry = %d/%s, want 409", got.status, got.Code)
	}
	f.expect(update([]uuid.UUID{f.north, f.south}), http.StatusOK)

	// Deleting the Unit releases its entry, and the schema then goes without a conflict.
	f.expect(f.do(http.MethodDelete, "/api/v1/units/"+unit.String(), map[string]any{"schemaVersion": 1, "expectedRevision": "1"}), http.StatusAccepted)
	f.expect(f.deleteSchema("station", "2"), http.StatusOK)
	if stored := f.scalar(`SELECT count(*) FROM units WHERE id = $1 AND jsonb_exists(settings->'attributes', 'station')`, unit); stored != 0 {
		t.Fatalf("a deleted Unit still stores the entry of the deleted schema")
	}
}

func TestUnitAttributeEditsLeaveRightsAndProcessingAlone(t *testing.T) {
	t.Parallel()
	f := newAttributeUnitsFixture(t)
	f.stationSchema()
	unit := f.id(f.expect(f.createUnit(f.north, nil, nil), http.StatusCreated))
	counts := func() [3]int {
		return [3]int{f.scalar(`SELECT count(*) FROM assignment_pending_scopes`), f.scalar(`SELECT count(*) FROM identity_grants`),
			f.scalar(`SELECT count(*) FROM processing_candidates`)}
	}
	before := counts()
	body := f.unitBody(f.north, nil, map[string]any{"station": map[string]any{"town": valueEntry(text("Edited"))}})
	delete(body, "commandId")
	body["expectedRevision"] = "1"
	f.expect(f.do(http.MethodPut, "/api/v1/units/"+unit.String(), body), http.StatusOK)
	if after := counts(); after != before {
		t.Fatalf("pending scopes, grants and candidates = %v, were %v", after, before)
	}
}
