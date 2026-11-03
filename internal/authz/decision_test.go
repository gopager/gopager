package authz_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

func TestConcreteActionScopeUnionAndKeyIntersection(t *testing.T) {
	owner, network, otherNetwork, unit, otherUnit, collection := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 10, 27, 12, 0, 0, 123456000, time.UTC)
	hour := int64(3600)
	makeGrant := func(actions []string, kind string, references ...uuid.UUID) authz.Grant {
		grant := authz.Grant{SubjectID: owner.String(), SubjectKind: "principal", Actions: actions,
			Scope: authz.Scope{Kind: kind}}
		for _, reference := range references {
			grant.Scope.References = append(grant.Scope.References, reference.String())
		}
		return grant
	}
	read := makeGrant([]string{"message.read"}, "network", network)
	read.MaxAgeSeconds = &hour
	authority := authz.Authority{PrincipalID: owner.String(), Active: true, Revision: 1, Grants: []authz.Grant{read},
		FieldPermissions: []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
			SubjectID: owner.String(), SubjectKind: "principal", Fields: []string{"payload"}}}}
	target := authz.Target{NetworkID: network, UnitID: unit, AgeAnchor: now.Add(-time.Hour)}
	check := func(action string, fields []string, want error) {
		t.Helper()
		if err := authz.Authorize(authority, action, fields, target, now); !errors.Is(err, want) {
			t.Fatalf("authorization for %s: %v, want %v", action, err, want)
		}
	}
	check("message.read", []string{"payload"}, nil)
	target.AgeAnchor = target.AgeAnchor.Add(-time.Microsecond)
	check("message.read", []string{"payload"}, authz.ErrForbidden)
	// Eligibility can come from a different matching grant, but not a different target.
	authority.Grants = append(authority.Grants, makeGrant([]string{"message.read"}, "unit", unit))
	check("message.read", []string{"payload"}, nil)
	target.UnitID = otherUnit
	check("message.read", []string{"payload"}, authz.ErrForbidden)
	target.UnitID = unit
	target.NetworkID = otherNetwork
	check("message.read", []string{"payload"}, nil)
	target.NetworkID = network
	authority.Grants = append(authority.Grants, makeGrant([]string{"message.detail"}, "unitCollection", collection))
	check("message.detail", []string{"payload"}, authz.ErrForbidden)
	target.UnitCollections = []uuid.UUID{collection}
	check("message.detail", []string{"payload"}, nil)
	authority.Key = &authz.Restrictions{Actions: []string{"message.read", "message.detail"},
		Scopes: []authz.Scope{{Kind: "unit", References: []string{unit.String()}}},
		Fields: []string{"payload"}, MaxAgeSeconds: &hour}
	check("message.detail", []string{"payload"}, authz.ErrForbidden)
	target.AgeAnchor = now.Add(-time.Hour)
	check("message.detail", []string{"payload"}, nil)
	authority.Key.Actions = []string{"message.detail"}
	check("message.detail", []string{"payload"}, authz.ErrForbidden)
	authority.Key.Actions = []string{"message.read", "message.detail"}
	authority.Key.Fields = nil
	check("message.detail", []string{"payload"}, authz.ErrForbidden)
	authority.Key = nil
	check("message.read", []string{"address"}, authz.ErrForbidden)
	check("message.read", []string{"unknown"}, authz.ErrInvalidInput)
	check("account.*", nil, authz.ErrInvalidInput)
	authority.Active = false
	check("message.read", nil, authz.ErrForbidden)
}

func TestAccountAndWithoutUnitActionsNeedConcreteContext(t *testing.T) {
	owner, account, network := uuid.New(), uuid.New(), uuid.New()
	authority := authz.Authority{PrincipalID: owner.String(), Active: true, Revision: 1,
		Grants: []authz.Grant{{
			SubjectID: owner.String(), SubjectKind: "principal",
			Actions: []string{"account.update"}, Scope: authz.Scope{Kind: "account", References: []string{account.String()}},
		}, {
			SubjectID: owner.String(), SubjectKind: "principal",
			Actions: []string{"without-unit.read"}, Scope: authz.Scope{Kind: "network", References: []string{network.String()}},
		}}}
	now := time.Now().UTC()
	for _, test := range []struct {
		action string
		target authz.Target
		want   error
	}{
		{"account.update", authz.Target{AccountID: account}, nil},
		{"account.update", authz.Target{AccountID: owner}, authz.ErrForbidden},
		{"account.delete", authz.Target{AccountID: account}, authz.ErrForbidden},
		{"without-unit.read", authz.Target{NetworkID: network}, authz.ErrForbidden},
		{"without-unit.read", authz.Target{NetworkID: network, WithoutUnit: true}, nil},
		{"without-unit.read", authz.Target{NetworkID: network, WithoutUnit: true, UnitID: uuid.New()}, authz.ErrInvalidInput},
	} {
		if err := authz.Authorize(authority, test.action, nil, test.target, now); !errors.Is(err, test.want) {
			t.Fatalf("%s: %v, want %v", test.action, err, test.want)
		}
	}
}

func TestAgeCutoffDoesNotOverflow(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 123456000, time.UTC)
	cutoff, err := authz.AgeCutoff(math.MaxInt64, now)
	if err != nil || cutoff.Year() != 0 || cutoff.Month() != time.January || cutoff.Day() != 1 {
		t.Fatalf("overflowed age floor: %v, %v", cutoff, err)
	}
	cutoff, err = authz.AgeCutoff(1, now)
	if err != nil || !cutoff.Equal(now.Add(-time.Second)) {
		t.Fatalf("lost age precision: %v, %v", cutoff, err)
	}
	for _, invalid := range []struct {
		seconds int64
		now     time.Time
	}{{-1, now}, {0, time.Time{}}, {0, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}} {
		if _, err := authz.AgeCutoff(invalid.seconds, invalid.now); !errors.Is(err, authz.ErrInvalidInput) {
			t.Fatalf("invalid age accepted: %v", err)
		}
	}
}
