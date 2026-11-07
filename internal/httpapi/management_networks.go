package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/protocols"
)

// networkCreation is the create body of a Network.
type networkCreation struct {
	SchemaVersion    int                `json:"schemaVersion"`
	CommandID        uuid.UUID          `json:"commandId"`
	ExpectedRevision int64              `json:"expectedRevision,string"`
	Name             string             `json:"name"`
	Protocol         protocols.Protocol `json:"protocol"`
	AgencyID         uuid.UUID          `json:"agencyId"`
}

// networkRename is the update body of a Network; only the name can change.
type networkRename struct {
	SchemaVersion    int    `json:"schemaVersion"`
	ExpectedRevision int64  `json:"expectedRevision,string"`
	Name             string `json:"name"`
}

type networkManagementAPI struct {
	routeGuard
	service *ingest.NetworkAdministration
}

// RegisterNetworkManagementRoutes binds the revisioned Network lifecycle (list, create, read,
// rename, delete) in the management data envelope.
func RegisterNetworkManagementRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator,
	service *ingest.NetworkAdministration) {
	api := networkManagementAPI{routeGuard: routeGuard{transport: transport, authenticator: authenticator}, service: service}
	mux.HandleFunc("GET /api/v1/networks", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.service.ListNetworks(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/networks", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input networkCreation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.service.CreateNetwork(r.Context(), proof, input.CommandID,
			ingest.NetworkInput{Name: input.Name, Protocol: input.Protocol, AgencyID: input.AgencyID}, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/networks/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.ReadNetwork(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/networks/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input networkRename
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
		return api.service.RenameNetwork(r.Context(), proof, id, input.ExpectedRevision, input.Name, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/networks/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
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
		return api.service.DeleteNetwork(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
