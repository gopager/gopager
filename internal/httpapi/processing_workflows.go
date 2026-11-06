package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

type workflowMutation struct {
	SchemaVersion    int         `json:"schemaVersion"`
	CommandID        uuid.UUID   `json:"commandId"`
	ExpectedRevision int64       `json:"expectedRevision,string"`
	Name             string      `json:"name"`
	Scope            authz.Scope `json:"scope"`
	Enabled          *bool       `json:"enabled"`
}

type workflowVersionMutation struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	CommandID        uuid.UUID             `json:"commandId"`
	ExpectedRevision int64                 `json:"expectedRevision,string"`
	Definition       processing.Definition `json:"definition"`
}

func (input workflowMutation) metadata() processingmanagement.MetadataInput {
	return processingmanagement.MetadataInput{Name: input.Name, Scope: input.Scope, Enabled: input.Enabled}
}

func (api processingWorkflowAPI) registerWorkflows(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workflows", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.service.ListWorkflows(r.Context(), proof, processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.ReadWorkflow(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/workflows", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.service.CreateWorkflow(r.Context(), proof, input.CommandID, input.metadata(), check)
	}, http.StatusCreated))
	mux.HandleFunc("PUT /api/v1/workflows/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.service.UpdateWorkflow(r.Context(), proof, id, input.ExpectedRevision, input.metadata(), check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/workflows/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.DeleteWorkflow(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}

func (api processingWorkflowAPI) registerVersions(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		parent, err := managementID(r)
		if err != nil {
			return nil, err
		}
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.service.ListVersions(r.Context(), proof, parent, processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions/{version}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		parent, err := managementID(r)
		if err != nil {
			return nil, err
		}
		version, err := uuid.Parse(r.PathValue("version"))
		if err != nil || version == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.service.ReadVersion(r.Context(), proof, parent, version)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/workflows/{id}/versions", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowVersionMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		parent, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.service.SaveVersion(r.Context(), proof, input.CommandID, parent,
			input.ExpectedRevision, input.Definition, check)
	}, http.StatusCreated))
	mux.HandleFunc("POST /api/v1/workflows/{id}/versions/{version}/activate", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		parent, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		version, err := uuid.Parse(r.PathValue("version"))
		if err != nil || version == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.service.ActivateVersion(r.Context(), proof, parent, version, input.ExpectedRevision, check)
	}, http.StatusOK))
}
