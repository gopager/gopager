package authz

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func grantFor(principal uuid.UUID, scope Scope, actions ...string) Grant {
	return Grant{ID: uuid.NewString(), SubjectID: principal.String(), SubjectKind: "principal",
		Actions: actions, Scope: scope}
}

// TestGatewayScopeIsLimitedToDelegationActions expects the gateway scope kind to be valid
// only for the two single-Gateway delegation actions, never for administration.
func TestGatewayScopeIsLimitedToDelegationActions(t *testing.T) {
	t.Parallel()
	for action, want := range map[string]bool{
		"gateway.diagnostics.read": true, "gateway.credential.rotate": true,
		"gateway.read": false, "gateway.update": false, "gateway.create": false, "gateway.delete": false,
		"network.read": false, "message.read": false,
	} {
		if got := ActionAllowsScope(action, "gateway"); got != want {
			t.Fatalf("ActionAllowsScope(%q, gateway)=%t want %t", action, got, want)
		}
	}
	for _, kind := range []string{"all", "agency"} {
		if !ActionAllowsScope("gateway.diagnostics.read", kind) || !ActionAllowsScope("gateway.credential.rotate", kind) {
			t.Fatalf("delegation actions do not accept %s", kind)
		}
	}
	if ValidScope(Scope{Kind: "gateway"}) {
		t.Fatal("gateway scope without references is valid")
	}
}

// TestGatewayGrantMatchesOnlyItsGateway expects a gateway-scoped grant to authorize exactly
// its Gateway and only its own action (diagnostics do not imply rotation and vice versa).
func TestGatewayGrantMatchesOnlyItsGateway(t *testing.T) {
	t.Parallel()
	principal, g1, g2, agency := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	authority := Authority{PrincipalID: principal.String(), Revision: 1, Active: true,
		Grants: []Grant{grantFor(principal, Scope{Kind: "gateway", References: []string{g1.String()}},
			"gateway.diagnostics.read")}}
	now := time.Now()
	if err := Authorize(authority, "gateway.diagnostics.read", nil, Target{AgencyID: agency, GatewayID: g1}, now); err != nil {
		t.Fatalf("own Gateway: %v", err)
	}
	for name, check := range map[string]func() error{
		"other gateway": func() error {
			return Authorize(authority, "gateway.diagnostics.read", nil, Target{AgencyID: agency, GatewayID: g2}, now)
		},
		"rotation": func() error {
			return Authorize(authority, "gateway.credential.rotate", nil, Target{GatewayID: g1}, now)
		},
		"administration": func() error { return Authorize(authority, "gateway.read", nil, Target{GatewayID: g1}, now) },
		"messages":       func() error { return Authorize(authority, "message.read", nil, Target{AgencyID: agency}, now) },
	} {
		if err := check(); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: %v, want forbidden", name, err)
		}
	}
}

// TestReachableAgencies expects the owner's all/agency grants for one action, intersected
// with a restricting key, and the Authorize failure classes.
func TestReachableAgencies(t *testing.T) {
	t.Parallel()
	principal, a, b, c := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	agencies := func(ids ...uuid.UUID) Scope {
		scope := Scope{Kind: "agency"}
		for _, id := range ids {
			scope.References = append(scope.References, id.String())
		}
		return scope
	}
	now := time.Now()
	owner := Authority{PrincipalID: principal.String(), Revision: 1, Active: true, Grants: []Grant{
		grantFor(principal, agencies(a, b), "network.read"),
		grantFor(principal, agencies(c), "network.update"),
		grantFor(principal, Scope{Kind: "gateway", References: []string{uuid.NewString()}}, "gateway.diagnostics.read"),
	}}
	reach, err := ReachableAgencies(owner, "network.read", now)
	if err != nil || reach.All || len(reach.Agencies) != 2 || !reach.Contains(a) || !reach.Contains(b) || reach.Contains(c) {
		t.Fatalf("owner reach %+v, %v", reach, err)
	}
	if reach, err := ReachableAgencies(owner, "gateway.diagnostics.read", now); err != nil || reach.All || len(reach.Agencies) != 0 {
		t.Fatalf("gateway scope widened a list: %+v, %v", reach, err)
	}
	if _, err := ReachableAgencies(owner, "gateway.read", now); !errors.Is(err, ErrForbidden) {
		t.Fatalf("no grant: %v", err)
	}
	all := owner
	all.Grants = append(slices.Clone(owner.Grants), grantFor(principal, Scope{Kind: "all"}, "network.read"))
	if reach, err := ReachableAgencies(all, "network.read", now); err != nil || !reach.All || !reach.Contains(uuid.New()) {
		t.Fatalf("all reach %+v, %v", reach, err)
	}
	keyed := all
	keyed.Key = &Restrictions{Actions: []string{"network.read"}, Scopes: []Scope{agencies(b, c)}}
	if reach, err := ReachableAgencies(keyed, "network.read", now); err != nil || reach.All ||
		len(reach.Agencies) != 2 || !reach.Contains(b) || !reach.Contains(c) {
		t.Fatalf("all owner with key %+v, %v", reach, err)
	}
	keyed = owner
	keyed.Key = &Restrictions{Actions: []string{"network.read"}, Scopes: []Scope{agencies(b, c)}}
	if reach, err := ReachableAgencies(keyed, "network.read", now); err != nil || len(reach.Agencies) != 1 || !reach.Contains(b) {
		t.Fatalf("key intersection %+v, %v", reach, err)
	}
	keyed.Key = &Restrictions{Actions: []string{"network.update"}, Scopes: []Scope{{Kind: "all"}}}
	if _, err := ReachableAgencies(keyed, "network.read", now); !errors.Is(err, ErrForbidden) {
		t.Fatalf("key without action: %v", err)
	}
	inactive := owner
	inactive.Active = false
	if _, err := ReachableAgencies(inactive, "network.read", now); !errors.Is(err, ErrForbidden) {
		t.Fatalf("inactive: %v", err)
	}
	aged := owner
	age := int64(60)
	aged.Grants = []Grant{grantFor(principal, agencies(a), "network.read")}
	aged.Grants[0].MaxAgeSeconds = &age
	if _, err := ReachableAgencies(aged, "network.read", now); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("aged grant: %v", err)
	}
}
