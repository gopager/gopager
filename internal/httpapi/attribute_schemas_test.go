package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/units"
)

// recordingSchemas records the calls the routes make.
type recordingSchemas struct {
	AttributeSchemaService
	agency   *uuid.UUID
	page     units.PageInput
	read     string
	created  attributes.SchemaInput
	updated  attributes.SchemaInput
	expected int64
	deleted  string
}

func (r *recordingSchemas) List(_ context.Context, _ identity.Proof, agency *uuid.UUID, page units.PageInput) (units.Page[attributes.Schema], error) {
	r.agency, r.page = agency, page
	return units.Page[attributes.Schema]{Items: []attributes.Schema{}}, nil
}

func (r *recordingSchemas) Read(_ context.Context, _ identity.Proof, key string) (attributes.Schema, error) {
	r.read = key
	return attributes.Schema{Key: key, Revision: 3}, nil
}

func (r *recordingSchemas) Create(_ context.Context, _ identity.Proof, _ uuid.UUID, input attributes.SchemaInput, _ func(identity.Authentication) error) (attributes.Schema, error) {
	r.created = input
	return attributes.Schema{Key: input.Key, Revision: 1}, nil
}

func (r *recordingSchemas) Update(_ context.Context, _ identity.Proof, key string, expected int64, input attributes.SchemaInput, _ func(identity.Authentication) error) (attributes.Schema, error) {
	r.updated, r.expected = input, expected
	return attributes.Schema{Key: key, Revision: expected + 1}, nil
}

func (r *recordingSchemas) Delete(_ context.Context, _ identity.Proof, key string, expected int64, _ func(identity.Authentication) error) (attributes.Deleted, error) {
	r.deleted = key
	return attributes.Deleted{Key: key, Revision: expected + 1}, nil
}

func schemaMux(t *testing.T, schemas AttributeSchemaService) *http.ServeMux {
	t.Helper()
	transport, err := identity.NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterAttributeSchemaRoutes(mux, transport, acceptingAuthenticator{}, schemas)
	return mux
}

func TestAttributeSchemaRoutes(t *testing.T) {
	t.Parallel()
	agency := uuid.NewString()
	content := `"label":"L","subject":"message","agencies":["` + agency + `"],"fields":{"code":{"kind":"string","label":"Code"}}`
	for _, test := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"list", "GET", "/api/v1/attribute-schemas", "", 200},
		{"list filtered", "GET", "/api/v1/attribute-schemas?agency=" + agency + "&limit=5", "", 200},
		{"list bad agency", "GET", "/api/v1/attribute-schemas?agency=x", "", 400},
		{"list unknown parameter", "GET", "/api/v1/attribute-schemas?x=1", "", 400},
		{"read", "GET", "/api/v1/attribute-schemas/alarm", "", 200},
		{"create", "POST", "/api/v1/attribute-schemas", `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","key":"alarm",` + content + `}`, 201},
		{"create wrong schema version", "POST", "/api/v1/attribute-schemas", `{"schemaVersion":2,"commandId":"` + uuid.NewString() + `","key":"alarm",` + content + `}`, 400},
		{"create unknown member", "POST", "/api/v1/attribute-schemas", `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","key":"alarm","x":1,` + content + `}`, 400},
		{"update", "PUT", "/api/v1/attribute-schemas/alarm", `{"schemaVersion":1,"expectedRevision":"4",` + content + `}`, 200},
		{"update numeric revision", "PUT", "/api/v1/attribute-schemas/alarm", `{"schemaVersion":1,"expectedRevision":4,` + content + `}`, 400},
		{"delete", "DELETE", "/api/v1/attribute-schemas/alarm", `{"schemaVersion":1,"expectedRevision":"4"}`, 200},
		{"delete wrong schema version", "DELETE", "/api/v1/attribute-schemas/alarm", `{"schemaVersion":2,"expectedRevision":"4"}`, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			schemas := &recordingSchemas{}
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.method != "GET" {
				secret, _, err := identity.NewSecret()
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+secret)
			}
			recorder := httptest.NewRecorder()
			schemaMux(t, schemas).ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d body %s, want %d", recorder.Code, recorder.Body, test.status)
			}
			switch test.name {
			case "list filtered":
				if schemas.agency == nil || schemas.agency.String() != agency || schemas.page.Limit != 5 {
					t.Fatalf("list call = %v %+v", schemas.agency, schemas.page)
				}
			case "read":
				if schemas.read != "alarm" || !strings.Contains(recorder.Body.String(), `"revision":"3"`) {
					t.Fatalf("read = %q %s", schemas.read, recorder.Body)
				}
			case "create":
				if schemas.created.Key != "alarm" || schemas.created.Subject != attributes.SubjectMessage ||
					len(schemas.created.Agencies) != 1 || schemas.created.Fields["code"].Label != "Code" {
					t.Fatalf("created = %+v", schemas.created)
				}
			case "update":
				if schemas.expected != 4 || schemas.updated.Key != "alarm" {
					t.Fatalf("update call = %d %+v", schemas.expected, schemas.updated)
				}
			case "delete":
				if schemas.deleted != "alarm" {
					t.Fatalf("deleted = %q", schemas.deleted)
				}
			}
		})
	}
}
