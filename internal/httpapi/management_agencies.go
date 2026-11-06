package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/units"
)

// agencyMutation is the create and rename body of an Agency.
type agencyMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Name             string    `json:"name"`
}

// registerAgencies binds the revisioned Agency lifecycle: list, create, read, rename, delete.
func (api managementAPI) registerAgencies(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agencies", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListAgencies(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/agencies", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input agencyMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.units.CreateAgency(r.Context(), proof, input.CommandID, units.AgencyInput{Name: input.Name}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/agencies/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadAgency(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/agencies/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input agencyMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ChangeAgency(r.Context(), proof, id, input.ExpectedRevision, units.AgencyInput{Name: input.Name}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/agencies/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.DeleteAgency(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
