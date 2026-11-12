package main

import (
	"net/http"

	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
)

// composeAttributeSchemas builds the Attribute-schema service over the core's catalog store.
func composeAttributeSchemas(c *core) (*attributes.Service, error) {
	return attributes.NewService(c.store, c.resolver, c.store, c.attributes, c.receipts, c.facts)
}

// registerAttributeRoutes binds the Attribute-schema routes.
func registerAttributeRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator httpapi.Authenticator, c *core) error {
	service, err := composeAttributeSchemas(c)
	if err != nil {
		return err
	}
	httpapi.RegisterAttributeSchemaRoutes(mux, transport, authenticator, service)
	return nil
}
