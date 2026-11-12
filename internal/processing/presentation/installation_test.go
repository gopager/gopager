package presentation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// memorySettings keeps the installation settings in memory.
type memorySettings struct {
	current Installation
	layouts map[uuid.UUID]bool
}

func (m *memorySettings) ReadInstallation(context.Context, platform.TransactionContext) (Installation, error) {
	return m.current, nil
}

func (m *memorySettings) UpdateInstallation(_ context.Context, _ platform.TransactionContext, expected int64, input InstallationInput, _ time.Time) error {
	if m.current.Revision != expected {
		return authz.ErrRevisionConflict
	}
	if input.DefaultLayoutID != nil && !m.layouts[*input.DefaultLayoutID] {
		return authz.InvalidField("defaultLayoutId")
	}
	m.current = Installation{Title: input.Title, Banner: input.Banner, Severity: input.Severity,
		DefaultLayoutID: input.DefaultLayoutID, Revision: expected + 1}
	return nil
}

func installationFor(t *testing.T, kind identity.ProofKind, actions ...string) (*InstallationService, *memorySettings) {
	t.Helper()
	store := &memorySettings{current: Installation{Title: "GoPager", Revision: 1}, layouts: map[uuid.UUID]bool{}}
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true}
	if len(actions) > 0 {
		authority.Grants = []authz.Grant{{ID: uuid.NewString(), SubjectID: authority.PrincipalID, SubjectKind: "principal",
			Actions: actions, Scope: authz.Scope{Kind: "all"}}}
	}
	service, err := NewInstallationService(fakeTransactions{}, fakeResolver{identity.Authentication{Authority: authority, Kind: kind}},
		fakeClock{}, store)
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func text(value string) *string { return &value }

func TestInstallationReadNeedsNoAuthority(t *testing.T) {
	t.Parallel()
	service, _ := installationFor(t, identity.AnonymousProof)
	got, err := service.Read(context.Background())
	if err != nil || got.Title != "GoPager" || got.Revision != 1 {
		t.Fatalf("read = %+v, %v", got, err)
	}
}

func TestInstallationUpdateNeedsNoticeManage(t *testing.T) {
	t.Parallel()
	input := InstallationInput{Title: "Control room"}
	service, _ := installationFor(t, identity.SessionProof, "message.read")
	if _, err := service.Update(context.Background(), identity.Proof{}, 1, input, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("without right: %v", err)
	}
	guest, _ := installationFor(t, identity.AnonymousProof, "notice.manage")
	if _, err := guest.Update(context.Background(), identity.Proof{}, 1, input, allow); !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("guest: %v", err)
	}
	service, _ = installationFor(t, identity.SessionProof, NoticeAction)
	got, err := service.Update(context.Background(), identity.Proof{}, 1, input, allow)
	if err != nil || got.Title != "Control room" || got.Revision != 2 {
		t.Fatalf("update = %+v, %v", got, err)
	}
	if _, err := service.Update(context.Background(), identity.Proof{}, 1, input, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale: %v", err)
	}
}

func TestInstallationUpdateValidatesFields(t *testing.T) {
	t.Parallel()
	service, store := installationFor(t, identity.SessionProof, NoticeAction)
	missing := uuid.New()
	for field, input := range map[string]InstallationInput{
		"title":           {Title: "  "},
		"title ":          {Title: strings.Repeat("x", 129)},
		"banner":          {Title: "t", Banner: text(""), Severity: text("info")},
		"banner ":         {Title: "t", Banner: text(strings.Repeat("x", 2049)), Severity: text("info")},
		"severity":        {Title: "t", Banner: text("b")},
		"severity ":       {Title: "t", Banner: text("b"), Severity: text("fatal")},
		"severity  ":      {Title: "t", Severity: text("info")},
		"defaultLayoutId": {Title: "t", DefaultLayoutID: &missing},
	} {
		_, err := service.Update(context.Background(), identity.Proof{}, 1, input, allow)
		var fieldError *authz.FieldError
		if !errors.As(err, &fieldError) || fieldError.Field != strings.TrimSpace(field) {
			t.Fatalf("%q: %v", field, err)
		}
	}
	if store.current.Revision != 1 {
		t.Fatal("an invalid update changed the settings")
	}
	if _, err := service.Update(context.Background(), identity.Proof{}, 1,
		InstallationInput{Title: strings.Repeat("ä", 128), Banner: text("Alarm"), Severity: text("critical")}, allow); err != nil {
		t.Fatalf("valid: %v", err)
	}
}
