package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestStatusHasNoProfileMember pins that the deployment profile is not part of
// the runtime state: Lite and Full differ only in topology (D-135).
func TestStatusHasNoProfileMember(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(Status{Live: true, Ready: true, Roles: []Role{RoleAPI}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "profile") {
		t.Fatalf("status JSON mentions profile: %s", data)
	}
}
