package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

func TestProcessingHTTPCodecPreservesKernelBoundaries(t *testing.T) {
	t.Parallel()
	const prefix = `{"value":"`
	const suffix = `"}`
	exact := prefix + strings.Repeat("x", processing.MaxInputBytes-len(prefix)-len(suffix)) + suffix
	for _, test := range []struct {
		name string
		body string
		code platform.ErrorCode
	}{
		{"maximum", exact, ""},
		{"too-large", exact + " ", platform.CodeResourceLimit},
		{"duplicate", `{"value":"first","value":"second"}`, platform.CodeInvalidInput},
		{"unknown", `{"unknown":true}`, platform.CodeInvalidInput},
		{"trailing", `{"value":"first"} {}`, platform.CodeInvalidInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			var input struct {
				Value string `json:"value"`
			}

			err := decodeProcessingJSON(request, &input)
			if platform.FailureCode(err) != test.code {
				t.Fatalf("code = %s, want %s: %v", platform.FailureCode(err), test.code, err)
			}
			if test.code == "" && len(input.Value) != processing.MaxInputBytes-len(prefix)-len(suffix) {
				t.Fatalf("maximum-sized input was not preserved")
			}
		})
	}
}

func TestProcessingStatusRevisionsUseLosslessDecimalStrings(t *testing.T) {
	t.Parallel()
	for _, status := range []any{
		processingmanagement.CandidateStatus{Revision: 9223372036854775807},
		processingmanagement.PrivateResultMetadata{Revision: 9223372036854775807},
	} {
		encoded, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), `"revision":"9223372036854775807"`) {
			t.Fatalf("revision lost its decimal string wire form: %s", encoded)
		}
	}
}
