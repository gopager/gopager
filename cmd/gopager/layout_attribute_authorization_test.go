package main

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
)

// TestLayoutAttributeReferenceNeedsTheRightInOneSchemaAgency expects a layout manager restricted
// to Agency A to add a reference to an Attribute schema only when the schema is assigned to A as
// well, and a reference the layout already holds to stay valid when the schema later loses A.
func TestLayoutAttributeReferenceNeedsTheRightInOneSchemaAgency(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	other := uuid.New()
	f.exec(`INSERT INTO agencies (id, name) VALUES ($1, 'Agency B')`, other)
	f.attributeSchema("incident", incidentFields)
	setAgencies := func(agencies ...uuid.UUID) {
		f.exec(`DELETE FROM attribute_schema_agencies WHERE schema_key = 'incident'`)
		for _, agency := range agencies {
			f.exec(`INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ('incident', $1)`, agency)
		}
	}
	manager, _ := f.caller([]string{"presentation.manage"}, authz.Scope{Kind: "agency", References: []string{f.agency.String()}}, []string{"payload"})
	withCity := func() map[string]any {
		return layoutBody("text", []any{map[string]any{"kind": "field", "field": attributeRef("city"), "label": "City"}})
	}

	setAgencies(other)
	if response := f.subCall(manager, http.MethodPost, "/api/v1/layouts", withAgency(withCity(), f.agency)); response.status != http.StatusForbidden {
		t.Fatalf("create with a schema of Agency B only returned %d, want 403", response.status)
	}
	plain := f.layoutCreated(manager, layoutBody("text", []any{textBlock("plain")}))
	if response := f.layoutUpdate(manager, plain.ID, plain.Revision, withCity()); response.status != http.StatusForbidden {
		t.Fatalf("update adding a schema of Agency B only returned %d, want 403", response.status)
	}

	setAgencies(f.agency, other)
	created := f.layoutCreated(manager, withCity())
	updated := f.layoutUpdate(manager, plain.ID, plain.Revision, withCity())
	if updated.status != http.StatusOK {
		t.Fatalf("update adding a schema of A and B returned %d", updated.status)
	}

	setAgencies(other)
	again := layoutBody("text", []any{map[string]any{"kind": "field", "field": attributeRef("city"), "label": "Town"}})
	if response := f.layoutUpdate(manager, created.ID, created.Revision, again); response.status != http.StatusOK {
		t.Fatalf("update keeping a reference after the schema lost A returned %d, want 200", response.status)
	}
}

// withAgency sets the Agencies of a layout create body.
func withAgency(body map[string]any, agencies ...uuid.UUID) map[string]any {
	body["agencies"] = agencies
	return body
}

// TestLayoutReferenceToARecreatedSchemaIsNew expects the exemption for a reference a layout
// already holds to apply only to the schema that existed when that version was saved: after the
// schema was deleted and re-created for Agency B only, an update by a manager of Agency A that
// keeps the reference is refused.
func TestLayoutReferenceToARecreatedSchemaIsNew(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	other := uuid.New()
	f.exec(`INSERT INTO agencies (id, name) VALUES ($1, 'Agency B')`, other)
	f.attributeSchema("incident", incidentFields)
	manager, _ := f.caller([]string{"presentation.manage"}, authz.Scope{Kind: "agency", References: []string{f.agency.String()}}, []string{"payload"})
	withCity := func(label string) map[string]any {
		return layoutBody("text", []any{map[string]any{"kind": "field", "field": attributeRef("city"), "label": label}})
	}
	created := f.layoutCreated(manager, withCity("City"))

	f.exec(`DELETE FROM attribute_schemas WHERE key = 'incident'`)
	f.attributeSchema("incident", incidentFields)
	f.exec(`DELETE FROM attribute_schema_agencies WHERE schema_key = 'incident'`)
	f.exec(`INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ('incident', $1)`, other)
	if response := f.layoutUpdate(manager, created.ID, created.Revision, withCity("Town")); response.status != http.StatusForbidden {
		t.Fatalf("update keeping a reference to a re-created schema of Agency B only returned %d, want 403", response.status)
	}
}

// TestLayoutUnitAttributeReferenceNeedsTheRightInOneSchemaAgency expects a reference to a Unit
// attribute (`unit.attributes.<key>…`) to count like one to a Message attribute: a layout manager
// restricted to Agency A cannot add a field of a `unit` schema assigned only to Agency B.
func TestLayoutUnitAttributeReferenceNeedsTheRightInOneSchemaAgency(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	other := uuid.New()
	f.exec(`INSERT INTO agencies (id, name) VALUES ($1, 'Agency B')`, other)
	f.exec(`INSERT INTO attribute_schemas (key, revision, label, subject, fields, created_by, created_at, updated_at)
		VALUES ('station', 1, 'Station', 'unit', '{"town":{"kind":"string"}}'::jsonb, $1, now(), now())`, uuid.New())
	f.exec(`INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ('station', $1)`, other)
	f.exec(`UPDATE attribute_catalog SET revision = revision + 1 WHERE id = 1`)
	manager, _ := f.caller([]string{"presentation.manage"}, authz.Scope{Kind: "agency", References: []string{f.agency.String()}},
		[]string{"payload", "unit"})
	town := layoutBody("text", []any{map[string]any{"kind": "field", "label": "Town",
		"field": map[string]any{"namespace": "unit", "path": []string{"attributes", "station", "town"}}}})

	if response := f.subCall(manager, http.MethodPost, "/api/v1/layouts", withAgency(town, f.agency)); response.status != http.StatusForbidden {
		t.Fatalf("create with a Unit attribute of Agency B only returned %d, want 403", response.status)
	}
	f.exec(`INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ('station', $1)`, f.agency)
	f.exec(`UPDATE attribute_catalog SET revision = revision + 1 WHERE id = 1`)
	if response := f.subCall(manager, http.MethodPost, "/api/v1/layouts", withAgency(town, f.agency)); response.status != http.StatusCreated {
		t.Fatalf("create with a Unit attribute of Agencies A and B returned %d, want 201", response.status)
	}
}
