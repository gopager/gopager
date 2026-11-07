package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
)

// gatewayCreation is the create body of a Gateway; networks may be empty.
type gatewayCreation struct {
	SchemaVersion    int                          `json:"schemaVersion"`
	CommandID        uuid.UUID                    `json:"commandId"`
	ExpectedRevision int64                        `json:"expectedRevision,string"`
	Name             string                       `json:"name"`
	Networks         []ingest.GatewayNetworkInput `json:"networks"`
}

// gatewayUpdate is the update body of a Gateway: name and enablement.
type gatewayUpdate struct {
	SchemaVersion    int    `json:"schemaVersion"`
	ExpectedRevision int64  `json:"expectedRevision,string"`
	Name             string `json:"name"`
	Enabled          *bool  `json:"enabled"`
}

// registerGatewayLifecycle binds the revisioned Gateway lifecycle (list, create, read, update,
// delete) and the diagnostics and history reads in the management data envelope.
func (api gatewayManagementAPI) registerGatewayLifecycle(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/gateways", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.service.ListGateways(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/gateways", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input gatewayCreation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.service.CreateGateway(r.Context(), proof, input.CommandID,
			ingest.GatewayInput{Name: input.Name, Networks: input.Networks}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/gateways/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.ReadGateway(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/gateways/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input gatewayUpdate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.Enabled == nil {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.ChangeGateway(r.Context(), proof, id, input.ExpectedRevision,
			ingest.GatewayChange{Name: input.Name, Enabled: *input.Enabled}, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/gateways/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
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
		return api.service.DeleteGateway(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/gateways/{id}/diagnostics", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.ReadDiagnostics(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/gateways/{id}/history", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.service.ListTransitions(r.Context(), proof, id, page)
	}, http.StatusOK))
}
