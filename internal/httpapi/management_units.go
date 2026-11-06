package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/units"
)

type unitMutation struct {
	SchemaVersion    int                      `json:"schemaVersion"`
	CommandID        uuid.UUID                `json:"commandId"`
	ExpectedRevision int64                    `json:"expectedRevision,string"`
	Name             string                   `json:"name"`
	AgencyID         uuid.UUID                `json:"agencyId"`
	CollectionIDs    *[]uuid.UUID             `json:"collectionIds"`
	OrganizationID   json.RawMessage          `json:"organizationId"`
	Settings         *units.Settings          `json:"settings"`
	NewOrganization  *units.OrganizationInput `json:"newOrganization"`
}

func (input unitMutation) organizationID(required bool) (*uuid.UUID, error) {
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

type fromTemplateMutation struct {
	SchemaVersion int          `json:"schemaVersion"`
	CommandID     uuid.UUID    `json:"commandId"`
	TemplateID    uuid.UUID    `json:"templateId"`
	AgencyID      uuid.UUID    `json:"agencyId"`
	Name          string       `json:"name"`
	CollectionIDs *[]uuid.UUID `json:"collectionIds"`
}

type bulkUnitMutation struct {
	SchemaVersion int                  `json:"schemaVersion"`
	CommandID     uuid.UUID            `json:"commandId"`
	Operation     string               `json:"operation"`
	Items         []units.BulkUnitItem `json:"items"`
}

// registerUnits binds complete Unit replacements and bounded bulk operations.
func (api managementAPI) registerUnits(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/units", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListUnits(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/units/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadUnit(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/units/from-template", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input fromTemplateMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CollectionIDs == nil {
			return nil, authz.ErrInvalidInput
		}
		return api.units.CreateUnitFromTemplate(r.Context(), proof, input.CommandID, input.TemplateID,
			units.UnitInput{Name: input.Name, AgencyID: input.AgencyID, CollectionIDs: *input.CollectionIDs}, check)
	}, http.StatusCreated))
	mux.HandleFunc("POST /api/v1/units/bulk/preview", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input bulkUnitMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.units.PreviewBulkUnits(r.Context(), proof, units.BulkUnitInput{
			Operation: input.Operation, Items: input.Items,
		}, check)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/units/bulk", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input bulkUnitMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		results, err := api.units.BulkUnits(r.Context(), proof, input.CommandID,
			units.BulkUnitInput{Operation: input.Operation, Items: input.Items}, check)
		if err != nil {
			return nil, err
		}
		return struct {
			Items     []units.BulkUnitResult `json:"items"`
			CommandID uuid.UUID              `json:"commandId"`
			Operation string                 `json:"operation"`
		}{Items: results, CommandID: input.CommandID, Operation: input.Operation}, nil
	}, http.StatusOK))
	for _, method := range []string{"POST", "PUT"} {
		status, path := http.StatusCreated, "/api/v1/units"
		if method == "PUT" {
			status, path = http.StatusOK, "/api/v1/units/{id}"
		}
		mux.HandleFunc(method+" "+path, api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
			var input unitMutation
			if err := decodeManagementJSON(r, &input); err != nil {
				return nil, err
			}
			if input.SchemaVersion != 1 || input.CollectionIDs == nil || input.Settings == nil {
				return nil, authz.ErrInvalidInput
			}
			organizationID, err := input.organizationID(r.Method == "PUT")
			if err != nil {
				return nil, err
			}
			values := units.UnitInput{Name: input.Name, AgencyID: input.AgencyID, OrganizationID: organizationID,
				CollectionIDs: *input.CollectionIDs, Settings: *input.Settings, NewOrganization: input.NewOrganization}
			if r.Method == "PUT" && input.NewOrganization != nil {
				return nil, authz.ErrInvalidInput
			}
			if r.Method == "POST" {
				if input.ExpectedRevision != 0 {
					return nil, authz.ErrInvalidInput
				}
				return api.units.CreateUnit(r.Context(), proof, input.CommandID, values, check)
			}
			id, err := managementID(r)
			if err != nil || input.CommandID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			return api.units.ChangeUnit(r.Context(), proof, id, input.ExpectedRevision, values, check)
		}, status))
	}
	mux.HandleFunc("DELETE /api/v1/units/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
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
		return api.units.DeleteUnit(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusAccepted))
}
