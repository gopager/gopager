package authz

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestAgencyScopeReplacesNetwork expects the grant scope kinds to be all, agency, unit,
// unitCollection and account: a Radio network is not a grant scope any more, and an Agency
// grant covers targets that carry that Agency.
func TestAgencyScopeReplacesNetwork(t *testing.T) {
	t.Parallel()
	agency := uuid.New()
	if ValidScope(Scope{Kind: "network", References: []string{agency.String()}}) {
		t.Fatal("network is still a valid scope kind")
	}
	if !ValidScope(Scope{Kind: "agency", References: []string{agency.String()}}) {
		t.Fatal("agency is not a valid scope kind")
	}
	for action, want := range map[string]bool{
		"message.read": true, "without-unit.read": true, "unit.create": true, "gateway.update": true,
		"network.update": true, "address-rule.update": true, "workflow.read": true, "audit.read": true,
		"agency.read": true, "agency.update": true, "agency.create": false, "agency.delete": false,
		"unit-template.read": false,
	} {
		if got := ActionAllowsScope(action, "agency"); got != want {
			t.Fatalf("ActionAllowsScope(%q, agency)=%t want %t", action, got, want)
		}
	}
	if ActionAllowsScope("message.read", "network") {
		t.Fatal("message.read still allows a network scope")
	}
	if !ActionAllowsScope("agency.create", "all") || !ActionAllowsScope("agency.delete", "all") {
		t.Fatal("agency management is not grantable on all")
	}
}

// TestAuthorizeMatchesAgencyTargets expects an Agency grant to authorize targets of that
// Agency, including Unit targets whose Unit belongs to it and unassigned traffic of its
// Radio networks, and to reject other Agencies.
func TestAuthorizeMatchesAgencyTargets(t *testing.T) {
	t.Parallel()
	principal, agency, other := uuid.New(), uuid.New(), uuid.New()
	authority := Authority{PrincipalID: principal.String(), Revision: 1, Active: true,
		Grants: []Grant{{ID: uuid.NewString(), SubjectID: principal.String(), SubjectKind: "principal",
			Actions: []string{"message.read", "without-unit.read", "unit.read"},
			Scope:   Scope{Kind: "agency", References: []string{agency.String()}}}}}
	now := time.Now()
	cases := []struct {
		name   string
		action string
		target Target
		want   error
	}{
		{"unit of the agency", "message.read", Target{AgencyID: agency, UnitID: uuid.New()}, nil},
		{"unassigned traffic of the agency", "without-unit.read", Target{AgencyID: agency, WithoutUnit: true}, nil},
		{"unit management in the agency", "unit.read", Target{AgencyID: agency, UnitID: uuid.New()}, nil},
		{"other agency", "message.read", Target{AgencyID: other, UnitID: uuid.New()}, ErrForbidden},
		{"no agency on the target", "message.read", Target{UnitID: uuid.New()}, ErrForbidden},
	}
	for _, c := range cases {
		err := Authorize(authority, c.action, nil, c.target, now)
		if !errors.Is(err, c.want) && (c.want != nil || err != nil) {
			t.Fatalf("%s: Authorize=%v want %v", c.name, err, c.want)
		}
	}
}
