package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
)

// fakeIntake rejects every bearer and records whether any later stage ran.
type fakeIntake struct{ submitted, reported bool }

func (f *fakeIntake) Authenticate(context.Context, string) error { return identity.ErrLoginRequired }

func (f *fakeIntake) Submit(context.Context, string, ingest.Submission, int) (ingest.Receipt, error) {
	f.submitted = true
	return ingest.Receipt{}, nil
}

func (f *fakeIntake) ReportState(context.Context, string, ingest.GatewayStateReport) error {
	f.reported = true
	return nil
}

// explodingBody fails the test when anything reads it.
type explodingBody struct{ t *testing.T }

func (b explodingBody) Read([]byte) (int, error) {
	b.t.Error("request body was read before the Gateway was authenticated")
	return 0, errors.New("body must not be read")
}

func (explodingBody) Close() error { return nil }

func TestIngestAuthenticatesBeforeReadingTheBody(t *testing.T) {
	secret, _, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/transmissions", "/api/v1/gateway/state"} {
		t.Run(path, func(t *testing.T) {
			intake := &fakeIntake{}
			mux := http.NewServeMux()
			RegisterIngestRoutes(mux, intake)
			request := httptest.NewRequest(http.MethodPost, path, nil)
			request.Body = explodingBody{t}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+secret)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized || intake.submitted || intake.reported {
				t.Fatalf("status %d submitted=%t reported=%t, want 401 and no later stage",
					recorder.Code, intake.submitted, intake.reported)
			}
		})
	}
}
