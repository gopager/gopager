package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

type instanceMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	processingmanagement.InstanceInput
}

func (api processingWorkflowAPI) registerInstances(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/instances", api.route(func(r *http.Request, proof identity.Proof,
		_ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.instances.ListInstances(r.Context(), proof,
			processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/instances/{id}", api.route(func(r *http.Request, proof identity.Proof,
		_ func(identity.Authentication) error) (any, error) {
		id, err := managementPathID(r, "id")
		if err != nil {
			return nil, err
		}
		return api.instances.ReadInstance(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/instances", api.route(func(r *http.Request, proof identity.Proof,
		check func(identity.Authentication) error) (any, error) {
		var input instanceMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.instances.SaveInstance(r.Context(), proof, input.CommandID, uuid.Nil, 0,
			input.InstanceInput, check)
	}, http.StatusCreated))
	mux.HandleFunc("PUT /api/v1/instances/{id}", api.route(func(r *http.Request, proof identity.Proof,
		check func(identity.Authentication) error) (any, error) {
		var input instanceMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementPathID(r, "id")
		if err != nil || input.SchemaVersion != 1 || input.ExpectedRevision <= 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.instances.SaveInstance(r.Context(), proof, input.CommandID, id, input.ExpectedRevision,
			input.InstanceInput, check)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/instances/inspection", api.route(func(r *http.Request, proof identity.Proof,
		check func(identity.Authentication) error) (any, error) {
		var input struct {
			SchemaVersion int                            `json:"schemaVersion"`
			Main          processing.ProvisionedArtifact `json:"main"`
		}
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.instances.InspectArtifact(r.Context(), proof, input.Main)
	}, http.StatusOK))
}
