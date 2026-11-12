package httpapi

import (
	"context"
	"net/http"

	"github.com/gopager/gopager/internal/identity"
)

// PreferenceService is the preference side the routes call. *identity.PreferenceService
// implements it.
type PreferenceService interface {
	Read(context.Context, identity.Proof) (identity.Preferences, error)
	Save(context.Context, identity.Proof, int64, identity.PreferencesInput, func(identity.Authentication) error) (identity.Preferences, error)
}

// preferencesUpdate is the body of PUT /me/preferences. ExpectedRevision 0 (or absent) is the
// first save.
type preferencesUpdate struct {
	SchemaVersion    int    `json:"schemaVersion"`
	ExpectedRevision int64  `json:"expectedRevision,string"`
	Locale           string `json:"locale"`
	TimeZone         string `json:"timeZone"`
	TimeGapMinutes   int    `json:"timeGapMinutes"`
}

// RegisterPreferenceRoutes adds the routes of the caller's own preferences. The service rejects
// everything but an interactive account acting through its own Session.
func RegisterPreferenceRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, preferences PreferenceService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/me/preferences", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		return preferences.Read(r.Context(), proof)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/me/preferences", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input preferencesUpdate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if err := checkSchemaVersion(input.SchemaVersion); err != nil {
			return nil, err
		}
		return preferences.Save(r.Context(), proof, input.ExpectedRevision, identity.PreferencesInput{
			Locale: input.Locale, TimeZone: input.TimeZone, TimeGapMinutes: input.TimeGapMinutes}, check)
	}, http.StatusOK))
}
