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

type authorityFixture struct {
	now          time.Time
	account      AccountRecord
	credential   CredentialRecord
	grants       []authz.Grant
	fields       []authz.FieldPermission
	fieldFailure error
	exists       bool
	touched      bool
	scopeCalls   int
	failure      error
}

func (f *authorityFixture) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return f.now, nil
}

func (f *authorityFixture) LockAccess(context.Context, platform.TransactionContext) (int64, error) {
	return 7, f.failure
}

func (f *authorityFixture) LockAccessForChange(context.Context, platform.TransactionContext) (int64, error) {
	return 7, f.failure
}

func (f *authorityFixture) Account(context.Context, platform.TransactionContext, uuid.UUID) (AccountRecord, error) {
	return f.account, nil
}

func (f *authorityFixture) Guest(context.Context, platform.TransactionContext) (AccountRecord, error) {
	return AccountRecord{ID: uuid.New(), Kind: "guest", State: "active"}, nil
}

func (f *authorityFixture) Credential(context.Context, platform.TransactionContext, [32]byte) (CredentialRecord, error) {
	return f.credential, nil
}

func (f *authorityFixture) Grants(context.Context, platform.TransactionContext, uuid.UUID) ([]authz.Grant, error) {
	return f.grants, nil
}

func (f *authorityFixture) FieldPermissions(context.Context, platform.TransactionContext, uuid.UUID) ([]authz.FieldPermission, error) {
	return f.fields, f.fieldFailure
}

func (f *authorityFixture) TouchCredential(context.Context, platform.TransactionContext, uuid.UUID, time.Time) error {
	f.touched = true
	return nil
}

func (f *authorityFixture) ScopesExist(_ context.Context, _ platform.TransactionContext, scopes []authz.Scope) ([]bool, error) {
	f.scopeCalls++
	result := make([]bool, len(scopes))
	for i := range result {
		result[i] = f.exists
	}
	return result, nil
}

func newAuthorityFixture(t *testing.T) (*authorityFixture, Proof) {
	t.Helper()
	now := time.Date(2026, 10, 26, 12, 0, 0, 123456000, time.UTC)
	expires := now.Add(time.Hour)
	principal := uuid.New()
	secret, _, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	return &authorityFixture{
		now: now, exists: true,
		account: AccountRecord{ID: principal, Kind: "human", State: "active"},
		credential: CredentialRecord{ID: uuid.New(), AccountID: principal, Kind: SessionProof,
			CreatedAt: now.Add(-time.Hour), LastUsedAt: now.Add(-10 * time.Minute), ExpiresAt: &expires},
		grants: []authz.Grant{{SubjectID: principal.String(), SubjectKind: "principal",
			Actions: []string{"account.read"}, Scope: authz.Scope{Kind: "all"}}},
	}, Proof{Kind: SessionProof, Secret: secret}
}

func TestResolverSessionBoundariesAndAccountState(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*authorityFixture)
		allow  bool
	}{
		{"current", func(*authorityFixture) {}, true},
		{"expiry inclusive rejection", func(f *authorityFixture) { f.credential.ExpiresAt = &f.now }, false},
		{"expiry microsecond before", func(f *authorityFixture) {
			expiry := f.now.Add(time.Microsecond)
			f.credential.ExpiresAt = &expiry
		}, true},
		{"missing session expiry", func(f *authorityFixture) { f.credential.ExpiresAt = nil }, false},
		{"absolute boundary", func(f *authorityFixture) { f.credential.CreatedAt = f.now.Add(-30 * 24 * time.Hour) }, false},
		{"absolute microsecond before", func(f *authorityFixture) {
			f.credential.CreatedAt = f.now.Add(-30*24*time.Hour + time.Microsecond)
		}, true},
		{"inactivity boundary", func(f *authorityFixture) { f.credential.LastUsedAt = f.now.Add(-7 * 24 * time.Hour) }, false},
		{"inactivity microsecond before", func(f *authorityFixture) {
			f.credential.LastUsedAt = f.now.Add(-7*24*time.Hour + time.Microsecond)
		}, true},
		{"revoked", func(f *authorityFixture) { f.credential.RevokedAt = &f.now }, false},
		{"consumed", func(f *authorityFixture) { f.credential.ConsumedAt = &f.now }, false},
		{"suspended", func(f *authorityFixture) { f.account.State = "suspended" }, false},
		{"deleting", func(f *authorityFixture) { f.account.State = "deleting" }, false},
		{"deleted", func(f *authorityFixture) { f.account.State = "deleted" }, false},
		{"technical cannot use session", func(f *authorityFixture) { f.account.Kind = "technical" }, false},
		{"guest cannot use session", func(f *authorityFixture) { f.account.Kind = "guest" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, proof := newAuthorityFixture(t)
			test.change(fixture)
			resolver, err := NewResolver(fixture, fixture, fixture)
			if err != nil {
				t.Fatal(err)
			}
			result, err := resolver.Resolve(t.Context(), nil, proof)
			if test.allow {
				if err != nil || !fixture.touched || result.Authority.Revision != 7 {
					t.Fatalf("valid session failed: %v", err)
				}
			} else if !errors.Is(err, ErrLoginRequired) || fixture.touched || result.Authority.Active {
				t.Fatalf("invalid session gained authority or last-use: %v", err)
			}
		})
	}
}

func TestResolverDisablesMissingGrantScopeAndRejectsBrokenKey(t *testing.T) {
	fixture, proof := newAuthorityFixture(t)
	fixture.exists = false
	resolver, err := NewResolver(fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}

	result, err := resolver.Resolve(t.Context(), nil, proof)
	if err != nil || len(result.Authority.Grants) != 0 {
		t.Fatalf("missing scope did not disable grant: %v", err)
	}
	proof.Kind = AccessKeyProof
	fixture.credential.Kind = AccessKeyProof
	fixture.touched = false
	if _, err := resolver.Resolve(t.Context(), nil, proof); err == nil || fixture.touched {
		t.Fatal("key without restrictions accepted or touched")
	}
	fixture.credential.Restrictions = &authz.Restrictions{Actions: []string{"account.read"},
		Scopes: []authz.Scope{{Kind: "all"}}}
	if _, err := resolver.Resolve(t.Context(), nil, proof); !errors.Is(err, ErrLoginRequired) || fixture.touched {
		t.Fatalf("deleted key scope expanded authority: %v", err)
	}
	fixture.exists = true
	fixture.credential.ExpiresAt = nil
	if _, err := resolver.Resolve(t.Context(), nil, proof); err != nil || !fixture.touched {
		t.Fatalf("explicit unlimited key failed: %v", err)
	}
	fixture.failure = errors.New("primary unavailable")
	fixture.touched = false
	if _, err := resolver.Resolve(t.Context(), nil, proof); !errors.Is(err, fixture.failure) || fixture.touched {
		t.Fatalf("authority storage failure swallowed: %v", err)
	}
}

func TestResolverRejectsBrokenGlobalFieldAuthorityWithoutFallback(t *testing.T) {
	fixture, proof := newAuthorityFixture(t)
	resolver, err := NewResolver(fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixture.fieldFailure = errors.New("field authority unavailable")
	if _, err := resolver.Resolve(t.Context(), nil, proof); !errors.Is(err, fixture.fieldFailure) || fixture.touched {
		t.Fatalf("global authority failure became empty success: %v", err)
	}
	fixture.fieldFailure = nil
	fixture.fields = []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
		SubjectID: fixture.account.ID.String(), SubjectKind: "principal", Fields: []string{"*"}}}
	if _, err := resolver.Resolve(t.Context(), nil, proof); platform.FailureCode(err) != platform.CodeAuthorityUnavailable || fixture.touched {
		t.Fatalf("invalid global field state became usable authority: %v", err)
	}
}

func TestResolverBatchesScopeExistenceChecks(t *testing.T) {
	fixture, proof := newAuthorityFixture(t)
	for i := 0; i < 25; i++ {
		fixture.grants = append(fixture.grants, authz.Grant{SubjectID: fixture.account.ID.String(),
			SubjectKind: "principal", Actions: []string{"unit.read"},
			Scope: authz.Scope{Kind: "unit", References: []string{uuid.NewString()}}})
	}
	resolver, err := NewResolver(fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := resolver.Resolve(t.Context(), nil, proof)
	if err != nil || len(result.Authority.Grants) != 26 || fixture.scopeCalls != 1 {
		t.Fatalf("session resolution grants=%d calls=%d err=%v, want 26 grants in one batched call",
			len(result.Authority.Grants), fixture.scopeCalls, err)
	}
	fixture.scopeCalls = 0
	proof.Kind = AccessKeyProof
	fixture.credential.Kind = AccessKeyProof
	fixture.credential.Restrictions = &authz.Restrictions{Actions: []string{"unit.read"}}
	for i := 0; i < 10; i++ {
		fixture.credential.Restrictions.Scopes = append(fixture.credential.Restrictions.Scopes,
			authz.Scope{Kind: "unit", References: []string{uuid.NewString()}})
	}
	if _, err := resolver.Resolve(t.Context(), nil, proof); err != nil || fixture.scopeCalls != 2 {
		t.Fatalf("key resolution calls=%d err=%v, want one call for grants and one for key scopes", fixture.scopeCalls, err)
	}
}

func TestResolverThrottlesLastUsedWrites(t *testing.T) {
	for _, test := range []struct {
		name string
		idle time.Duration
		want bool
	}{
		{"just used", time.Minute, false},
		{"one microsecond short", touchInterval - time.Microsecond, false},
		{"at interval", touchInterval, true},
		{"long idle", time.Hour, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, proof := newAuthorityFixture(t)
			fixture.credential.LastUsedAt = fixture.now.Add(-test.idle)
			resolver, err := NewResolver(fixture, fixture, fixture)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := resolver.Resolve(t.Context(), nil, proof); err != nil || fixture.touched != test.want {
				t.Fatalf("touched=%t want %t err=%v", fixture.touched, test.want, err)
			}
		})
	}
}
