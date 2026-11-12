package presentation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

type fakeGuard struct{ referenced map[uuid.UUID]bool }

func (g fakeGuard) LayoutReferenced(_ context.Context, _ platform.TransactionContext, id uuid.UUID) (bool, error) {
	return g.referenced[id], nil
}

func TestDeleteRefusesAReferencedLayout(t *testing.T) {
	t.Parallel()
	service, _ := serviceFor(t, "session", ManageAction)
	created, err := service.Create(context.Background(), identity.Proof{}, uuid.New(),
		LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: agencies()}, allow)
	if err != nil {
		t.Fatal(err)
	}
	service.guard = fakeGuard{referenced: map[uuid.UUID]bool{created.ID: true}}
	if _, err := service.Delete(context.Background(), identity.Proof{}, created.ID, created.Revision, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("referenced delete: %v", err)
	}
	service.guard = fakeGuard{}
	if _, err := service.Delete(context.Background(), identity.Proof{}, created.ID, created.Revision, allow); err != nil {
		t.Fatalf("unreferenced delete: %v", err)
	}
}
