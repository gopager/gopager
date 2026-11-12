package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/units"
)

// AttributeSchemaService is the Attribute-schema side the routes call. *attributes.Service
// implements it.
type AttributeSchemaService interface {
	List(context.Context, identity.Proof, *uuid.UUID, units.PageInput) (units.Page[attributes.Schema], error)
	Read(context.Context, identity.Proof, string) (attributes.Schema, error)
	Create(context.Context, identity.Proof, uuid.UUID, attributes.SchemaInput, func(identity.Authentication) error) (attributes.Schema, error)
	Update(context.Context, identity.Proof, string, int64, attributes.SchemaInput, func(identity.Authentication) error) (attributes.Schema, error)
	Delete(context.Context, identity.Proof, string, int64, func(identity.Authentication) error) (attributes.Deleted, error)
}

// attributeSchemaContent is the content shared by the create and update bodies.
type attributeSchemaContent struct {
	Label       string                      `json:"label"`
	Description string                      `json:"description"`
	Subject     attributes.Subject          `json:"subject"`
	Fields      map[string]attributes.Field `json:"fields"`
	Agencies    []uuid.UUID                 `json:"agencies"`
	Deprecated  bool                        `json:"deprecated"`
}

// input converts the body content to the service input under the key.
func (c attributeSchemaContent) input(key string) attributes.SchemaInput {
	return attributes.SchemaInput{Key: key, Label: c.Label, Description: c.Description, Subject: c.Subject,
		Fields: c.Fields, Agencies: c.Agencies, Deprecated: c.Deprecated}
}

// attributeSchemaCreate is the body of POST /attribute-schemas.
type attributeSchemaCreate struct {
	SchemaVersion int       `json:"schemaVersion"`
	CommandID     uuid.UUID `json:"commandId"`
	Key           string    `json:"key"`
	attributeSchemaContent
}

// attributeSchemaUpdate is the body of PUT /attribute-schemas/{key}.
type attributeSchemaUpdate struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
	attributeSchemaContent
}

// RegisterAttributeSchemaRoutes adds the Attribute-schema routes. The service checks the rights.
func RegisterAttributeSchemaRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, schemas AttributeSchemaService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/attribute-schemas", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, filters, err := managementPageWith(r, "agency")
		if err != nil {
			return nil, err
		}
		var agency *uuid.UUID
		if raw, ok := filters["agency"]; ok {
			id, err := uuid.Parse(raw)
			if err != nil || id == uuid.Nil {
				return nil, authz.ErrInvalidInput
			}
			agency = &id
		}
		return schemas.List(r.Context(), proof, agency, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/attribute-schemas", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var body attributeSchemaCreate
		if err := decodeManagementJSON(r, &body); err != nil {
			return nil, err
		}
		if body.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return schemas.Create(r.Context(), proof, body.CommandID, body.input(body.Key), check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/attribute-schemas/{key}", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		return schemas.Read(r.Context(), proof, r.PathValue("key"))
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/attribute-schemas/{key}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var body attributeSchemaUpdate
		if err := decodeManagementJSON(r, &body); err != nil {
			return nil, err
		}
		if body.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		key := r.PathValue("key")
		return schemas.Update(r.Context(), proof, key, body.ExpectedRevision, body.input(key), check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/attribute-schemas/{key}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var body deleteMutation
		if err := decodeManagementJSON(r, &body); err != nil {
			return nil, err
		}
		if body.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return schemas.Delete(r.Context(), proof, r.PathValue("key"), body.ExpectedRevision, check)
	}, http.StatusOK))
}
