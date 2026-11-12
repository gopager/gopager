package presentation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing"
)

func metadataFor(t *testing.T, permissions []string, actions ...string) (EditorMetadata, error) {
	t.Helper()
	service, _ := serviceFor(t, identity.SessionProof, actions...)
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true}
	if len(actions) > 0 {
		authority.Grants = []authz.Grant{{ID: uuid.NewString(), SubjectID: authority.PrincipalID, SubjectKind: "principal",
			Actions: actions, Scope: authz.Scope{Kind: "all"}}}
	}
	if len(permissions) > 0 {
		authority.FieldPermissions = []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1, SubjectID: authority.PrincipalID,
			SubjectKind: "principal", Fields: permissions}}
	}
	service.resolver = fakeResolver{identity.Authentication{Authority: authority, Kind: identity.SessionProof}}
	return service.EditorMetadata(context.Background(), identity.Proof{})
}

func TestEditorMetadataFollowsTheCallersRights(t *testing.T) {
	t.Parallel()
	for _, action := range []string{ManageAction, "subscription.manage-own", "subscription.manage-global"} {
		got, err := metadataFor(t, nil, action)
		if err != nil || len(got.BlockKinds) == 0 || len(got.OutputKinds) != 3 || got.Bounds.OutputBytes != processing.MaxLayoutOutputBytes {
			t.Fatalf("%s: %+v, %v", action, got, err)
		}
	}
	if _, err := metadataFor(t, nil, "message.read"); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("message.read alone: %v", err)
	}
	if _, err := metadataFor(t, nil); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("no rights: %v", err)
	}
}

func TestEditorMetadataListsOnlyReadableDeliveryFields(t *testing.T) {
	t.Parallel()
	without, err := metadataFor(t, nil, ManageAction)
	if err != nil {
		t.Fatal(err)
	}
	with, err := metadataFor(t, []string{"payload", "address", "unit", "gateway", "quality", "derived"}, ManageAction)
	if err != nil {
		t.Fatal(err)
	}
	if len(with.Fields) <= len(without.Fields) {
		t.Fatalf("field permissions did not add fields: %d <= %d", len(with.Fields), len(without.Fields))
	}
	for _, field := range without.Fields {
		if field.Group != "" {
			t.Fatalf("field %v of group %q listed without permission", field.Ref, field.Group)
		}
	}
	for _, field := range with.Fields {
		if field.Ref.Namespace == "" || field.Label == "" {
			t.Fatalf("incomplete field %+v", field)
		}
	}
}
