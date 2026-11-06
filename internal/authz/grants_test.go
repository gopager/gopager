package authz_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

func TestExplicitActionScopeCatalog(t *testing.T) {
	for _, action := range authz.Actions() {
		if !authz.ValidAction(action) {
			t.Fatalf("enumerated action %q is invalid", action)
		}
	}
	for _, action := range []string{"*", "account.*", "plugin.install", "unknown.read", "rule.preview/activate/reprocess"} {
		if authz.ValidAction(action) {
			t.Fatalf("non-grantable action %q accepted", action)
		}
	}
	actions := authz.Actions()
	actions[0] = "corrupted"
	if slices.Contains(authz.Actions(), "corrupted") {
		t.Fatal("caller changed the central action catalog")
	}
	if authz.ActionAllowsScope("grouping.configure", "agency") ||
		!authz.ActionAllowsScope("grouping.configure", "all") ||
		authz.ActionAllowsScope("without-unit.read", "unit") ||
		authz.ActionAllowsScope("unit-template.read", "unit") ||
		!authz.ActionAllowsScope("unit-template.read", "all") ||
		!authz.ActionAllowsScope("assignment-correction.retry", "unitCollection") {
		t.Fatal("action-specific scope restrictions were lost")
	}
}

func TestGrantValidationRejectsMalformedOrImplicitAuthority(t *testing.T) {
	valid := authz.Grant{SubjectID: uuid.NewString(), SubjectKind: "principal",
		Actions: []string{"account.read"}, Scope: authz.Scope{Kind: "account", References: []string{uuid.NewString()}}}
	if err := authz.ValidateGrant(valid); err != nil {
		t.Fatal(err)
	}
	negative := int64(-1)
	tests := map[string]func(*authz.Grant){
		"unknown subject":     func(g *authz.Grant) { g.SubjectKind = "administrator" },
		"nonidentity subject": func(g *authz.Grant) { g.SubjectID = "admin" },
		"empty actions":       func(g *authz.Grant) { g.Actions = nil },
		"wildcard action":     func(g *authz.Grant) { g.Actions = []string{"*"} },
		"implicit all":        func(g *authz.Grant) { g.Scope = authz.Scope{} },
		"empty restriction":   func(g *authz.Grant) { g.Scope.References = nil },
		"all with references": func(g *authz.Grant) { g.Scope.Kind = "all" },
		"unknown reference":   func(g *authz.Grant) { g.Scope.References = []string{"someone"} },
		"wrong action scope":  func(g *authz.Grant) { g.Actions = []string{"gateway.update"} },
		"negative age":        func(g *authz.Grant) { g.MaxAgeSeconds = &negative },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			grant := valid
			change(&grant)
			if err := authz.ValidateGrant(grant); !errors.Is(err, authz.ErrInvalidInput) {
				t.Fatalf("validation = %v", err)
			}
		})
	}
}
