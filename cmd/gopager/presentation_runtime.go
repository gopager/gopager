package main

import (
	"net/http"

	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// registerPresentationRoutes binds the installation settings, layout assignment and own
// preference routes.
func registerPresentationRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator httpapi.Authenticator, c *core) error {
	runtime, err := composeDelivery(c)
	if err != nil {
		return err
	}
	installation, err := presentation.NewInstallationService(c.store, c.resolver, c.store, runtime.presentation)
	if err != nil {
		return err
	}
	assignments, err := presentation.NewAssignmentService(c.store, c.resolver, c.store, runtime.presentation, runtime.layoutStore)
	if err != nil {
		return err
	}
	preferences, err := identity.NewPreferenceService(c.store, c.resolver, c.store, c.identity)
	if err != nil {
		return err
	}
	httpapi.RegisterInstallationRoutes(mux, transport, authenticator, installation)
	httpapi.RegisterLayoutAssignmentRoutes(mux, transport, authenticator, assignments)
	httpapi.RegisterPreferenceRoutes(mux, transport, authenticator, preferences)
	return nil
}
