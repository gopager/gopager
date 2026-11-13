package presentation

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

func TestRequireManageRefusesAnEmptyAgencySet(t *testing.T) {
	t.Parallel()
	agency := uuid.New()
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true,
		Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: uuid.NewString(), SubjectKind: "principal",
			Actions: []string{ManageAction}, Scope: authz.Scope{Kind: "agency", References: []string{agency.String()}}}}}
	now := time.Now()
	if err := requireManage(authority, now, []uuid.UUID{agency}); err != nil {
		t.Fatalf("managed Agency: %v", err)
	}
	for name, sets := range map[string][][]uuid.UUID{
		"empty":            {{}},
		"nil":              {nil},
		"no sets":          {},
		"managed and none": {{agency}, {}},
	} {
		if err := requireManage(authority, now, sets...); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("%s: err = %v, want authz.ErrForbidden", name, err)
		}
	}
}
