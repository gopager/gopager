package permissions

import (
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

// TestReadScopeBindsAgencyAttribute expects an Agency grant to become a predicate on the
// consumer's Agency column.
func TestReadScopeBindsAgencyAttribute(t *testing.T) {
	t.Parallel()
	principal, agency := uuid.New(), uuid.New()
	authority := authz.Authority{PrincipalID: principal.String(), Revision: 1, Active: true,
		Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: principal.String(), SubjectKind: "principal",
			Actions: []string{"unit.read"}, Scope: authz.Scope{Kind: "agency", References: []string{agency.String()}}}}}
	plan, err := ReadScope(authority, "unit.read", nil, Attributes{Agency: "agency_id"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	want := entql.And(entql.FieldIn("agency_id", agency), entql.FieldNotNil("id")).String()
	if plan.Predicate.String() != want {
		t.Fatalf("predicate=%s want %s", plan.Predicate.String(), want)
	}
}
