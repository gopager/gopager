package main

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
)

type templateMutation struct {
	SchemaVersion    int             `json:"schemaVersion"`
	CommandID        uuid.UUID       `json:"commandId"`
	ExpectedRevision int64           `json:"expectedRevision,string"`
	Name             string          `json:"name"`
	OrganizationID   json.RawMessage `json:"organizationId"`
	Settings         *units.Settings `json:"settings"`
}

func (input templateMutation) organizationID(required bool) (*uuid.UUID, error) {
	if len(input.OrganizationID) == 0 {
		if required {
			return nil, authz.ErrInvalidInput
		}
		return nil, nil
	}
	if string(input.OrganizationID) == "null" {
		return nil, nil
	}
	var id uuid.UUID
	if err := json.Unmarshal(input.OrganizationID, &id); err != nil || id == uuid.Nil {
		return nil, authz.ErrInvalidInput
	}
	return &id, nil
}

// registerUnitTemplates binds standalone template management and snapshot operations.
func (api managementAPI) registerUnitTemplates(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/unit-templates", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListUnitTemplates(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/unit-templates", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input templateMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil || input.ExpectedRevision != 0 || input.Settings == nil {
			return nil, authz.ErrInvalidInput
		}
		organizationID, err := input.organizationID(false)
		if err != nil {
			return nil, err
		}
		return api.units.CreateUnitTemplate(r.Context(), proof, input.CommandID, units.UnitTemplateInput{
			Name: input.Name, OrganizationID: organizationID, Settings: *input.Settings,
		}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/unit-templates/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadUnitTemplate(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/unit-templates/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input templateMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil || input.Settings == nil {
			return nil, authz.ErrInvalidInput
		}
		organizationID, err := input.organizationID(true)
		if err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ChangeUnitTemplate(r.Context(), proof, id, input.ExpectedRevision, units.UnitTemplateInput{
			Name: input.Name, OrganizationID: organizationID, Settings: *input.Settings,
		}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/unit-templates/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
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
		return api.units.DeleteUnitTemplate(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
