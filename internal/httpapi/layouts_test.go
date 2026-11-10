package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing/presentation"
	"github.com/gopager/gopager/internal/units"
)

// recordingLayouts records the calls the routes make.
type recordingLayouts struct {
	LayoutService
	versionID uuid.UUID
	version   int64
	updated   int64
	created   presentation.LayoutInput
}

func (r *recordingLayouts) ReadVersion(_ context.Context, _ identity.Proof, id uuid.UUID, version int64) (presentation.LayoutVersion, error) {
	r.versionID, r.version = id, version
	return presentation.LayoutVersion{LayoutID: id, Version: version}, nil
}

func (r *recordingLayouts) Create(_ context.Context, _ identity.Proof, _ uuid.UUID, input presentation.LayoutInput, _ func(identity.Authentication) error) (presentation.Layout, error) {
	r.created = input
	return presentation.Layout{ID: uuid.New(), Revision: 1, Name: input.Name, OutputKind: input.Definition.OutputKind}, nil
}

func (r *recordingLayouts) Update(_ context.Context, _ identity.Proof, id uuid.UUID, expected int64, _ presentation.LayoutInput, _ func(identity.Authentication) error) (presentation.Layout, error) {
	r.updated = expected
	return presentation.Layout{ID: id, Revision: expected + 1}, nil
}

func (r *recordingLayouts) Delete(_ context.Context, _ identity.Proof, id uuid.UUID, expected int64, _ func(identity.Authentication) error) (commands.Result, error) {
	return commands.Result{ID: id, Revision: expected + 1}, nil
}

func (r *recordingLayouts) List(context.Context, identity.Proof, units.PageInput) (units.Page[presentation.Layout], error) {
	return units.Page[presentation.Layout]{Items: []presentation.Layout{}}, nil
}

// acceptingAuthenticator accepts every credential.
type acceptingAuthenticator struct{}

func (acceptingAuthenticator) Authenticate(context.Context, identity.Proof) error { return nil }

func layoutMux(t *testing.T, layouts LayoutService) *http.ServeMux {
	t.Helper()
	transport, err := identity.NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterLayoutRoutes(mux, transport, acceptingAuthenticator{}, layouts)
	return mux
}

func TestLayoutRoutes(t *testing.T) {
	t.Parallel()
	id := uuid.NewString()
	definition := `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"text","text":"x"}]}`
	for _, test := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"list", "GET", "/api/v1/layouts", "", 200},
		{"version", "GET", "/api/v1/layouts/" + id + "/versions/3", "", 200},
		{"version zero", "GET", "/api/v1/layouts/" + id + "/versions/0", "", 400},
		{"version text", "GET", "/api/v1/layouts/" + id + "/versions/x", "", 400},
		{"version overflow", "GET", "/api/v1/layouts/" + id + "/versions/99999999999999999999", "", 400},
		{"bad id", "GET", "/api/v1/layouts/nope", "", 400},
		{"create", "POST", "/api/v1/layouts", `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","name":"n","definition":` + definition + `}`, 201},
		{"create without definition", "POST", "/api/v1/layouts", `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","name":"n"}`, 400},
		{"update", "PUT", "/api/v1/layouts/" + id, `{"schemaVersion":1,"expectedRevision":"4","name":"n","definition":` + definition + `}`, 200},
		{"delete", "DELETE", "/api/v1/layouts/" + id, `{"schemaVersion":1,"expectedRevision":"4"}`, 200},
		{"delete wrong schema", "DELETE", "/api/v1/layouts/" + id, `{"schemaVersion":2,"expectedRevision":"4"}`, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			layouts := &recordingLayouts{}
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
			layoutMux(t, layouts).ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d body %s, want %d", recorder.Code, recorder.Body, test.status)
			}
			switch test.name {
			case "version":
				if layouts.version != 3 || layouts.versionID.String() != id {
					t.Fatalf("version call = %v %d", layouts.versionID, layouts.version)
				}
			case "update":
				if layouts.updated != 4 {
					t.Fatalf("expected revision = %d", layouts.updated)
				}
			}
		})
	}
}
