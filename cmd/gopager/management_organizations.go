package main

import (
	"net/http"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
)

type organizationMutation struct {
	SchemaVersion    int             `json:"schemaVersion"`
	CommandID        uuid.UUID       `json:"commandId"`
	ExpectedRevision int64           `json:"expectedRevision,string"`
	Name             string          `json:"name"`
	Defaults         *units.Defaults `json:"defaults"`
}

// registerOrganizations binds the revisioned Organization lifecycle.
func (api managementAPI) registerOrganizations(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/organizations", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListOrganizations(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/organizations", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input organizationMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil || input.ExpectedRevision != 0 || input.Defaults == nil {
			return nil, authz.ErrInvalidInput
		}
		return api.units.CreateOrganization(r.Context(), proof, input.CommandID,
			units.OrganizationInput{Name: input.Name, Defaults: *input.Defaults}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/organizations/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadOrganization(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/organizations/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input organizationMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil || input.Defaults == nil {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ChangeOrganization(r.Context(), proof, id, input.ExpectedRevision,
			units.OrganizationInput{Name: input.Name, Defaults: *input.Defaults}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/organizations/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
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
		return api.units.DeleteOrganization(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
