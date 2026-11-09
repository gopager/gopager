package httpapi

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/readmodel"
)

// MessageReads is the Message read side the routes call. *readmodel.Service implements it.
type MessageReads interface {
	Feed(context.Context, identity.Proof, readmodel.FeedRequest) (readmodel.FeedPage, error)
	Detail(context.Context, identity.Proof, uuid.UUID) (readmodel.DetailItem, error)
	FilterOptions(context.Context, identity.Proof) (readmodel.OptionsItem, error)
}

// SearchAdmission limits search requests per caller. *readmodel.Admission implements it.
type SearchAdmission interface {
	Admit(ctx context.Context, proof identity.Proof, source string) error
}

// RegisterMessageRoutes adds the Message feed, detail and filter-options reads. The guest may
// use them under its grants like any other caller. Every feed page is admitted against the
// caller's search window after its parameters parsed and before the read; a request with
// invalid parameters is not counted.
func RegisterMessageRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, reads MessageReads, admission SearchAdmission) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/messages", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			return nil, authz.ErrInvalidInput
		}
		request, err := readmodel.ParseFeedRequest(values)
		if err != nil {
			return nil, err
		}
		if err := admission.Admit(r.Context(), proof, identity.ClientSource(r)); err != nil {
			return nil, err
		}
		return reads.Feed(r.Context(), proof, request)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/messages/filter-options", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, authz.ErrInvalidInput
		}
		return reads.FilterOptions(r.Context(), proof)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/messages/{id}", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		if r.URL.RawQuery != "" {
			return nil, authz.ErrInvalidInput
		}
		return reads.Detail(r.Context(), proof, id)
	}, http.StatusOK))
}
