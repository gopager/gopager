package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/units"
)

// SubscriptionService is the Subscription side the routes call. *delivery.Subscriptions
// implements it.
type SubscriptionService interface {
	List(context.Context, identity.Proof, units.PageInput) (units.Page[delivery.Subscription], error)
	Read(context.Context, identity.Proof, uuid.UUID) (delivery.Subscription, error)
	Create(context.Context, identity.Proof, uuid.UUID, delivery.SubscriptionInput, func(identity.Authentication) error) (delivery.Subscription, error)
	Update(context.Context, identity.Proof, uuid.UUID, int64, delivery.SubscriptionInput, func(identity.Authentication) error) (delivery.Subscription, error)
	Delete(context.Context, identity.Proof, uuid.UUID, int64, func(identity.Authentication) error) (commands.Result, error)
}

// subscriptionCreate is the body of POST /subscriptions.
type subscriptionCreate struct {
	SchemaVersion int       `json:"schemaVersion"`
	CommandID     uuid.UUID `json:"commandId"`
	delivery.SubscriptionInput
}

// subscriptionUpdate is the body of PUT /subscriptions/{id}.
type subscriptionUpdate struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
	delivery.SubscriptionInput
}

// RegisterSubscriptionRoutes adds the Subscription routes. The service checks the rights; the
// guest cannot manage Subscriptions.
func RegisterSubscriptionRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator,
	subscriptions SubscriptionService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/subscriptions", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return subscriptions.List(r.Context(), proof, page)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/subscriptions", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input subscriptionCreate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return subscriptions.Create(r.Context(), proof, input.CommandID, input.SubscriptionInput, check)
	}, http.StatusCreated))
	mux.HandleFunc("GET /api/v1/subscriptions/{id}", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return subscriptions.Read(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/subscriptions/{id}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input subscriptionUpdate
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return subscriptions.Update(r.Context(), proof, id, input.ExpectedRevision, input.SubscriptionInput, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/subscriptions/{id}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return subscriptions.Delete(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}
