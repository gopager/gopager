package identity

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
)

// Transactions is the existing platform transaction boundary, without store ownership.
type Transactions interface {
	WithinTransaction(context.Context, platform.TxFunc) error
}

// LoginAccount contains the minimal primary-store password-verification state.
type LoginAccount struct {
	AccountRecord
	PasswordHash string
}

// LogValue prevents password material from entering structured diagnostics.
func (a LoginAccount) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", a.ID.String()))
}

// SessionCreation contains storage-only credential material for a new login Session.
type SessionCreation struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	SecretHash [32]byte
	CSRFToken  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// LogValue exposes only nonsecret Session references.
func (s SessionCreation) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", s.ID.String()), slog.String("accountId", s.AccountID.String()))
}

// LoginAttemptKeys identify one login attempt in the persistent throttle buckets. Both
// are hashes, so the throttle table holds neither usernames nor client addresses.
type LoginAttemptKeys struct {
	// Account is the hash of the normalized username.
	Account [32]byte
	// Source is the hash of the client source (see AuthenticateLocal).
	Source [32]byte
}

// LoginRepository owns bounded persistent throttling and Session/audit persistence.
// TakeLoginAttempt serializes its own accounting and returns a safe retry delay.
type LoginRepository interface {
	TakeLoginAttempt(context.Context, platform.TransactionContext, LoginAttemptKeys, time.Time, LoginRateLimits) (time.Duration, error)
	LoginAccount(context.Context, platform.TransactionContext, string) (LoginAccount, error)
	CreateSession(context.Context, platform.TransactionContext, SessionCreation) error
	RevokeSession(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID, time.Time) (int64, error)
}

// RateLimited exposes only a safe retry interval, not account existence.
type RateLimited struct{ RetryAfter time.Duration }

// Error returns a generic user-facing message without account details.
func (e *RateLimited) Error() string { return "Too many attempts. Try again later." }

// SessionResult is returned once after a successful ordinary login.
type SessionResult struct {
	SessionID uuid.UUID
	Secret    string
	CSRFToken string
	ExpiresAt time.Time
}

// SessionView is the current browser's own Session state, without grants or other users' data.
type SessionView struct {
	PrincipalID string
	SessionID   uuid.UUID
	CSRFToken   string
	ExpiresAt   time.Time
}

// LogValue omits the Session-bound CSRF material.
func (s SessionView) LogValue() slog.Value {
	return slog.GroupValue(slog.String("sessionId", s.SessionID.String()))
}

// LogValue omits all browser authentication material.
func (s SessionResult) LogValue() slog.Value {
	return slog.GroupValue(slog.String("sessionId", s.SessionID.String()))
}

// LoginService implements local password login (AuthenticateLocal), Logout and
// ReadSession with persistent per-account, per-source and per-instance throttling. Create
// it with NewLoginService.
type LoginService struct {
	transactions Transactions
	resolver     *Resolver
	repository   LoginRepository
	facts        events.Appender
	dummyHash    string
	hasher       PasswordHasher
	limits       LoginRateLimits
}

// LoginRateLimits are per-minute limits. The account bucket is the normalized
// username only, so one shared address cannot lock out other accounts through it;
// the source bucket caps what one client source can try across all usernames, so
// that a single anonymous client cannot exhaust the instance bucket (and with it
// every operator's login) or lock accounts out one by one.
type LoginRateLimits struct {
	AccountAttempts  int
	SourceAttempts   int
	InstanceAttempts int
}

// DefaultLoginRateLimits returns the default limits: 10 attempts per minute per
// account, 30 per minute per client source and 300 per minute per instance. Each
// call returns a fresh value.
func DefaultLoginRateLimits() LoginRateLimits {
	return LoginRateLimits{AccountAttempts: 10, SourceAttempts: 30, InstanceAttempts: 300}
}

// NewLoginService composes ordinary login and prepares one salted dummy password hash.
// Options are optional; see WithPasswordHasher.
func NewLoginService(transactions Transactions, resolver *Resolver, repository LoginRepository, facts events.Appender, limits LoginRateLimits, options ...Option) (*LoginService, error) {
	if transactions == nil || resolver == nil || repository == nil || facts == nil ||
		limits.AccountAttempts <= 0 || limits.SourceAttempts <= 0 || limits.InstanceAttempts <= 0 {
		return nil, ErrInvalidInput
	}
	dummy, _, err := NewSecret()
	if err != nil {
		return nil, err
	}
	hasher := newSettings(options).hasher
	hash, err := hasher.Hash(dummy)
	if err != nil {
		return nil, err
	}
	return &LoginService{transactions: transactions, resolver: resolver, repository: repository,
		facts: facts, dummyHash: hash, hasher: hasher, limits: limits}, nil
}

// AuthenticateLocal verifies current credentials/rights and commits one new Session.
// Attempt accounting commits in its own short transaction even when the attempt is
// rejected; password verification runs outside any transaction so that the Argon2id work
// holds no lock; the Session is created in a second transaction under the shared access
// lock after the account state was re-read. Infrastructure failures never become login
// success.
//
// source identifies the client for the per-source throttle, normally the host part of
// the connection's remote address (see NewHTTPHandler). It is hashed before storage
// and never trusted for anything but throttling; an empty source shares one bucket.
func (s *LoginService) AuthenticateLocal(ctx context.Context, source, username, password string) (SessionResult, error) {
	key, err := UsernameKey(username)
	if err != nil {
		return SessionResult{}, err
	}
	keys := LoginAttemptKeys{
		Account: sha256.Sum256([]byte(key)),
		// The prefix keeps the two bucket namespaces disjoint even if a username
		// equals a source string.
		Source: sha256.Sum256([]byte("login-source:" + source)),
	}
	var account LoginAccount
	var rejection error
	err = s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := s.resolver.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		retry, err := s.repository.TakeLoginAttempt(tx.Context(), tx, keys, now, s.limits)
		if err != nil {
			return err
		}
		if retry > 0 {
			rejection = &RateLimited{RetryAfter: retry}
			return nil
		}
		account, err = s.repository.LoginAccount(tx.Context(), tx, key)
		if err != nil && !errors.Is(err, ErrLoginRequired) {
			return err
		}
		return nil
	})
	if err != nil {
		return SessionResult{}, err
	}
	if rejection != nil {
		return SessionResult{}, rejection
	}
	hash := account.PasswordHash
	if hash == "" {
		hash = s.dummyHash
	}
	valid, err := s.hasher.Verify(hash, password)
	if err != nil {
		return SessionResult{}, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	if !valid || account.ID == uuid.Nil || account.Kind != "human" ||
		account.State != "active" || account.PasswordHash == "" {
		return SessionResult{}, ErrLoginRequired
	}
	var result SessionResult
	err = s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		revision, err := s.resolver.repository.LockAccess(tx.Context(), tx)
		if err != nil {
			return err
		}
		now, err := s.resolver.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		current, err := s.repository.LoginAccount(tx.Context(), tx, key)
		if err != nil && !errors.Is(err, ErrLoginRequired) {
			return err
		}
		if err != nil || current.ID != account.ID || current.Kind != "human" ||
			current.State != "active" || current.PasswordHash != account.PasswordHash {
			rejection = ErrLoginRequired
			return nil
		}
		authority, err := s.resolver.accountAuthority(tx.Context(), tx, account.ID, revision)
		if err != nil {
			return err
		}
		if err := authz.Authorize(authority, "password.local", nil, authz.Target{AccountID: account.ID}, now); err != nil {
			if !errors.Is(err, authz.ErrForbidden) {
				return err
			}
			rejection = ErrLoginRequired
			return nil
		}
		secret, secretHash, err := NewSecret()
		if err != nil {
			return err
		}
		csrf, _, err := NewSecret()
		if err != nil {
			return err
		}
		session := SessionCreation{ID: uuid.New(), AccountID: account.ID, SecretHash: secretHash,
			CSRFToken: csrf, CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}
		if err := s.repository.CreateSession(tx.Context(), tx, session); err != nil {
			return err
		}
		if _, err := s.facts.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
			ID: uuid.New(), Type: events.ConfigurationChanged,
			Aggregate: events.EntityRef{Type: "credential", ID: session.ID}, Revision: 1,
			OccurredAt: now, ChangeKind: events.CredentialIssued,
		}); err != nil {
			return err
		}
		result = SessionResult{SessionID: session.ID, Secret: secret, CSRFToken: csrf, ExpiresAt: session.ExpiresAt}
		return nil
	})
	if err != nil {
		return SessionResult{}, err
	}
	if rejection != nil {
		return SessionResult{}, rejection
	}
	return result, nil
}

// Logout revokes the current Session with current own-session authority. It is an
// access change: it takes the exclusive access lock before resolving the Session, so it
// waits for in-flight authority readers and never deadlocks with another access change.
// The transport must check Origin/CSRF against the authentication inside this transaction.
func (s *LoginService) Logout(ctx context.Context, proof Proof, check func(Authentication) error) error {
	if proof.Kind != SessionProof || check == nil {
		return ErrLoginRequired
	}
	var rejection error
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		if _, err := s.resolver.repository.LockAccessForChange(tx.Context(), tx); err != nil {
			return err
		}
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			if errors.Is(err, ErrLoginRequired) {
				rejection = err
				return nil
			}
			return err
		}
		if err := check(authentication); err != nil {
			rejection = err
			return nil
		}
		now, err := s.resolver.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		id, err := uuid.Parse(authentication.Authority.PrincipalID)
		if err != nil {
			return ErrInvalidInput
		}
		if err := authz.Authorize(authentication.Authority, "session.manage-own", nil, authz.Target{AccountID: id}, now); err != nil {
			rejection = err
			return nil
		}
		revision, err := s.repository.RevokeSession(tx.Context(), tx, id, authentication.CredentialID, now)
		if err != nil {
			return err
		}
		_, err = s.facts.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
			ID: uuid.New(), Type: events.PrincipalAccessChanged,
			Aggregate: events.EntityRef{Type: "credential", ID: authentication.CredentialID}, Revision: revision,
			OccurredAt: now, ChangeKind: events.CredentialRevoked,
		})
		return err
	})
	if err != nil {
		return err
	}
	return rejection
}

// ReadSession resolves the current browser Session afresh, including its CSRF token after reload.
func (s *LoginService) ReadSession(ctx context.Context, proof Proof) (SessionView, error) {
	if proof.Kind != SessionProof {
		return SessionView{}, ErrLoginRequired
	}
	var result SessionView
	var rejection error
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			if errors.Is(err, ErrLoginRequired) {
				rejection = err
				return nil
			}
			return err
		}
		if authentication.ExpiresAt == nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Session expiry unavailable"))
		}
		result = SessionView{PrincipalID: authentication.Authority.PrincipalID, SessionID: authentication.CredentialID,
			CSRFToken: authentication.CSRFToken, ExpiresAt: *authentication.ExpiresAt}
		return nil
	})
	if err != nil {
		return SessionView{}, err
	}
	if rejection != nil {
		return SessionView{}, rejection
	}
	return result, nil
}
