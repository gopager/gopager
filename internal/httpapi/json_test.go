package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagementRejectsInvalidShapes(t *testing.T) {
	for _, body := range []string{
		`{"schemaVersion":1,"unexpected":true}`, `{} {}`, `null`,
		`{"name":"` + strings.Repeat("x", 64*1024) + `"}`,
	} {
		request := httptest.NewRequest("POST", "/api/v1/units", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		var input unitMutation
		err := decodeManagementJSON(request, &input)
		if body == "null" {
			if err != nil || input.SchemaVersion != 0 {
				t.Fatal("null input lost its invalid schema marker")
			}
		} else if err == nil {
			t.Fatal("invalid/oversized JSON accepted")
		}
	}

}
