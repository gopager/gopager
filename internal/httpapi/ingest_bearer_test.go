package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/gopager/gopager/internal/identity"
)

func TestGatewayBearerRequiresOneValidAuthorizationHeader(t *testing.T) {
	t.Parallel()
	secret, _, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		headers []string
		valid   bool
	}{
		{"valid", []string{"Bearer " + secret}, true},
		{"wrong scheme", []string{"Basic " + secret}, false},
		{"multiple", []string{"Bearer " + secret, "Bearer " + secret}, false},
		{"extra secret token", []string{"Bearer " + secret + " other"}, false},
		{"missing", nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("POST", "/api/v1/transmissions", nil)
			for _, value := range test.headers {
				request.Header.Add("Authorization", value)
			}
			_, err := gatewayBearer(request)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t err=%v", test.valid, err)
			}
		})
	}
}
