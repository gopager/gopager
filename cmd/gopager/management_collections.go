package main

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
)

type collectionMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Name             string    `json:"name"`
}

// registerCollections exposes the approved name/membership lifecycle, not general bulk operations.
func (api managementAPI) registerCollections(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/unit-collections", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.units.ListCollections(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/unit-collections/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.units.ReadCollection(r.Context(), proof, id)
	}, http.StatusOK))
	for _, method := range []string{"POST", "PUT"} {
		status, path := http.StatusCreated, "/api/v1/unit-collections"
		if method == "PUT" {
			status, path = http.StatusOK, "/api/v1/unit-collections/{id}"
		}
		mux.HandleFunc(method+" "+path, api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
			var input collectionMutation
			if err := decodeManagementJSON(r, &input); err != nil {
				return nil, err
			}
			if input.SchemaVersion != 1 {
				return nil, authz.ErrInvalidInput
			}
			if r.Method == "POST" {
				if input.ExpectedRevision != 0 {
					return nil, authz.ErrInvalidInput
				}
				return api.units.CreateCollection(r.Context(), proof, input.CommandID, input.Name, check)
			}
			id, err := managementID(r)
			if err != nil || input.CommandID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			return api.units.ChangeCollection(r.Context(), proof, id, input.ExpectedRevision, input.Name, check)
		}, status))
	}
	mux.HandleFunc("DELETE /api/v1/unit-collections/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
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
		return api.units.DeleteCollection(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
