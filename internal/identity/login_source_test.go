package identity

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
)

type fakeTransactionContext struct{ ctx context.Context }

func (f fakeTransactionContext) Context() context.Context { return f.ctx }

type fakeTransactions struct{}

func (fakeTransactions) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	return callback(fakeTransactionContext{ctx})
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return c.now, nil
}

// recordingLoginRepository counts attempts per source bucket the way the real store
// does, so the test pins down what the service hands to accounting.
type recordingLoginRepository struct {
	LoginRepository
	keys      []LoginAttemptKeys
	perSource map[[32]byte]int
}

func (r *recordingLoginRepository) TakeLoginAttempt(_ context.Context, _ platform.TransactionContext, keys LoginAttemptKeys, _ time.Time, limits LoginRateLimits) (time.Duration, error) {
	r.keys = append(r.keys, keys)
	if r.perSource == nil {
		r.perSource = map[[32]byte]int{}
	}
	if r.perSource[keys.Source] >= limits.SourceAttempts {
		return time.Minute, nil
	}
	r.perSource[keys.Source]++
	return 0, nil
}

func (r *recordingLoginRepository) LoginAccount(context.Context, platform.TransactionContext, string) (LoginAccount, error) {
	return LoginAccount{}, ErrLoginRequired
}

type noFacts struct{}

func (noFacts) AppendIdentityFact(context.Context, platform.TransactionContext, events.IdentityFact) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func newSourceTestService(t *testing.T, repository LoginRepository) *LoginService {
	t.Helper()
	resolver := &Resolver{clock: fixedClock{time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}}
	// Reduced costs: the throttle, not Argon2id, is under test. The internal package cannot use
	// testutil/passwords without an import cycle.
	hasher, err := NewPasswordHasher(PasswordParams{MemoryKiB: 8, Iterations: 1, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewLoginService(fakeTransactions{}, resolver, repository, noFacts{}, DefaultLoginRateLimits(), WithPasswordHasher(hasher))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestDefaultLoginRateLimitsIncludeSourceLimit(t *testing.T) {
	if limits := DefaultLoginRateLimits(); limits.SourceAttempts != 30 || limits.AccountAttempts != 10 || limits.InstanceAttempts != 300 {
		t.Fatalf("default limits = %+v", limits)
	}
	if _, err := NewLoginService(fakeTransactions{}, &Resolver{}, &recordingLoginRepository{}, noFacts{},
		LoginRateLimits{AccountAttempts: 1, InstanceAttempts: 1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero source limit accepted: %v", err)
	}
}

// TestSourceBucketSpansUsernames: one source cycling through usernames is stopped by the
// 31st attempt, while another source is unaffected.
func TestSourceBucketSpansUsernames(t *testing.T) {
	repository := &recordingLoginRepository{}
	service := newSourceTestService(t, repository)
	for i := range 30 {
		_, err := service.AuthenticateLocal(t.Context(), "203.0.113.7", "user-"+string(rune('a'+i%26))+string(rune('a'+i/26)), "password-password")
		if !errors.Is(err, ErrLoginRequired) {
			t.Fatalf("attempt %d: %v, want the generic rejection", i+1, err)
		}
	}
	var limited *RateLimited
	if _, err := service.AuthenticateLocal(t.Context(), "203.0.113.7", "yet-another", "password-password"); !errors.As(err, &limited) {
		t.Fatalf("31st attempt from one source = %v, want RateLimited", err)
	}
	if _, err := service.AuthenticateLocal(t.Context(), "203.0.113.8", "yet-another", "password-password"); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("another source was affected: %v", err)
	}
	if repository.keys[0].Source != sha256.Sum256([]byte("login-source:203.0.113.7")) {
		t.Fatal("source key is not the domain-separated hash of the source")
	}
	if repository.keys[0].Account == repository.keys[0].Source {
		t.Fatal("account and source keys share one namespace")
	}
}

func TestLoginSourceFromRequestUsesRemoteHostOnly(t *testing.T) {
	for _, test := range []struct{ remote, header, want string }{
		{"203.0.113.7:51234", "", "203.0.113.7"},
		{"[2001:db8::1]:443", "", "2001:db8::1"},
		{"203.0.113.7:51234", "198.51.100.1", "203.0.113.7"},
		{"unix-socket", "", "unix-socket"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		request.RemoteAddr = test.remote
		if test.header != "" {
			request.Header.Set("X-Forwarded-For", test.header)
		}
		if got := loginSource(request); got != test.want {
			t.Errorf("loginSource(%q, XFF %q) = %q, want %q", test.remote, test.header, got, test.want)
		}
	}
}
