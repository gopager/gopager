package ingest

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
)

// TestConfigureNetworksRejectsInvalidPriorityLists checks that the priorities of a request must be
// exactly 0..n-1 before any transaction starts, so a zero-value service is enough.
func TestConfigureNetworksRejectsInvalidPriorityLists(t *testing.T) {
	t.Parallel()
	entry := func(identifier string, priority int) GatewayNetworkInput {
		return GatewayNetworkInput{Identifier: identifier, NetworkID: uuid.New(), Priority: priority}
	}
	tests := []struct {
		name  string
		input []GatewayNetworkInput
	}{
		{"duplicate priority", []GatewayNetworkInput{entry("a", 0), entry("b", 0)}},
		{"gap", []GatewayNetworkInput{entry("a", 0), entry("b", 2)}},
		{"not starting at zero", []GatewayNetworkInput{entry("a", 1), entry("b", 2)}},
		{"negative", []GatewayNetworkInput{entry("a", -1), entry("b", 0)}},
		{"single entry above zero", []GatewayNetworkInput{entry("a", 1)}},
	}
	check := func(identity.Authentication) error { return nil }
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := (&GatewayAdministration{}).ConfigureNetworks(t.Context(), identity.Proof{},
				uuid.New(), 1, test.input, check)
			if !errors.Is(err, authz.ErrInvalidInput) {
				t.Fatalf("err=%v want authz.ErrInvalidInput", err)
			}
		})
	}
}

func TestValidPrioritiesAcceptsAnyOrderOfTheContiguousSet(t *testing.T) {
	t.Parallel()
	input := []GatewayNetworkInput{{Priority: 2}, {Priority: 0}, {Priority: 1}}
	if !validPriorities(input) {
		t.Fatal("0..n-1 in any request order must be valid")
	}
}
