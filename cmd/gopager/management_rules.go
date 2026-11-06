package main

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/protocols"
)

type ruleMutation struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	CommandID        uuid.UUID             `json:"commandId"`
	ID               uuid.UUID             `json:"id"`
	ExpectedRevision int64                 `json:"expectedRevision,string"`
	NetworkID        uuid.UUID             `json:"networkId"`
	UnitID           uuid.UUID             `json:"unitId"`
	Protocol         protocols.Protocol    `json:"protocol"`
	Selector         *assignments.Selector `json:"selector"`
	ValidFrom        *time.Time            `json:"validFrom"`
	ValidUntil       *time.Time            `json:"validUntil"`
	Enabled          *bool                 `json:"enabled"`
}

// input requires explicit full mutable values and leaves reference/Protocol validation to the owner.
func (input ruleMutation) input() (assignments.RuleInput, error) {
	if input.Selector == nil || input.Enabled == nil {
		return assignments.RuleInput{}, authz.ErrInvalidInput
	}
	return assignments.RuleInput{NetworkID: input.NetworkID, UnitID: input.UnitID, Protocol: input.Protocol,
		Selector: *input.Selector, ValidFrom: input.ValidFrom, ValidUntil: input.ValidUntil, Enabled: *input.Enabled}, nil
}

// registerRules exposes current configuration and pure preview, never arbitrary public matching.
func (api managementAPI) registerRules(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/address-rules", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.rules.ListRules(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/address-rules/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.rules.ReadRule(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/address-rules/preview", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input ruleMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		values, err := input.input()
		if err != nil {
			return nil, err
		}
		if err := api.rules.Preview(r.Context(), proof, input.ID, input.ExpectedRevision, values, check); err != nil {
			return nil, err
		}
		return struct {
			Valid bool `json:"valid"`
		}{true}, nil
	}, http.StatusOK))
	for _, method := range []string{"POST", "PUT"} {
		status, path := http.StatusCreated, "/api/v1/address-rules"
		if method == "PUT" {
			status, path = http.StatusOK, "/api/v1/address-rules/{id}"
		}
		mux.HandleFunc(method+" "+path, api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
			var input ruleMutation
			if err := decodeManagementJSON(r, &input); err != nil {
				return nil, err
			}
			if input.SchemaVersion != 1 || input.ID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			values, err := input.input()
			if err != nil {
				return nil, err
			}
			if r.Method == "POST" {
				if input.ExpectedRevision != 0 {
					return nil, authz.ErrInvalidInput
				}
				return api.rules.CreateRule(r.Context(), proof, input.CommandID, values, check)
			}
			id, err := managementID(r)
			if err != nil || input.CommandID != uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			return api.rules.UpdateRule(r.Context(), proof, id, input.ExpectedRevision, values, check)
		}, status))
	}
	mux.HandleFunc("DELETE /api/v1/address-rules/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
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
		result, err := api.rules.DeleteRule(r.Context(), proof, id, input.ExpectedRevision, check)
		if err != nil {
			return nil, err
		}
		return units.Deletion{Result: result, Status: "pending"}, nil
	}, http.StatusAccepted))
}
