package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// memoryPreferences keeps preferences in memory.
type memoryPreferences struct{ rows map[uuid.UUID]Preferences }

func (m *memoryPreferences) ReadPreferences(_ context.Context, _ platform.TransactionContext, account uuid.UUID) (Preferences, bool, error) {
	row, ok := m.rows[account]
	return row, ok, nil
}

func (m *memoryPreferences) SavePreferences(_ context.Context, _ platform.TransactionContext, account uuid.UUID,
	expected int64, input PreferencesInput, _ time.Time) error {
	if m.rows[account].Revision != expected {
		return authz.ErrRevisionConflict
	}
	m.rows[account] = Preferences{Locale: input.Locale, TimeZone: input.TimeZone, TimeGapMinutes: input.TimeGapMinutes, Revision: expected + 1}
	return nil
}

func preferencesFor(t *testing.T, kind ProofKind, actions ...string) (*PreferenceService, *memoryPreferences) {
	t.Helper()
	var inside bool
	var began int
	store := &memoryPreferences{rows: map[uuid.UUID]Preferences{}}
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true}
	if len(actions) > 0 {
		authority.Grants = []authz.Grant{{ID: uuid.NewString(), SubjectID: authority.PrincipalID, SubjectKind: "principal",
			Actions: actions, Scope: authz.Scope{Kind: "all"}}}
	}
	service, err := NewPreferenceService(trackedTransactions{&inside, &began},
		&fakeAuthorityResolver{inside: &inside, authentication: Authentication{Authority: authority, Kind: kind}},
		fixedClock{time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)}, store)
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func allowAll(Authentication) error { return nil }

func TestPreferencesDefaultUntilSaved(t *testing.T) {
	t.Parallel()
	service, _ := preferencesFor(t, SessionProof, PreferenceAction)
	got, err := service.Read(t.Context(), Proof{})
	if err != nil || got != (Preferences{Locale: "en", TimeZone: "UTC", TimeGapMinutes: 0, Revision: 0}) {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	saved, err := service.Save(t.Context(), Proof{}, 0, PreferencesInput{Locale: "de-AT", TimeZone: "Europe/Vienna", TimeGapMinutes: 15}, allowAll)
	if err != nil || saved.Revision != 1 || saved.Locale != "de-AT" || saved.TimeGapMinutes != 15 {
		t.Fatalf("save = %+v, %v", saved, err)
	}
	if got, err = service.Read(t.Context(), Proof{}); err != nil || got != saved {
		t.Fatalf("read = %+v, %v", got, err)
	}
	if _, err := service.Save(t.Context(), Proof{}, 0, PreferencesInput{Locale: "en", TimeZone: "UTC"}, allowAll); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("second first-save: %v", err)
	}
	if _, err := service.Save(t.Context(), Proof{}, 1, PreferencesInput{Locale: "en", TimeZone: "UTC"}, allowAll); err != nil {
		t.Fatalf("replace: %v", err)
	}
}

func TestPreferencesAreOwnedBySessionsOnly(t *testing.T) {
	t.Parallel()
	input := PreferencesInput{Locale: "en", TimeZone: "UTC"}
	key, _ := preferencesFor(t, AccessKeyProof, PreferenceAction)
	if _, err := key.Read(t.Context(), Proof{}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("key read: %v", err)
	}
	if _, err := key.Save(t.Context(), Proof{}, 0, input, allowAll); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("key save: %v", err)
	}
	guest, _ := preferencesFor(t, AnonymousProof, PreferenceAction)
	if _, err := guest.Read(t.Context(), Proof{}); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("guest read: %v", err)
	}
	denied, _ := preferencesFor(t, SessionProof, "message.read")
	if _, err := denied.Read(t.Context(), Proof{}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("session without the action: %v", err)
	}
	if _, err := denied.Save(t.Context(), Proof{}, 0, input, allowAll); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("session without the action saving: %v", err)
	}
}

func TestPreferencesValidationNamesTheField(t *testing.T) {
	t.Parallel()
	service, store := preferencesFor(t, SessionProof, PreferenceAction)
	for _, test := range []struct {
		field string
		input PreferencesInput
	}{
		{"locale", PreferencesInput{Locale: "", TimeZone: "UTC"}},
		{"locale", PreferencesInput{Locale: "not a locale", TimeZone: "UTC"}},
		{"locale", PreferencesInput{Locale: "und", TimeZone: "UTC"}},
		{"locale", PreferencesInput{Locale: "en-US-x-aaaaaaaa-bbbbbbbb-cccccccc-dddddddd", TimeZone: "UTC"}},
		{"timeZone", PreferencesInput{Locale: "en", TimeZone: ""}},
		{"timeZone", PreferencesInput{Locale: "en", TimeZone: "Local"}},
		{"timeZone", PreferencesInput{Locale: "en", TimeZone: "Mars/Olympus"}},
		{"timeGapMinutes", PreferencesInput{Locale: "en", TimeZone: "UTC", TimeGapMinutes: -1}},
		{"timeGapMinutes", PreferencesInput{Locale: "en", TimeZone: "UTC", TimeGapMinutes: 1441}},
	} {
		_, err := service.Save(t.Context(), Proof{}, 0, test.input, allowAll)
		var fieldError *authz.FieldError
		if !errors.As(err, &fieldError) || fieldError.Field != test.field {
			t.Fatalf("%+v: %v, want field %q", test.input, err, test.field)
		}
	}
	if len(store.rows) != 0 {
		t.Fatal("an invalid save stored a row")
	}
	saved, err := service.Save(t.Context(), Proof{}, 0, PreferencesInput{Locale: "zh_Hant_TW", TimeZone: "Asia/Taipei", TimeGapMinutes: 1440}, allowAll)
	if err != nil || saved.Locale != "zh-Hant-TW" {
		t.Fatalf("canonical locale = %+v, %v", saved, err)
	}
}

// TestSavingPreferencesNeedsTheTransportCheck expects a save without the state-change check of
// the transport to be refused before anything runs, as identity.WithAuthority requires.
func TestSavingPreferencesNeedsTheTransportCheck(t *testing.T) {
	t.Parallel()
	service, _ := preferencesFor(t, SessionProof, PreferenceAction)
	if _, err := service.Save(t.Context(), Proof{}, 0, PreferencesInput{Locale: "en", TimeZone: "UTC"}, nil); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("save without a check = %v, want INVALID_INPUT", err)
	}
}
