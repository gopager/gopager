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

// LoginRepository owns bounded persistent throttling and Session/audit persistence.
// TakeLoginAttempt is called under the shared authority lock and returns a safe retry delay.
type LoginRepository interface {
	TakeLoginAttempt(context.Context, platform.TransactionContext, [32]byte, time.Time, LoginRateLimits) (time.Duration, error)
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
// ReadSession with persistent per-account and per-instance throttling. Create
// it with NewLoginService.
type LoginService struct {
	transactions Transactions
	resolver     *Resolver
	repository   LoginRepository
	facts        events.Appender
	dummyHash    string
	limits       LoginRateLimits
}

// LoginRateLimits are per-minute limits; account identity is the normalized username, never the IP.
type LoginRateLimits struct {
	AccountAttempts  int
	InstanceAttempts int
}

// DefaultLoginRateLimits returns the default limits: 10 attempts per minute per
// account and 300 per minute per instance. Each call returns a fresh value.
func DefaultLoginRateLimits() LoginRateLimits {
	return LoginRateLimits{AccountAttempts: 10, InstanceAttempts: 300}
}

// NewLoginService composes ordinary login and prepares one salted dummy password hash.
func NewLoginService(transactions Transactions, resolver *Resolver, repository LoginRepository, facts events.Appender, limits LoginRateLimits) (*LoginService, error) {
	if transactions == nil || resolver == nil || repository == nil || facts == nil ||
		limits.AccountAttempts <= 0 || limits.InstanceAttempts <= 0 {
		return nil, ErrInvalidInput
	}
	dummy, _, err := NewSecret()
	if err != nil {
		return nil, err
	}
	hash, err := HashPassword(dummy)
	if err != nil {
		return nil, err
	}
	return &LoginService{transactions: transactions, resolver: resolver, repository: repository,
		facts: facts, dummyHash: hash, limits: limits}, nil
}

// AuthenticateLocal verifies current credentials/rights and commits one new Session.
// Rejected attempts still commit throttle accounting; infrastructure failures never become login success.
func (s *LoginService) AuthenticateLocal(ctx context.Context, username, password string) (SessionResult, error) {
	key, err := UsernameKey(username)
	if err != nil {
		return SessionResult{}, err
	}
	var result SessionResult
	var rejection error
	err = s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		revision, err := s.resolver.repository.LockAccess(tx.Context(), tx)
		if err != nil {
			return err
		}
		now, err := s.resolver.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		retry, err := s.repository.TakeLoginAttempt(tx.Context(), tx, sha256.Sum256([]byte(key)), now, s.limits)
		if err != nil {
			return err
		}
		if retry > 0 {
			rejection = &RateLimited{RetryAfter: retry}
			return nil
		}
		account, err := s.repository.LoginAccount(tx.Context(), tx, key)
		if err != nil && !errors.Is(err, ErrLoginRequired) {
			return err
		}
		hash := account.PasswordHash
		if hash == "" {
			hash = s.dummyHash
		}
		valid, err := VerifyPassword(hash, password)
		if err != nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		if !valid || account.ID == uuid.Nil || account.Kind != "human" ||
			account.State != "active" || account.PasswordHash == "" {
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

// Logout revokes the current Session with current own-session authority.
// The transport must check Origin/CSRF against the authentication inside this transaction.
func (s *LoginService) Logout(ctx context.Context, proof Proof, check func(Authentication) error) error {
	if proof.Kind != SessionProof || check == nil {
		return ErrLoginRequired
	}
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
