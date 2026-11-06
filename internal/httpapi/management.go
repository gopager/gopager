// Package httpapi holds the HTTP adapters of the /api/v1 application routes: management,
// Gateway intake and administration, and Processing. Adapters translate requests into domain
// service calls and domain errors into the shared error body; they own no business rules and
// never import a store, so the composing binary decides which implementations back them.
package httpapi

import (
	"net/http"

	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/units"
)

type managementAPI struct {
	transport *identity.HTTPTransport
	units     *units.Service
	rules     *assignments.Service
}

type managementOperation func(*http.Request, identity.Proof, func(identity.Authentication) error) (any, error)

type deleteMutation struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

// RegisterManagementRoutes adds the current master-data and assignment management surfaces.
func RegisterManagementRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, unitService *units.Service, ruleService *assignments.Service) {
	api := managementAPI{transport: transport, units: unitService, rules: ruleService}
	api.registerAgencies(mux)
	api.registerOrganizations(mux)
	api.registerUnitTemplates(mux)
	api.registerUnits(mux)
	api.registerCollections(mux)
	api.registerRules(mux)
	api.registerCorrections(mux)
}

// route selects transport proof before invoking a domain service with transaction-local CSRF checking.
func (api managementAPI) route(operation managementOperation, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proof, err := api.transport.SelectProof(r)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		result, err := operation(r, proof, func(authentication identity.Authentication) error {
			return api.transport.CheckStateChange(r, authentication)
		})
		if err != nil {
			writeManagementError(w, err)
			return
		}
		writeManagementJSON(w, status, struct {
			SchemaVersion int `json:"schemaVersion"`
			Data          any `json:"data"`
		}{1, result})
	}
}
