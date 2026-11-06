package httpapi

import (
	"context"
	"net/http"

	"github.com/gopager/gopager/internal/identity"
)

// Authenticator is the cheap early credential check of the authenticated routes. It is satisfied
// by *identity.Preauthenticator.
type Authenticator interface {
	Authenticate(context.Context, identity.Proof) error
}

// managementOperation runs one route's domain call. check is the transport's state-change check;
// the service invokes it with its own, transaction-local authentication.
type managementOperation func(*http.Request, identity.Proof, func(identity.Authentication) error) (any, error)

// statusResult lets a result choose its success status, for example a replayed command answering
// 200 where the first execution answered 201.
type statusResult interface {
	responseStatus(fallback int) int
}

type routeOption func(*routeConfig)

type routeConfig struct{ bare bool }

// withoutEnvelope writes the result as the response body instead of wrapping it in
// {"schemaVersion":1,"data":...}.
func withoutEnvelope() routeOption {
	return func(c *routeConfig) { c.bare = true }
}

// routeGuard is the shared HTTP-level authentication of every management, Gateway administration
// and Processing route.
type routeGuard struct {
	transport     *identity.HTTPTransport
	authenticator Authenticator
}

// safeMethod reports whether the method only reads. The guest account may only view and filter,
// so an anonymous proof is limited to these methods.
func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// route builds the handler for one operation. It selects the transport proof and, for a session
// or access key, has the Authenticator reject an unknown, revoked, expired or inactive credential
// with LOGIN_REQUIRED before the operation can read the request body, so an unauthenticated
// caller cannot make the server buffer and decode up to the size limit. That check is advisory
// and runs in its own short transaction. The real authority (current grants, restrictions) and
// the CSRF/Origin check are still resolved by the service inside the operation's transaction,
// where they stay consistent with the data the operation changes. Both exist on purpose: early
// rejection and body protection here, transactional consistency there. An anonymous proof
// (the guest) is read-only: on any method except GET, HEAD and OPTIONS it is rejected with
// LOGIN_REQUIRED before the body is read. On safe methods it passes through, because the guest
// account's rights are only known inside the operation's transaction.
func (g routeGuard) route(operation managementOperation, status int, options ...routeOption) http.HandlerFunc {
	var config routeConfig
	for _, option := range options {
		option(&config)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		proof, err := g.transport.SelectProof(r)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		if proof.Kind == identity.AnonymousProof && !safeMethod(r.Method) {
			writeManagementError(w, identity.ErrLoginRequired)
			return
		}
		if err := g.authenticator.Authenticate(r.Context(), proof); err != nil {
			writeManagementError(w, err)
			return
		}
		result, err := operation(r, proof, func(authentication identity.Authentication) error {
			return g.transport.CheckStateChange(r, authentication)
		})
		if err != nil {
			writeManagementError(w, err)
			return
		}
		responseStatus := status
		if shaped, ok := result.(statusResult); ok {
			responseStatus = shaped.responseStatus(status)
		}
		if config.bare {
			writeManagementJSON(w, responseStatus, result)
			return
		}
		writeManagementJSON(w, responseStatus, struct {
			SchemaVersion int `json:"schemaVersion"`
			Data          any `json:"data"`
		}{1, result})
	}
}
