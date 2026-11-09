package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/readmodel"
	"github.com/gopager/gopager/internal/units"
)

// SavedViewService is the saved view side the routes call. *readmodel.SavedViews implements it.
type SavedViewService interface {
	List(context.Context, identity.Proof, units.PageInput) (units.Page[readmodel.SavedView], error)
	Read(context.Context, identity.Proof, uuid.UUID) (readmodel.SavedView, error)
	Create(context.Context, identity.Proof, uuid.UUID, readmodel.SavedViewInput, func(identity.Authentication) error) (readmodel.SavedView, error)
	Update(context.Context, identity.Proof, uuid.UUID, int64, string, readmodel.Definition, func(identity.Authentication) error) (readmodel.SavedView, error)
	Delete(context.Context, identity.Proof, uuid.UUID, int64, func(identity.Authentication) error) (commands.Result, error)
}

// savedViewCreate is the body of POST /saved-views.
type savedViewCreate struct {
	SchemaVersion int                   `json:"schemaVersion"`
	CommandID     uuid.UUID             `json:"commandId"`
	Ownership     string                `json:"ownership"`
	Name          string                `json:"name"`
	Definition    *readmodel.Definition `json:"definition"`
}

// savedViewUpdate is the body of PUT /saved-views/{id}.
type savedViewUpdate struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	ExpectedRevision int64                 `json:"expectedRevision,string"`
	Name             string                `json:"name"`
	Definition       *readmodel.Definition `json:"definition"`
}

// RegisterSavedViewRoutes adds the saved view routes. The guest may read the global views its
// grants allow; the shared route guard rejects its state-changing requests.
func RegisterSavedViewRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, views SavedViewService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/saved-views", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return views.List(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/saved-views", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input savedViewCreate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.Definition == nil {
			return nil, authz.ErrInvalidInput
		}
		return views.Create(r.Context(), proof, input.CommandID, readmodel.SavedViewInput{
			Ownership: input.Ownership, Name: input.Name, Definition: *input.Definition}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/saved-views/{id}", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return views.Read(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/saved-views/{id}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input savedViewUpdate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 || input.Definition == nil {
			return nil, authz.ErrInvalidInput
		}
		return views.Update(r.Context(), proof, id, input.ExpectedRevision, input.Name, *input.Definition, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/saved-views/{id}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return views.Delete(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
