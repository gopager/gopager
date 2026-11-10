package authz

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeliveryAccessMatchesRecordedContext(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	subscriber, agency, unit, collection, other := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	hour := int64(3600)
	authority := func(grants ...Grant) Authority {
		return Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true, Grants: grants}
	}
	grant := func(kind string, maxAge *int64, refs ...uuid.UUID) Grant {
		scope := Scope{Kind: kind}
		for _, ref := range refs {
			scope.References = append(scope.References, ref.String())
		}
		return Grant{Actions: []string{"delivery.read"}, Scope: scope, MaxAgeSeconds: maxAge}
	}
	target := DeliveryTarget{SubscriberID: subscriber, Agencies: []uuid.UUID{agency}, Units: []uuid.UUID{unit},
		UnitCollections: []uuid.UUID{collection}, AgeAnchor: now.Add(-30 * time.Minute)}
	cases := []struct {
		name  string
		grant Grant
		want  bool
	}{
		{"all", grant("all", nil), true},
		{"own account", grant("account", nil, subscriber), true},
		{"other account", grant("account", nil, other), false},
		{"agency", grant("agency", nil, agency), true},
		{"other agency", grant("agency", nil, other), false},
		{"unit", grant("unit", nil, unit), true},
		{"collection", grant("unitCollection", nil, collection), true},
		{"age within", grant("all", &hour), true},
	}
	for _, tc := range cases {
		access, err := NewDeliveryAccess(authority(tc.grant), "delivery.read", now)
		if err != nil {
			t.Fatal(err)
		}
		if got := access.AllowsDelivery(target); got != tc.want {
			t.Fatalf("%s: got %v", tc.name, got)
		}
	}
	old := target
	old.AgeAnchor = now.Add(-2 * time.Hour)
	access, err := NewDeliveryAccess(authority(grant("all", &hour)), "delivery.read", now)
	if err != nil {
		t.Fatal(err)
	}
	if access.AllowsDelivery(old) {
		t.Fatal("grant age must apply to the Delivery age anchor")
	}
	if _, err := NewDeliveryAccess(authority(), "message.read", now); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("non-delivery action: %v", err)
	}
	none, err := NewDeliveryAccess(authority(grant("all", nil)), "delivery.retry", now)
	if err != nil || none.Granted() || none.AllowsDelivery(target) {
		t.Fatalf("another action must not grant retry: %v", err)
	}
}
