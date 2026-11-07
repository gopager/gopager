package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
)

type gatewayManagementAPI struct {
	routeGuard
	service *ingest.GatewayAdministration
}

type gatewayRevisionResult struct {
	SchemaVersion int   `json:"schemaVersion"`
	Revision      int64 `json:"revision,string"`
}

// gatewayNetworksResult is the Network replacement response: the new revision and the IDs of the
// queued re-resolution corrections.
type gatewayNetworksResult struct {
	SchemaVersion int `json:"schemaVersion"`
	ingest.GatewayNetworksResult
}

type gatewayNetworksMutation struct {
	SchemaVersion    int                          `json:"schemaVersion"`
	ExpectedRevision int64                        `json:"expectedRevision,string"`
	Networks         []ingest.GatewayNetworkInput `json:"networks"`
}

type gatewayCredentialMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
}

type gatewayCredentialIssueResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	CredentialID  string `json:"credentialId"`
	Secret        string `json:"secret,omitempty"`
	Status        string `json:"status"`
	Revision      int64  `json:"revision,string"`
	replayed      bool
}

// responseStatus answers 200 for a replayed command, which created nothing, and the route's own
// success status otherwise.
func (r gatewayCredentialIssueResponse) responseStatus(fallback int) int {
	if r.replayed {
		return http.StatusOK
	}
	return fallback
}

// RegisterGatewayManagementRoutes adds the Gateway lifecycle, diagnostics and history routes in the
// management data envelope, and the Network assignment and credential routes, whose success bodies
// are not wrapped in the envelope.
func RegisterGatewayManagementRoutes(mux *http.ServeMux, transport *identity.HTTPTransport,
	authenticator Authenticator, service *ingest.GatewayAdministration) {
	api := gatewayManagementAPI{routeGuard: routeGuard{transport: transport, authenticator: authenticator},
		service: service}
	api.registerGatewayLifecycle(mux)
	mux.HandleFunc("PUT /api/v1/gateways/{id}/networks", api.route(func(r *http.Request, proof identity.Proof,
		check func(identity.Authentication) error) (any, error) {
		gateway, err := managementID(r)
		if err != nil {
			return nil, err
		}
		var input gatewayNetworksMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		result, err := api.service.ConfigureNetworks(r.Context(), proof, gateway, input.ExpectedRevision,
			input.Networks, check)
		if err != nil {
			return nil, err
		}
		return gatewayNetworksResult{SchemaVersion: 1, GatewayNetworksResult: result}, nil
	}, http.StatusOK, withoutEnvelope()))
	mux.HandleFunc("GET /api/v1/gateways/{id}/credentials", api.route(func(r *http.Request, proof identity.Proof,
		_ func(identity.Authentication) error) (any, error) {
		gateway, err := managementID(r)
		if err != nil {
			return nil, err
		}
		credentials, err := api.service.ReadCredentialStatus(r.Context(), proof, gateway)
		if err != nil {
			return nil, err
		}
		return struct {
			SchemaVersion int                            `json:"schemaVersion"`
			Credentials   []ingest.GatewayCredentialView `json:"credentials"`
		}{1, credentials}, nil
	}, http.StatusOK, withoutEnvelope()))
	mux.HandleFunc("POST /api/v1/gateways/{id}/credentials", api.route(func(r *http.Request, proof identity.Proof,
		check func(identity.Authentication) error) (any, error) {
		gateway, err := managementID(r)
		if err != nil {
			return nil, err
		}
		var input gatewayCredentialMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		result, err := api.service.IssueCredential(r.Context(), proof, gateway, input.CommandID,
			input.ExpectedRevision, check)
		if err != nil {
			return nil, err
		}
		return gatewayCredentialIssueResponse{SchemaVersion: 1, CredentialID: result.CredentialID.String(),
			Secret: result.Secret, Status: result.Status, Revision: result.Revision, replayed: result.Replayed}, nil
	}, http.StatusCreated, withoutEnvelope()))
	mux.HandleFunc("DELETE /api/v1/gateways/{id}/credentials/{credentialId}", api.route(func(r *http.Request,
		proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		gateway, err := managementID(r)
		if err != nil {
			return nil, err
		}
		credential, err := uuid.Parse(r.PathValue("credentialId"))
		if err != nil || credential == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		var input gatewayCredentialMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		revision, err := api.service.RevokeCredential(r.Context(), proof, gateway, credential,
			input.CommandID, input.ExpectedRevision, check)
		if err != nil {
			return nil, err
		}
		return gatewayRevisionResult{SchemaVersion: 1, Revision: revision}, nil
	}, http.StatusOK, withoutEnvelope()))
}
