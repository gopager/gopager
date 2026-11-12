package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
)

func TestManagementErrorFallbacksAndRetryability(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		err       error
		status    int
		code      string
		fallback  string
		retryable bool
	}{
		{authz.ErrNotFound, http.StatusNotFound, "NOT_FOUND", "The requested item is not available.", false},
		{commands.ErrIdempotencyConflict, http.StatusConflict, "IDEMPOTENCY_CONFLICT",
			"This operation identifier was already used differently.", false},
		{ingest.ErrSubmissionConflict, http.StatusConflict, "IDEMPOTENCY_CONFLICT",
			"This operation identifier was already used differently.", false},
		{authz.ErrRevisionConflict, http.StatusConflict, "REVISION_CONFLICT",
			"This item changed. Reload before saving.", true},
	} {
		recorder := httptest.NewRecorder()
		writeManagementError(recorder, test.err)
		var result struct {
			Code      string `json:"code"`
			TextKey   string `json:"textKey"`
			Fallback  string `json:"fallback"`
			Retryable bool   `json:"retryable"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != test.status || result.Code != test.code || result.TextKey != "error."+test.code ||
			result.Fallback != test.fallback || result.Retryable != test.retryable {
			t.Fatalf("%v mapped to %d %+v", test.err, recorder.Code, result)
		}
	}
}

func TestManagementErrorMapping(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{authz.ErrForbidden, http.StatusForbidden, "FORBIDDEN"},
		{authz.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{authz.ErrRevisionConflict, http.StatusConflict, "REVISION_CONFLICT"},
		{commands.ErrIdempotencyConflict, http.StatusConflict, "IDEMPOTENCY_CONFLICT"},
		{platform.NewFailure(platform.CodeResourceLimit, errors.New("private infrastructure detail")), http.StatusRequestEntityTooLarge, "RESOURCE_LIMIT"},
	} {
		recorder := httptest.NewRecorder()
		actual := test.err
		if test.code != "RESOURCE_LIMIT" {
			actual = platform.NewFailure(platform.CodeStorageUnavailable, actual)
		}
		writeManagementError(recorder, actual)
		var result struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != test.status || result.Code != test.code {
			t.Fatalf("domain cause hidden by storage wrapper: %d", recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "private infrastructure detail") {
			t.Fatal("private wrapped error escaped HTTP classification")
		}
	}
}

// TestInvalidFieldNamesTheFieldInParameters expects a field-level validation failure to be an
// ordinary INVALID_INPUT that names the offending request field.
func TestInvalidFieldNamesTheFieldInParameters(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	writeManagementError(recorder, authz.InvalidField("timeZone"))
	var result struct {
		Code       string         `json:"code"`
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || result.Code != "INVALID_INPUT" || result.Parameters["field"] != "timeZone" {
		t.Fatalf("mapped to %d %+v", recorder.Code, result)
	}
	if !errors.Is(authz.InvalidField("x"), authz.ErrInvalidInput) {
		t.Fatal("a field error does not match ErrInvalidInput")
	}
}
