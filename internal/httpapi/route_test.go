package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

// rejectingAuthenticator refuses every credential and records the proofs it saw.
type rejectingAuthenticator struct{ proofs []identity.Proof }

func (a *rejectingAuthenticator) Authenticate(_ context.Context, proof identity.Proof) error {
	a.proofs = append(a.proofs, proof)
	return identity.ErrLoginRequired
}

func TestAuthenticatedRoutesRejectBadCredentialsBeforeReadingTheBody(t *testing.T) {
	transport, err := identity.NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	gateway := "/api/v1/gateways/" + uuid.NewString() + "/credentials"
	for _, test := range []struct {
		name, path string
		header     http.Header
		kind       identity.ProofKind
	}{
		{"management bearer", "/api/v1/agencies", http.Header{"Authorization": {"Bearer " + secret}}, identity.AccessKeyProof},
		{"management session", "/api/v1/agencies", http.Header{"Cookie": {identity.SessionCookieName + "=" + secret}}, identity.SessionProof},
		{"gateway administration", gateway, http.Header{"Authorization": {"Bearer " + secret}}, identity.AccessKeyProof},
		{"processing", "/api/v1/workflows", http.Header{"Authorization": {"Bearer " + secret}}, identity.AccessKeyProof},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth := &rejectingAuthenticator{}
			mux := http.NewServeMux()
			RegisterManagementRoutes(mux, transport, auth, nil, nil)
			RegisterGatewayManagementRoutes(mux, transport, auth, nil)
			RegisterProcessingRoutes(mux, transport, auth, ProcessingRoutes{Workflows: &processingmanagement.Service{}})
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			request.Body = explodingBody{t}
			request.Header = test.header.Clone()
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), `"LOGIN_REQUIRED"`) {
				t.Fatalf("status %d body %s, want 401 LOGIN_REQUIRED", recorder.Code, recorder.Body)
			}
			if len(auth.proofs) != 1 || auth.proofs[0].Kind != test.kind || auth.proofs[0].Secret != secret {
				t.Fatalf("authenticator saw %v, want one %s proof", auth.proofs, test.kind)
			}
		})
	}
}

func TestMalformedCredentialIsRejectedWithoutAuthenticatorOrBody(t *testing.T) {
	transport, err := identity.NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	auth := &rejectingAuthenticator{}
	mux := http.NewServeMux()
	RegisterManagementRoutes(mux, transport, auth, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agencies", nil)
	request.Body = explodingBody{t}
	request.Header.Set("Authorization", "Basic abc")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || len(auth.proofs) != 0 {
		t.Fatalf("status %d proofs %v, want 401 without authenticator", recorder.Code, auth.proofs)
	}
}
