package main

import (
	"net/http"

	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/readmodel"
)

// readSide is the composed Message read service with its search admission.
type readSide struct {
	service   *readmodel.Service
	admission *readmodel.Admission
}

// composeReadSide returns the process's Message read side, composing it on first use.
func composeReadSide(c *core) (*readSide, error) {
	if c.readSide != nil {
		return c.readSide, nil
	}
	cards, err := composeCardRenderer(c)
	if err != nil {
		return nil, err
	}
	service, err := readmodel.NewService(c.store, c.resolver, c.store, c.reads, c.reads, c.fields, readmodel.WithCards(cards))
	if err != nil {
		return nil, err
	}
	limit := c.searchAttempts
	if limit == 0 {
		limit = readmodel.DefaultSearchAttempts
	}
	admission, err := readmodel.NewAdmission(c.store, c.resolver, c.store, c.reads, limit)
	if err != nil {
		return nil, err
	}
	c.readSide = &readSide{service: service, admission: admission}
	return c.readSide, nil
}

// registerQueryRoutes composes the Message read side and binds its routes.
func registerQueryRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator httpapi.Authenticator, c *core) error {
	side, err := composeReadSide(c)
	if err != nil {
		return err
	}
	views, err := readmodel.NewSavedViews(c.store, c.resolver, c.store, c.reads, c.receipts, c.fields)
	if err != nil {
		return err
	}
	httpapi.RegisterMessageRoutes(mux, transport, authenticator, side.service, side.admission)
	httpapi.RegisterSavedViewRoutes(mux, transport, authenticator, views)
	return nil
}
