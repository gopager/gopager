package permissions_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
)

func TestDetailUsesReadFieldsAndRequiresBothActions(t *testing.T) {
	principal := uuid.NewString()
	read := authz.Grant{SubjectID: principal, SubjectKind: "principal",
		Actions: []string{"message.read"}, Scope: authz.Scope{Kind: "all"}}
	detail := authz.Grant{SubjectID: principal, SubjectKind: "principal",
		Actions: []string{"message.detail"}, Scope: authz.Scope{Kind: "all"}}
	authority := authz.Authority{PrincipalID: principal, Active: true, Revision: 1,
		Grants: []authz.Grant{read, detail},
		FieldPermissions: []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
			SubjectID: principal, SubjectKind: "principal", Fields: []string{"payload"}}}}
	fields := []string{"payload"}
	plan, err := permissions.ReadScope(authority, "message.detail", fields, permissions.Attributes{}, time.Now())
	if err != nil || plan.Predicate == nil || !slices.Equal(plan.Fields, fields) {
		t.Fatalf("read fields plus detail action failed: %v", err)
	}
	fields[0] = "address"
	if plan.Fields[0] != "payload" {
		t.Fatal("caller mutated the authorized projection fields")
	}
	for _, grants := range [][]authz.Grant{{read}, {detail}} {
		authority.Grants = grants
		if _, err := permissions.ReadScope(authority, "message.detail", []string{"payload"},
			permissions.Attributes{}, time.Now()); !errors.Is(err, authz.ErrForbidden) {
			t.Fatalf("detail/read action was optional: %v", err)
		}
	}
	authority.Grants = []authz.Grant{read, detail}
	for _, actions := range [][]string{{"message.read"}, {"message.detail"}} {
		authority.Key = &authz.Restrictions{Actions: actions, Scopes: []authz.Scope{{Kind: "all"}}, Fields: []string{"payload"}}
		if _, err := permissions.ReadScope(authority, "message.detail", []string{"payload"},
			permissions.Attributes{}, time.Now()); !errors.Is(err, authz.ErrForbidden) {
			t.Fatalf("key did not require both actions: %v", err)
		}
	}
}

func TestReadScopeRejectsMissingAuthorityAndUnknownCatalogValues(t *testing.T) {
	principal := uuid.NewString()
	grant := authz.Grant{SubjectID: principal, SubjectKind: "principal",
		Actions: []string{"message.read"}, Scope: authz.Scope{Kind: "all"}}
	valid := authz.Authority{PrincipalID: principal, Active: true, Revision: 1, Grants: []authz.Grant{grant}}
	for name, authority := range map[string]authz.Authority{
		"missing": {}, "inactive": {PrincipalID: principal, Revision: 1},
		"unversioned": {PrincipalID: principal, Active: true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := permissions.ReadScope(authority, "message.read", nil,
				permissions.Attributes{}, time.Now()); !errors.Is(err, authz.ErrForbidden) {
				t.Fatalf("invalid authority accepted: %v", err)
			}
		})
	}
	for _, test := range []struct{ action, field string }{{"*", "payload"}, {"message.read", "*"}} {
		if _, err := permissions.ReadScope(valid, test.action, []string{test.field},
			permissions.Attributes{}, time.Now()); !errors.Is(err, authz.ErrInvalidInput) {
			t.Fatalf("unknown action/field accepted: %v", err)
		}
	}
}
