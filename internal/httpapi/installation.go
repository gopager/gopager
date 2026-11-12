package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// InstallationService is the installation settings side the routes call.
// *presentation.InstallationService implements it.
type InstallationService interface {
	Read(context.Context) (presentation.Installation, error)
	Update(context.Context, identity.Proof, int64, presentation.InstallationInput, func(identity.Authentication) error) (presentation.Installation, error)
}

// installationUpdate is the body of PUT /installation. Absent banner, severity and defaultLayoutId
// mean none, because the request replaces every field.
type installationUpdate struct {
	SchemaVersion    int        `json:"schemaVersion"`
	ExpectedRevision int64      `json:"expectedRevision,string"`
	Title            string     `json:"title"`
	Banner           *string    `json:"banner"`
	Severity         *string    `json:"severity"`
	DefaultLayoutID  *uuid.UUID `json:"defaultLayoutId"`
}

// checkSchemaVersion accepts an absent (0) or the current (1) schemaVersion of a request body.
func checkSchemaVersion(version int) error {
	if version != 0 && version != 1 {
		return authz.ErrInvalidInput
	}
	return nil
}

// RegisterInstallationRoutes adds the installation routes. GET needs no credential: clients show
// the title and banner before anyone signs in, and the guest reads them too. PUT is guarded like
// every management write; the service checks the right.
func RegisterInstallationRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, installation InstallationService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/installation", func(w http.ResponseWriter, r *http.Request) {
		result, err := installation.Read(r.Context())
		if err != nil {
			writeManagementError(w, err)
			return
		}
		writeManagementJSON(w, http.StatusOK, struct {
			SchemaVersion int `json:"schemaVersion"`
			Data          any `json:"data"`
		}{1, result})
	})
	mux.HandleFunc("PUT /api/v1/installation", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input installationUpdate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if err := checkSchemaVersion(input.SchemaVersion); err != nil {
			return nil, err
		}
		return installation.Update(r.Context(), proof, input.ExpectedRevision, presentation.InstallationInput{
			Title: input.Title, Banner: input.Banner, Severity: input.Severity, DefaultLayoutID: input.DefaultLayoutID}, check)
	}, http.StatusOK))
}
