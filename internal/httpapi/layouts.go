package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/presentation"
	"github.com/gopager/gopager/internal/units"
)

// LayoutService is the layout side the routes call. *presentation.Service implements it.
type LayoutService interface {
	List(context.Context, identity.Proof, units.PageInput) (units.Page[presentation.Layout], error)
	Read(context.Context, identity.Proof, uuid.UUID) (presentation.Layout, error)
	ReadVersion(context.Context, identity.Proof, uuid.UUID, int64) (presentation.LayoutVersion, error)
	Create(context.Context, identity.Proof, uuid.UUID, presentation.LayoutInput, func(identity.Authentication) error) (presentation.Layout, error)
	Update(context.Context, identity.Proof, uuid.UUID, int64, presentation.LayoutInput, func(identity.Authentication) error) (presentation.Layout, error)
	Delete(context.Context, identity.Proof, uuid.UUID, int64, func(identity.Authentication) error) (commands.Result, error)
}

// layoutCreate is the body of POST /layouts.
type layoutCreate struct {
	SchemaVersion int                `json:"schemaVersion"`
	CommandID     uuid.UUID          `json:"commandId"`
	Name          string             `json:"name"`
	Definition    *processing.Layout `json:"definition"`
}

// layoutUpdate is the body of PUT /layouts/{id}.
type layoutUpdate struct {
	SchemaVersion    int                `json:"schemaVersion"`
	ExpectedRevision int64              `json:"expectedRevision,string"`
	Name             string             `json:"name"`
	Definition       *processing.Layout `json:"definition"`
}

// layoutVersion parses the positive decimal int64 version path parameter.
func layoutVersion(r *http.Request) (int64, error) {
	version, err := strconv.ParseInt(r.PathValue("version"), 10, 64)
	if err != nil || version <= 0 {
		return 0, authz.ErrInvalidInput
	}
	return version, nil
}

// RegisterLayoutRoutes adds the layout routes. The service checks the rights.
func RegisterLayoutRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, layouts LayoutService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/layouts", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return layouts.List(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/layouts", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input layoutCreate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.Definition == nil {
			return nil, authz.ErrInvalidInput
		}
		return layouts.Create(r.Context(), proof, input.CommandID,
			presentation.LayoutInput{Name: input.Name, Definition: *input.Definition}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/layouts/{id}", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return layouts.Read(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/layouts/{id}/versions/{version}", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		version, err := layoutVersion(r)
		if err != nil {
			return nil, err
		}
		return layouts.ReadVersion(r.Context(), proof, id, version)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/layouts/{id}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input layoutUpdate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 || input.Definition == nil {
			return nil, authz.ErrInvalidInput
		}
		return layouts.Update(r.Context(), proof, id, input.ExpectedRevision,
			presentation.LayoutInput{Name: input.Name, Definition: *input.Definition}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/layouts/{id}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return layouts.Delete(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
