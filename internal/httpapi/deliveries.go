package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/units"
)

// DeliveryService is the Delivery side the routes call. *delivery.Deliveries implements it.
type DeliveryService interface {
	List(context.Context, identity.Proof, delivery.DeliveryFilter, units.PageInput) (units.Page[delivery.Delivery], error)
	Read(context.Context, identity.Proof, uuid.UUID) (delivery.Delivery, error)
	Retry(context.Context, identity.Proof, uuid.UUID, int64, func(identity.Authentication) error) (delivery.Delivery, error)
	Redeliver(context.Context, identity.Proof, uuid.UUID, uuid.UUID, bool, func(identity.Authentication) error) (delivery.Delivery, error)
}

// deliveryRetry is the body of POST /deliveries/{id}/retry.
type deliveryRetry struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

// deliveryRedeliver is the body of POST /deliveries/{id}/redeliver.
type deliveryRedeliver struct {
	SchemaVersion        int       `json:"schemaVersion"`
	CommandID            uuid.UUID `json:"commandId"`
	ConfirmDuplicateRisk bool      `json:"confirmDuplicateRisk"`
}

// deliveryFilter parses the optional subscriptionId and state query parameters.
func deliveryFilter(r *http.Request) (delivery.DeliveryFilter, error) {
	query := r.URL.Query()
	filter := delivery.DeliveryFilter{State: query.Get("state")}
	if value := query.Get("subscriptionId"); value != "" {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return delivery.DeliveryFilter{}, authz.ErrInvalidInput
		}
		filter.SubscriptionID = &id
	}
	return filter, nil
}

// RegisterDeliveryRoutes adds the Delivery result, retry and redelivery routes. The service
// checks the rights.
func RegisterDeliveryRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator,
	deliveries DeliveryService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/deliveries", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		filter, err := deliveryFilter(r)
		if err != nil {
			return nil, err
		}
		return deliveries.List(r.Context(), proof, filter, page)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/deliveries/{id}", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return deliveries.Read(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/deliveries/{id}/retry", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deliveryRetry
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return deliveries.Retry(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/deliveries/{id}/redeliver", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deliveryRedeliver
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return deliveries.Redeliver(r.Context(), proof, id, input.CommandID, input.ConfirmDuplicateRisk, check)
	}, http.StatusCreated))
}
