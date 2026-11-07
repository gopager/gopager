package borrowed_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/ingest"
)

// TestGatewayRemapNeedsAuthorityOverUnmappedTransmissionAgency covers F-102: a Transmission an
// earlier replacement left unresolved on Agency A's no longer mapped Network can only be moved by a
// caller who also holds gateway.update on Agency A, even though the current and the new mapping
// name only Agency B's Network.
func TestGatewayRemapNeedsAuthorityOverUnmappedTransmissionAgency(t *testing.T) {
	t.Parallel()
	f := newRematchFixture(t)
	receipt := f.submit(t, "orphaned", "default", 100)
	f.replace(t, mapping("b", f.networks[1], 0))
	drainCorrections(t, f.runner)
	if got := f.state(t, receipt.TransmissionID); got.network != f.networks[0] || !got.unresolved {
		t.Fatalf("setup: %+v", got)
	}
	grant := func(agencies ...uuid.UUID) authz.Grant {
		references := make([]string, len(agencies))
		for i, agency := range agencies {
			references[i] = agency.String()
		}
		return authz.Grant{Actions: []string{"gateway.read", "gateway.update"},
			Scope: authz.Scope{Kind: "agency", References: references}}
	}
	var revision int64
	f.queryRow(t, `SELECT revision FROM gateways WHERE id=$1`, []any{f.gateway}, &revision)
	replaceManagementGrants(t, f.matcherFixture, []authz.Grant{grant(f.agencies[1])})
	if _, err := f.admin.ConfigureNetworks(t.Context(), f.proof, f.gateway, revision,
		[]ingest.GatewayNetworkInput{mapping("default", f.networks[1], 0)}, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("replacement with Agency B only = %v, want forbidden", err)
	}
	if got := f.state(t, receipt.TransmissionID); got.network != f.networks[0] {
		t.Fatalf("a forbidden replacement moved the Transmission: %+v", got)
	}
	replaceManagementGrants(t, f.matcherFixture, []authz.Grant{grant(f.agencies[0], f.agencies[1])})
	result, err := f.admin.ConfigureNetworks(t.Context(), f.proof, f.gateway, revision,
		[]ingest.GatewayNetworkInput{mapping("default", f.networks[1], 0)}, checkedMutation)
	if err != nil || len(result.Corrections) != 1 {
		t.Fatalf("replacement with Agencies A and B = %+v, %v", result, err)
	}
	drainCorrections(t, f.runner)
	if got := f.state(t, receipt.TransmissionID); got.network != f.networks[1] || got.unresolved {
		t.Fatalf("not moved after the authorized replacement: %+v", got)
	}
}
