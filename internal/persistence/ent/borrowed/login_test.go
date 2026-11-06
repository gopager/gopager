package borrowed_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gopager/gopager/internal/testutil/passwords"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

type loginClock struct{ now time.Time }

func (c *loginClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return c.now, nil
}

type failingLoginFacts struct{ failure error }

func (f failingLoginFacts) AppendIdentityFact(context.Context, platform.TransactionContext, events.IdentityFact) (uuid.UUID, error) {
	return uuid.Nil, f.failure
}

// seedLoginAccount supplies explicit test-only password.local rights, not an administrator bypass.
func seedLoginAccount(t *testing.T, store *postgres.Store, username string, allowPassword bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	key, err := identity.UsernameKey(username)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := passwords.Hasher().Hash("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Create: map[string]bool{"Account": true, "Grant": true}})
		if err := client.Account.Create().SetID(id).SetKind(account.KindHuman).
			SetUsername(username).SetUsernameKey(key).SetPasswordHash(hash).SetCreatedAt(now).Exec(ctx); err != nil {
			return err
		}
		if !allowPassword {
			return nil
		}
		return client.Grant.Create().SetSubjectID(id).SetSubjectKind(grant.SubjectKindPrincipal).
			SetActions([]string{"password.local", "session.manage-own"}).SetScope(authz.Scope{Kind: "account", References: []string{id.String()}}).
			Exec(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOrdinaryLoginCommitsSessionAuditAndFactWithoutCommandIdentity(t *testing.T) {
	store := newStore(t)
	id := seedLoginAccount(t, store, "Straße", true)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(repository, store, repository)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginService(store, resolver, repository, facts, identity.DefaultLoginRateLimits(), passwords.Option())
	if err != nil {
		t.Fatal(err)
	}
	first, err := login.AuthenticateLocal(t.Context(), "test-source", "STRASSE", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := login.AuthenticateLocal(t.Context(), "test-source", "Straße", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID == second.SessionID || first.Secret == second.Secret || first.CSRFToken == second.CSRFToken {
		t.Fatal("ordinary repeated login did not create an independent Session")
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		actual, err := resolver.Resolve(tx.Context(), tx, identity.Proof{Kind: identity.SessionProof, Secret: first.Secret})
		if err != nil {
			return err
		}
		if actual.Authority.PrincipalID != id.String() || actual.CSRFToken != first.CSRFToken {
			t.Fatal("new Session cannot resolve current identity")
		}
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var credentials, audits, events, rawSecrets int
		hash, err := identity.SecretHash(first.Secret)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM identity_credentials),
			(SELECT count(*) FROM identity_audit),
			(SELECT count(*) FROM domain_events),
			(SELECT count(*) FROM identity_credentials WHERE encode(secret_hash, 'escape') = $1)`, first.Secret).
			Scan(&credentials, &audits, &events, &rawSecrets); err != nil {
			return err
		}
		if credentials != 2 || audits != 2 || events != 2 || rawSecrets != 0 {
			t.Fatalf("partial/secret-bearing login persistence: %d %d %d %d", credentials, audits, events, rawSecrets)
		}
		credential, err := repository.Credential(tx.Context(), tx, hash)
		if err != nil {
			return err
		}
		if credential.ExpiresAt == nil || !credential.ExpiresAt.Equal(credential.CreatedAt.Add(30*24*time.Hour)) {
			t.Fatal("incorrect absolute Session lifetime")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("fact append unavailable")
	broken, err := identity.NewLoginService(store, resolver, repository, failingLoginFacts{failure}, identity.DefaultLoginRateLimits(), passwords.Option())
	if err != nil {
		t.Fatal(err)
	}
	if result, err := broken.AuthenticateLocal(t.Context(), "test-source", "STRASSE", "correct-password"); !errors.Is(err, failure) || result.Secret != "" {
		t.Fatalf("failed fact append returned effective login: %v", err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var credentials, audit, facts int
		if err := executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM identity_credentials),
			(SELECT count(*) FROM identity_audit),
			(SELECT count(*) FROM domain_events)`).Scan(&credentials, &audit, &facts); err != nil {
			return err
		}
		if credentials != 2 || audit != 2 || facts != 2 {
			t.Fatal("login fact failure did not roll back Session and audit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoginRejectionAndPersistentAccountThrottle(t *testing.T) {
	store := newStore(t)
	id := seedLoginAccount(t, store, "Straße", true)
	seedLoginAccount(t, store, "excluded", false)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	clock := &loginClock{now: time.Date(2026, 10, 27, 12, 0, 0, 123456000, time.UTC)}
	resolver, err := identity.NewResolver(repository, clock, repository)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginService(store, resolver, repository, facts, identity.DefaultLoginRateLimits(), passwords.Option())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unknown", "excluded"} {
		if result, err := login.AuthenticateLocal(t.Context(), "test-source", name, "correct-password"); !errors.Is(err, identity.ErrLoginRequired) || result.Secret != "" {
			t.Fatalf("unknown/excluded account returned unexpected rejection: %v", err)
		}
	}
	for i := 0; i < 10; i++ {
		if _, err := login.AuthenticateLocal(t.Context(), "test-source", "STRASSE", "wrong-password"); !errors.Is(err, identity.ErrLoginRequired) {
			t.Fatalf("attempt %d not accounted/rejected: %v", i+1, err)
		}
	}
	// A new service instance must observe the same persisted account bucket.
	other, err := identity.NewLoginService(store, resolver, repository, facts, identity.DefaultLoginRateLimits(), passwords.Option())
	if err != nil {
		t.Fatal(err)
	}
	_, err = other.AuthenticateLocal(t.Context(), "test-source", "Straße", "correct-password")
	var limited *identity.RateLimited
	if !errors.As(err, &limited) || limited.RetryAfter != time.Minute {
		t.Fatalf("equivalent username bypassed persisted throttle: %v", err)
	}
	// Another account at the same location is not blocked by the account bucket.
	seedLoginAccount(t, store, "other-account", true)
	if _, err := other.AuthenticateLocal(t.Context(), "test-source", "other-account", "correct-password"); err != nil {
		t.Fatalf("unrelated account was blocked: %v", err)
	}
	clock.now = clock.now.Add(time.Minute)
	if _, err := login.AuthenticateLocal(t.Context(), "test-source", "STRASSE", "correct-password"); err != nil {
		t.Fatalf("account throttle did not expire at the exact boundary: %v", err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := repository.LockAccess(tx.Context(), tx); err != nil {
			return err
		}
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		ctx := accountContext(tx.Context(), driver, entql.FieldEQ("id", id), false)
		_, err = ent.NewClient(ent.Driver(driver)).Account.Update().SetState(account.StateSuspended).Save(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := login.AuthenticateLocal(t.Context(), "test-source", "STRASSE", "correct-password"); !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("suspended account logged in: %v", err)
	}
}

func TestLoginLogoutHTTPWithPrimaryAuthorityAndTLSProxy(t *testing.T) {
	for _, proxyTLS := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP", true: "HTTPS proxy HTTP upstream"}[proxyTLS], func(t *testing.T) {
			store := newStore(t)
			seedLoginAccount(t, store, "http-user", true)
			repository, err := identitystore.New(store)
			if err != nil {
				t.Fatal(err)
			}
			resolver, err := identity.NewResolver(repository, store, repository)
			if err != nil {
				t.Fatal(err)
			}
			facts, err := eventstore.New(store)
			if err != nil {
				t.Fatal(err)
			}
			login, err := identity.NewLoginService(store, resolver, repository, facts, identity.DefaultLoginRateLimits(), passwords.Option())
			if err != nil {
				t.Fatal(err)
			}
			var handler http.Handler
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.TLS != nil {
					t.Error("application was not reached over HTTP")
				}
				handler.ServeHTTP(w, r)
			}))
			defer backend.Close()
			edge := backend
			if proxyTLS {
				upstream, err := url.Parse(backend.URL)
				if err != nil {
					t.Fatal(err)
				}
				edge = httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
				defer edge.Close()
			}
			transport, err := identity.NewHTTPTransport(edge.URL)
			if err != nil {
				t.Fatal(err)
			}
			handler, err = identity.NewHTTPHandler(transport, login)
			if err != nil {
				t.Fatal(err)
			}
			client := edge.Client()
			client.Jar, err = cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			body := `{"schemaVersion":1,"username":"http-user","password":"correct-password"}`
			send := func(path, body, origin, csrf string, status int) ([]byte, []*http.Cookie) {
				t.Helper()
				request, err := http.NewRequest(http.MethodPost, edge.URL+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Origin", origin)
				request.Header.Set(identity.CSRFHeaderName, csrf)
				request.Header.Set("X-Forwarded-Proto", "http")
				request.Header.Set("X-Forwarded-Host", "foreign.example")
				response, err := client.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				content, readErr := io.ReadAll(response.Body)
				closeErr := response.Body.Close()
				if err := errors.Join(readErr, closeErr); err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != status {
					t.Fatalf("%s returned %d, want %d: %s", path, response.StatusCode, status, content)
				}
				if response.Header.Get("Cache-Control") != "no-store" {
					t.Fatal("authentication response is cacheable")
				}
				return content, response.Cookies()
			}
			send("/api/v1/auth/login", body, "https://foreign.example", "", http.StatusForbidden)
			prefix, suffix := `{"schemaVersion":1,"username":"http-user","password":"`, `"}`
			oversized := prefix + strings.Repeat("x", 16*1024+1-len(prefix)-len(suffix)) + suffix
			if len(oversized) != 16*1024+1 {
				t.Fatal("oversized login body has the wrong length")
			}
			limited, _ := send("/api/v1/auth/login", oversized, edge.URL, "", http.StatusRequestEntityTooLarge)
			var limitError struct {
				Code      string `json:"code"`
				Retryable bool   `json:"retryable"`
			}
			if err := json.Unmarshal(limited, &limitError); err != nil || limitError.Code != "RESOURCE_LIMIT" || limitError.Retryable {
				t.Fatalf("oversized login body was not RESOURCE_LIMIT: %s", limited)
			}
			content, cookies := send("/api/v1/auth/login", body, edge.URL, "", http.StatusOK)
			var result struct {
				SchemaVersion int    `json:"schemaVersion"`
				SessionID     string `json:"sessionId"`
				CSRFToken     string `json:"csrfToken"`
			}
			if err := json.Unmarshal(content, &result); err != nil {
				t.Fatal(err)
			}
			if result.SchemaVersion != 1 || result.CSRFToken == "" || len(cookies) != 1 || cookies[0].Secure != proxyTLS {
				t.Fatal("ordinary login did not issue correct browser Session")
			}
			if strings.Contains(string(content), cookies[0].Value) || strings.Contains(string(content), "correct-password") {
				t.Fatal("login JSON leaks password or cookie secret")
			}
			secret := cookies[0].Value
			response, err := client.Get(edge.URL + "/api/v1/auth/session")
			if err != nil {
				t.Fatal(err)
			}
			var current struct {
				SessionID string `json:"sessionId"`
				CSRFToken string `json:"csrfToken"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&current)
			closeErr := response.Body.Close()
			if err := errors.Join(decodeErr, closeErr); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || current.SessionID != result.SessionID || current.CSRFToken != result.CSRFToken {
				t.Fatal("browser reload cannot recover its own current Session/CSRF state")
			}
			bearerRequest, err := http.NewRequest(http.MethodGet, edge.URL+"/api/v1/auth/session", nil)
			if err != nil {
				t.Fatal(err)
			}
			bearerRequest.Header.Set("Authorization", "Bearer invalid")
			response, err = client.Do(bearerRequest)
			if err != nil {
				t.Fatal(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatal("current Session endpoint fell back from explicit bearer to Cookie")
			}
			send("/api/v1/auth/logout", "", edge.URL, "", http.StatusForbidden)
			send("/api/v1/auth/logout", "", "https://foreign.example", result.CSRFToken, http.StatusForbidden)
			send("/api/v1/auth/logout", "", edge.URL, result.CSRFToken, http.StatusNoContent)
			response, err = client.Get(edge.URL + "/api/v1/auth/session")
			if err != nil {
				t.Fatal(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatal("logged-out browser retained current Session access")
			}
			send("/api/v1/auth/logout", "", edge.URL, result.CSRFToken, http.StatusUnauthorized)
			err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				_, err := resolver.Resolve(tx.Context(), tx, identity.Proof{Kind: identity.SessionProof, Secret: secret})
				return err
			})
			if !errors.Is(err, identity.ErrLoginRequired) {
				t.Fatalf("logout did not revoke stored Session: %v", err)
			}
			// A browser can sign in again normally, without a receipt or command token.
			send("/api/v1/auth/login", body, edge.URL, "", http.StatusOK)
		})
	}
}
func TestLoginInstanceThrottleBoundsUnknownAccountStorage(t *testing.T) {
	store := newStore(t)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := repository.LockAccess(tx.Context(), tx); err != nil {
			return err
		}
		for i := 0; i < 300; i++ {
			var key, source [32]byte
			key[0], key[1] = byte(i), byte(i>>8)
			// Distinct sources keep the per-source bucket out of this instance-bucket test.
			source[0], source[1] = byte(i), byte(i>>8)
			retry, err := repository.TakeLoginAttempt(tx.Context(), tx, identity.LoginAttemptKeys{Account: key, Source: source}, now, identity.DefaultLoginRateLimits())
			if err != nil {
				return err
			}
			if retry != 0 {
				t.Fatalf("instance limit prematurely blocked attempt %d", i+1)
			}
		}
		for i := 0; i < 10; i++ {
			retry, err := repository.TakeLoginAttempt(tx.Context(), tx, identity.LoginAttemptKeys{Account: [32]byte{255, 255}, Source: [32]byte{250}}, now, identity.DefaultLoginRateLimits())
			if err != nil {
				return err
			}
			if retry != time.Minute {
				t.Fatal("instance throttle missing safe retry interval")
			}
		}
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var count int
		if err := executor.QueryRowContext(tx.Context(), "SELECT count(*) FROM identity_login_attempts").Scan(&count); err != nil {
			return err
		}
		if count != 300 {
			t.Fatalf("unknown-account attempts exceed bounded storage: %d", count)
		}
		retry, err := repository.TakeLoginAttempt(tx.Context(), tx, identity.LoginAttemptKeys{Account: [32]byte{255, 255}, Source: [32]byte{250}}, now.Add(time.Minute), identity.DefaultLoginRateLimits())
		if err != nil {
			return err
		}
		if retry != 0 {
			t.Fatal("instance throttle failed to clear expired attempts")
		}
		limits := identity.LoginRateLimits{AccountAttempts: 1, SourceAttempts: 100, InstanceAttempts: 2}
		retry, err = repository.TakeLoginAttempt(tx.Context(), tx, identity.LoginAttemptKeys{Account: [32]byte{255, 255}, Source: [32]byte{250}}, now.Add(time.Minute), limits)
		if err != nil {
			return err
		}
		if retry != time.Minute {
			t.Fatal("configured shorter account limit was ignored")
		}
		retry, err = repository.TakeLoginAttempt(tx.Context(), tx, identity.LoginAttemptKeys{Account: [32]byte{254, 254}, Source: [32]byte{251}}, now.Add(time.Minute), limits)
		if err != nil {
			return err
		}
		if retry != 0 {
			t.Fatal("configured account limit blocked a different account")
		}
		retry, err = repository.TakeLoginAttempt(tx.Context(), tx, identity.LoginAttemptKeys{Account: [32]byte{253, 253}, Source: [32]byte{252}}, now.Add(time.Minute), limits)
		if err != nil {
			return err
		}
		if retry != time.Minute {
			t.Fatal("configured instance limit was ignored")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLoginSourceThrottleSpansUsernames: one client source trying many usernames is stopped
// by the 31st attempt within the minute without blocking other sources or the instance.
func TestLoginSourceThrottleSpansUsernames(t *testing.T) {
	store := newStore(t)
	seedLoginAccount(t, store, "operator", true)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	clock := &loginClock{now: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	resolver, err := identity.NewResolver(repository, clock, repository)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginService(store, resolver, repository, facts, identity.DefaultLoginRateLimits(), passwords.Option())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		_, err := login.AuthenticateLocal(t.Context(), "203.0.113.7", fmt.Sprintf("guess-%d", i), "wrong-password")
		if !errors.Is(err, identity.ErrLoginRequired) {
			t.Fatalf("attempt %d: %v, want the generic rejection", i+1, err)
		}
	}
	_, err = login.AuthenticateLocal(t.Context(), "203.0.113.7", "guess-30", "wrong-password")
	var limited *identity.RateLimited
	if !errors.As(err, &limited) || limited.RetryAfter != time.Minute {
		t.Fatalf("31st attempt from one source = %v, want RateLimited for a minute", err)
	}
	// The flooding source cannot lock the operator out from another address.
	if _, err := login.AuthenticateLocal(t.Context(), "198.51.100.9", "operator", "correct-password"); err != nil {
		t.Fatalf("other source blocked: %v", err)
	}
	if _, err := login.AuthenticateLocal(t.Context(), "203.0.113.7", "operator", "correct-password"); !errors.As(err, &limited) {
		t.Fatalf("throttled source still reached the account: %v", err)
	}
	clock.now = clock.now.Add(time.Minute)
	if _, err := login.AuthenticateLocal(t.Context(), "203.0.113.7", "operator", "correct-password"); err != nil {
		t.Fatalf("source throttle did not expire at the boundary: %v", err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var raw int
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM identity_login_attempts
			WHERE encode(source_key, 'escape') LIKE '%203.0.113%'`).Scan(&raw); err != nil {
			return err
		}
		if raw != 0 {
			t.Fatal("client address stored in clear")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
