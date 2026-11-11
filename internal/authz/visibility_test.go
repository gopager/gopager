package authz

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func visibilityAuthority(grants ...Grant) Authority {
	return Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true, Grants: grants}
}

func visibilityGrant(scope Scope, actions ...string) Grant {
	return Grant{ID: uuid.NewString(), SubjectID: uuid.NewString(), SubjectKind: "principal", Actions: actions, Scope: scope}
}

func TestAttributeSchemaManageScopes(t *testing.T) {
	t.Parallel()
	if !ActionAllowsScope("attribute-schema.manage", "all") || !ActionAllowsScope("attribute-schema.manage", "agency") ||
		ActionAllowsScope("attribute-schema.manage", "unit") {
		t.Fatal("attribute-schema.manage must accept the all and agency scopes only")
	}
}

func TestVisibleAgenciesByScopeKind(t *testing.T) {
	t.Parallel()
	agency, unit, collection := uuid.New(), uuid.New(), uuid.New()
	actions := []string{"message.read", "presentation.manage"}
	tests := []struct {
		name    string
		scope   Scope
		account bool
		check   func(AgencyVisibility) bool
	}{
		{"all", Scope{Kind: "all"}, false, func(v AgencyVisibility) bool { return v.Unrestricted() }},
		{"agency", Scope{Kind: "agency", References: []string{agency.String()}}, false,
			func(v AgencyVisibility) bool { return len(v.Terms) == 1 && v.Terms[0].Grant.Agencies[0] == agency }},
		{"unit", Scope{Kind: "unit", References: []string{unit.String()}}, false,
			func(v AgencyVisibility) bool { return len(v.Terms) == 1 && v.Terms[0].Grant.Units[0] == unit }},
		{"collection", Scope{Kind: "unitCollection", References: []string{collection.String()}}, false,
			func(v AgencyVisibility) bool {
				return len(v.Terms) == 1 && v.Terms[0].Grant.Collections[0] == collection
			}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := VisibleAgencies(visibilityAuthority(visibilityGrant(test.scope, "message.read")), actions, test.account)
			if err != nil || !test.check(got) {
				t.Fatalf("visibility = %+v, %v", got, err)
			}
		})
	}
}

func TestVisibleAgenciesAccountScope(t *testing.T) {
	t.Parallel()
	authority := visibilityAuthority(visibilityGrant(Scope{Kind: "account", References: []string{uuid.NewString()}}, "delivery.read"))
	if _, err := VisibleAgencies(authority, []string{"delivery.read"}, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("account scope without account mapping: %v", err)
	}
	got, err := VisibleAgencies(authority, []string{"delivery.read"}, true)
	if err != nil || !got.Unrestricted() {
		t.Fatalf("account scope with account mapping = %+v, %v", got, err)
	}
}

func TestVisibleAgenciesKeyRestrictsAndErrors(t *testing.T) {
	t.Parallel()
	agency, other := uuid.New(), uuid.New()
	authority := visibilityAuthority(visibilityGrant(Scope{Kind: "all"}, "message.read", "workflow.read"))
	authority.Key = &Restrictions{Actions: []string{"message.read"},
		Scopes: []Scope{{Kind: "agency", References: []string{agency.String()}}}}
	got, err := VisibleAgencies(authority, []string{"message.read", "workflow.read"}, false)
	if err != nil || got.Unrestricted() || len(got.Terms) != 1 || got.Terms[0].Key.Agencies[0] != agency {
		t.Fatalf("key restricted visibility = %+v, %v", got, err)
	}
	if _, err := VisibleAgencies(authority, []string{"workflow.read"}, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("action outside the key: %v", err)
	}
	if _, err := VisibleAgencies(visibilityAuthority(visibilityGrant(Scope{Kind: "agency", References: []string{other.String()}}, "message.read")),
		[]string{"unit.update"}, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("no grant: %v", err)
	}
	if _, err := VisibleAgencies(authority, []string{"no.such"}, false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown action: %v", err)
	}
	if _, err := VisibleAgencies(Authority{}, []string{"message.read"}, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("inactive authority: %v", err)
	}
}
