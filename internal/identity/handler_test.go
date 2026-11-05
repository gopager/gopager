package identity

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/commands"
)

type identityErrorBody struct {
	Code      string `json:"code"`
	Fallback  string `json:"fallback"`
	Retryable bool   `json:"retryable"`
}

func TestLoginRejectsOversizedBodyAsResourceLimit(t *testing.T) {
	const origin = "https://pager.example"
	transport, err := NewHTTPTransport(origin)
	if err != nil {
		t.Fatal(err)
	}
	// The size limit is enforced before any credential check, so no login
	// dependencies are reached.
	handler, err := NewHTTPHandler(transport, &LoginService{})
	if err != nil {
		t.Fatal(err)
	}
	prefix, suffix := `{"schemaVersion":1,"username":"user","password":"`, `"}`
	for _, test := range []struct {
		size   int
		status int
		code   string
	}{
		{16*1024 + 1, http.StatusRequestEntityTooLarge, "RESOURCE_LIMIT"},
		// A within-limit body that is not valid input remains INVALID_INPUT.
		{16 * 1024, http.StatusBadRequest, "INVALID_INPUT"},
	} {
		body := prefix + strings.Repeat("x", test.size-len(prefix)-len(suffix)) + suffix
		if test.code == "INVALID_INPUT" {
			body = prefix + strings.Repeat("x", test.size-len(prefix)-len(suffix)-1) + suffix + "x"
		}
		if len(body) != test.size {
			t.Fatalf("body length %d, want %d", len(body), test.size)
		}
		request := httptest.NewRequest(http.MethodPost, origin+"/api/v1/auth/login", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		var result identityErrorBody
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != test.status || result.Code != test.code || result.Retryable {
			t.Fatalf("%d-byte login body returned %d %+v", test.size, recorder.Code, result)
		}
	}
}

func TestIdentityErrorMapsIdempotencyConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeIdentityError(recorder, commands.ErrIdempotencyConflict)
	var result identityErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || result.Code != "IDEMPOTENCY_CONFLICT" || result.Retryable ||
		result.Fallback != "This operation identifier was already used differently." {
		t.Fatalf("idempotency conflict mapped to %d %+v", recorder.Code, result)
	}
}
