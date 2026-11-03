package authz_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
)

// fieldAuthority creates explicit global fields and independent scoped data rights.
func fieldAuthority() (authz.Authority, uuid.UUID, uuid.UUID) {
	principal, first, second := uuid.NewString(), uuid.New(), uuid.New()
	return authz.Authority{PrincipalID: principal, Active: true, Revision: 1,
		Grants: []authz.Grant{{SubjectID: principal, SubjectKind: "principal",
			Actions: []string{"message.read", "message.detail"},
			Scope:   authz.Scope{Kind: "unit", References: []string{first.String(), second.String()}}}},
		FieldPermissions: []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
			SubjectID: principal, SubjectKind: "principal", Fields: []string{"address"}, Delegable: true},
			{ID: uuid.NewString(), Revision: 1, SubjectID: uuid.NewString(), SubjectKind: "userGroup",
				Fields: []string{"payload"}}}}, first, second
}

func TestGlobalFieldsDoNotDependOnUnitAndDoNotGrantData(t *testing.T) {
	authority, first, second := fieldAuthority()
	now := time.Now().UTC()
	for _, unit := range []uuid.UUID{first, second} {
		if err := authz.Authorize(authority, "message.detail", []string{"address", "payload"},
			authz.Target{UnitID: unit}, now); err != nil {
			t.Fatalf("global groups were scoped to one Unit: %v", err)
		}
	}
	if err := authz.Authorize(authority, "message.read", []string{"address"},
		authz.Target{UnitID: uuid.New()}, now); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("global fields exposed an unauthorized Unit: %v", err)
	}
	authority.Grants = nil
	if err := authz.Authorize(authority, "message.read", []string{"address"},
		authz.Target{UnitID: first}, now); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("field permission granted rows: %v", err)
	}
}

func TestGlobalFieldRemovalAndKeyRestrictionApplyToPointAndSQL(t *testing.T) {
	authority, first, _ := fieldAuthority()
	authority.FieldPermissions = authority.FieldPermissions[1:]
	now := time.Now().UTC()
	if err := authz.Authorize(authority, "message.read", []string{"address"},
		authz.Target{UnitID: first}, now); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("removed global field still permitted: %v", err)
	}
	if _, err := permissions.ReadScope(authority, "message.read", []string{"address"},
		permissions.Attributes{Unit: "unit_id"}, now); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("SQL planner bypassed missing global field: %v", err)
	}
	authority.Key = &authz.Restrictions{Actions: []string{"message.read"},
		Scopes: []authz.Scope{{Kind: "all"}}, Fields: []string{"address"}}
	if err := authz.AuthorizeFields(authority, []string{"address"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("key granted a field the owner lacks: %v", err)
	}
	if err := authz.AuthorizeFields(authority, []string{"payload"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("owner bypassed restrictive key fields: %v", err)
	}
}

func TestFieldPermissionValidationAndIndependentDelegation(t *testing.T) {
	authority, _, _ := fieldAuthority()
	valid := authority.FieldPermissions[0]
	for _, invalid := range []authz.FieldPermission{
		{}, {ID: valid.ID, Revision: 1, SubjectID: valid.SubjectID, SubjectKind: "principal"},
		{ID: valid.ID, Revision: 0, SubjectID: valid.SubjectID, SubjectKind: "principal", Fields: valid.Fields},
		{ID: valid.ID, Revision: 1, SubjectID: valid.SubjectID, SubjectKind: "principal", Fields: []string{"*"}},
	} {
		if err := authz.ValidateFieldPermission(invalid); !errors.Is(err, authz.ErrInvalidInput) {
			t.Fatalf("invalid global permission accepted: %v", err)
		}
	}
	if err := authz.AuthorizeFieldDelegation(authority, []string{"address"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("data rights implied permission management: %v", err)
	}
	authority.Grants = append(authority.Grants, authz.Grant{SubjectID: authority.PrincipalID,
		SubjectKind: "principal", Actions: []string{"grant.manage"}, Scope: authz.Scope{Kind: "all"}})
	if err := authz.AuthorizeFieldDelegation(authority, []string{"address"}); err != nil {
		t.Fatalf("explicit global manager/delegable group failed: %v", err)
	}
	if err := authz.AuthorizeFieldDelegation(authority, []string{"payload"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("nondelegable field was delegated: %v", err)
	}
	authority.Grants[1].Scope = authz.Scope{Kind: "account", References: []string{authority.PrincipalID}}
	if err := authz.AuthorizeFieldDelegation(authority, []string{"address"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("scoped manager minted global field rights: %v", err)
	}
}
