package identity

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

func TestInternalExecutionAuthorityUsesCurrentOwnerWithoutCredentialProof(t *testing.T) {
	fixture, _ := newAuthorityFixture(t)
	principal := fixture.account.ID
	fixture.fields = []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
		SubjectID: principal.String(), SubjectKind: "principal", Fields: []string{"payload"}}}
	fixture.credential.RevokedAt = &fixture.now
	fixture.credential.Restrictions = &authz.Restrictions{}
	resolver, err := NewResolver(fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := resolver.ResolvePrincipalAuthority(t.Context(), nil, principal)
	if err != nil || authority.PrincipalID != principal.String() || authority.Revision != 7 ||
		!authority.Active || authority.Key != nil || fixture.touched ||
		authz.AuthorizeFields(authority, []string{"payload"}) != nil {
		t.Fatalf("internal owner resolution fabricated credentials or authority: %v", err)
	}
	fixture.fields = nil
	authority, err = resolver.ResolvePrincipalAuthority(t.Context(), nil, principal)
	if err != nil || !errors.Is(authz.AuthorizeFields(authority, []string{"payload"}), authz.ErrForbidden) {
		t.Fatalf("internal resolution retained removed global fields: %v", err)
	}
	fixture.exists = false
	authority, err = resolver.ResolvePrincipalAuthority(t.Context(), nil, principal)
	if err != nil || len(authority.Grants) != 0 {
		t.Fatalf("missing current scope widened execution authority: %v", err)
	}
	for _, state := range []string{"suspended", "deleting", "deleted"} {
		fixture.account.State = state
		if _, err := resolver.ResolvePrincipalAuthority(t.Context(), nil, principal); !errors.Is(err, ErrLoginRequired) {
			t.Fatalf("inactive execution Principal accepted: %v", err)
		}
	}
	fixture.account.State = "active"
	fixture.account.Kind = "technical"
	if _, err := resolver.ResolvePrincipalAuthority(t.Context(), nil, principal); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolvePrincipalAuthority(t.Context(), nil, uuid.New()); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("mismatched primary identity accepted: %v", err)
	}
	if _, err := resolver.ResolvePrincipalAuthority(t.Context(), nil, uuid.Nil); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("zero execution Principal accepted: %v", err)
	}
	fixture.fieldFailure = errors.New("field store unavailable")
	if _, err := resolver.ResolvePrincipalAuthority(t.Context(), nil, principal); !errors.Is(err, fixture.fieldFailure) {
		t.Fatalf("field store failure became empty authority: %v", err)
	}
	fixture.fieldFailure = nil
	fixture.fields = []authz.FieldPermission{{Fields: []string{"*"}}}
	if _, err := resolver.ResolvePrincipalAuthority(t.Context(), nil, principal); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("broken fields became usable internal authority: %v", err)
	}
}
