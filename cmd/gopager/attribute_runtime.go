package main

import (
	"net/http"

	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/attributestore"
)

// composeAttributeSchemas builds the Attribute-schema service and the store that also serves
// the catalog to the field registry.
func composeAttributeSchemas(c *core) (*attributes.Service, *attributestore.Store, error) {
	store, err := attributestore.New(c.store)
	if err != nil {
		return nil, nil, err
	}
	service, err := attributes.NewService(c.store, c.resolver, c.store, store, c.receipts)
	if err != nil {
		return nil, nil, err
	}
	return service, store, nil
}

// registerAttributeRoutes binds the Attribute-schema routes.
func registerAttributeRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator httpapi.Authenticator, c *core) error {
	service, _, err := composeAttributeSchemas(c)
	if err != nil {
		return err
	}
	httpapi.RegisterAttributeSchemaRoutes(mux, transport, authenticator, service)
	return nil
}
