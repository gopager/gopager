package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
)

// TestDeliveryErrorsCarryTheirParametersAndCodeRetryability proves that the Delivery errors keep
// the retryability of their code and add their documented parameters.
func TestDeliveryErrorsCarryTheirParametersAndCodeRetryability(t *testing.T) {
	t.Parallel()
	instance, deliveryID := uuid.New(), uuid.New()
	for _, test := range []struct {
		err        error
		status     int
		code       string
		retryable  bool
		parameters map[string]any
	}{
		{&delivery.TargetUnavailableError{InstanceID: instance}, http.StatusServiceUnavailable, "AUTHORITY_UNAVAILABLE", true,
			map[string]any{"status": "unavailable", "instanceId": instance.String()}},
		{&delivery.DependencyError{Dependency: "input-ceiling"}, http.StatusConflict, "DEPENDENCY_UNAVAILABLE", false,
			map[string]any{"dependency": "input-ceiling"}},
		{&delivery.UncertainError{DeliveryID: deliveryID}, http.StatusConflict, "EXTERNAL_UNCERTAIN", false,
			map[string]any{"deliveryId": deliveryID.String()}},
	} {
		recorder := httptest.NewRecorder()
		writeManagementError(recorder, test.err)
		var result struct {
			Code       string         `json:"code"`
			Retryable  bool           `json:"retryable"`
			Parameters map[string]any `json:"parameters"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != test.status || result.Code != test.code || result.Retryable != test.retryable ||
			len(result.Parameters) != len(test.parameters) {
			t.Fatalf("%s: %d %+v", test.code, recorder.Code, result)
		}
		for name, value := range test.parameters {
			if result.Parameters[name] != value {
				t.Fatalf("%s: parameter %s = %v", test.code, name, result.Parameters[name])
			}
		}
	}
}
