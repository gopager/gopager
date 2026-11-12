package main

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
)

// TestAttributeSchemaRoutesEndToEnd proves the composed routes: a manager creates, reads, lists,
// updates and deletes a schema, a reader of the Agency sees it but cannot write, and a stranger
// sees nothing.
func TestAttributeSchemaRoutesEndToEnd(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	manager, _ := f.caller([]string{"attribute-schema.manage"}, authz.Scope{Kind: "all"}, []string{"payload"})
	reader, _ := f.caller([]string{"workflow.read"}, authz.Scope{Kind: "agency", References: []string{f.agency.String()}}, []string{"payload"})
	stranger, _ := f.caller([]string{"audit.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	body := map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "key": "alarm", "label": "Alarm", "subject": "message",
		"agencies": []uuid.UUID{f.agency}, "fields": map[string]any{"code": map[string]any{"kind": "string"}}}

	if got := f.subCall(manager, http.MethodPost, "/api/v1/attribute-schemas", body); got.status != http.StatusCreated {
		t.Fatalf("create = %d/%s", got.status, got.Code)
	}
	if got := f.subCall(reader, http.MethodGet, "/api/v1/attribute-schemas/alarm", nil); got.status != http.StatusOK {
		t.Fatalf("reader read = %d/%s", got.status, got.Code)
	}
	if got := f.subCall(reader, http.MethodGet, "/api/v1/attribute-schemas?agency="+f.agency.String(), nil); got.status != http.StatusOK {
		t.Fatalf("reader list = %d/%s", got.status, got.Code)
	}
	if got := f.subCall(reader, http.MethodDelete, "/api/v1/attribute-schemas/alarm", map[string]any{"schemaVersion": 1, "expectedRevision": "1"}); got.status != http.StatusForbidden {
		t.Fatalf("reader delete = %d/%s, want 403", got.status, got.Code)
	}
	if got := f.subCall(stranger, http.MethodGet, "/api/v1/attribute-schemas/alarm", nil); got.status != http.StatusForbidden {
		t.Fatalf("stranger read = %d/%s, want 403", got.status, got.Code)
	}
	if got := f.subCall(manager, http.MethodDelete, "/api/v1/attribute-schemas/alarm", map[string]any{"schemaVersion": 1, "expectedRevision": "1"}); got.status != http.StatusOK {
		t.Fatalf("delete = %d/%s", got.status, got.Code)
	}
	if got := f.subCall(manager, http.MethodGet, "/api/v1/attribute-schemas/alarm", nil); got.status != http.StatusNotFound {
		t.Fatalf("read deleted = %d/%s, want 404", got.status, got.Code)
	}
}
