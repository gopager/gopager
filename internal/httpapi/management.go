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
	routeGuard
	units *units.Service
	rules *assignments.Service
}

type deleteMutation struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

// RegisterManagementRoutes adds the current master-data and assignment management surfaces.
func RegisterManagementRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, unitService *units.Service, ruleService *assignments.Service) {
	api := managementAPI{routeGuard: routeGuard{transport: transport, authenticator: authenticator},
		units: unitService, rules: ruleService}
	api.registerAgencies(mux)
	api.registerOrganizations(mux)
	api.registerUnitTemplates(mux)
	api.registerUnits(mux)
	api.registerCollections(mux)
	api.registerRules(mux)
	api.registerCorrections(mux)
}
