package identity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// ProofKind is the kind of transport proof a request carried.
type ProofKind string

// Proof kinds. A credential's Kind must match the proof kind it is presented as.
const (
	// AnonymousProof carries no secret and resolves to the guest account.
	AnonymousProof ProofKind = "anonymous"
	// SessionProof is a browser session cookie secret for a human account.
	SessionProof ProofKind = "session"
	// AccessKeyProof is a bearer access-key secret, optionally restricted.
	AccessKeyProof ProofKind = "accessKey"
)

// Proof contains only transport proof, never caller-supplied identity or grants.
type Proof struct {
	Kind   ProofKind
	Secret string
}

// LogValue implements slog.LogValuer and logs only the proof kind, never the
// secret.
func (p Proof) LogValue() slog.Value {
	return slog.GroupValue(slog.String("kind", string(p.Kind)))
}

// AccountRecord is the stored account state used for authority resolution.
// Kind is human, technical or guest; only State "active" can authenticate.
type AccountRecord struct {
	ID    uuid.UUID
	Kind  string
	State string
}

// CredentialRecord is the stored state of a session or access-key credential,
// looked up by secret hash. Restrictions is set only for restricted access
// keys; CSRFToken only for sessions.
type CredentialRecord struct {
	ID           uuid.UUID
	AccountID    uuid.UUID
	Kind         ProofKind
	CreatedAt    time.Time
	LastUsedAt   time.Time
	ExpiresAt    *time.Time
	RevokedAt    *time.Time
	ConsumedAt   *time.Time
	CSRFToken    string
	Restrictions *authz.Restrictions
}

// LogValue implements slog.LogValuer and logs only the credential ID and kind.
func (c CredentialRecord) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", c.ID.String()), slog.String("kind", string(c.Kind)))
}

// AuthorityRepository loads primary identity state under the access lock. LockAccess is the
// shared lock that authority resolution holds; configuration changes bump the access
// revision without waiting for it. LockAccessForChange is the exclusive lock for commands
// that change grants, field permissions, credentials or account state; such a command takes
// it before it resolves authority, so two of them never wait for each other's shared lock.
type AuthorityRepository interface {
	LockAccess(context.Context, platform.TransactionContext) (int64, error)
	LockAccessForChange(context.Context, platform.TransactionContext) (int64, error)
	Account(context.Context, platform.TransactionContext, uuid.UUID) (AccountRecord, error)
	Guest(context.Context, platform.TransactionContext) (AccountRecord, error)
	Credential(context.Context, platform.TransactionContext, [32]byte) (CredentialRecord, error)
	Grants(context.Context, platform.TransactionContext, uuid.UUID) ([]authz.Grant, error)
	FieldPermissions(context.Context, platform.TransactionContext, uuid.UUID) ([]authz.FieldPermission, error)
	TouchCredential(context.Context, platform.TransactionContext, uuid.UUID, time.Time) error
}

// touchInterval is the minimum age of a credential's stored last-use time before Resolve
// writes it again. It spares a write per authenticated request while keeping the session
// inactivity rule honest to within this interval.
const touchInterval = 5 * time.Minute

// ScopeReferences checks current existence through the owning domain, not a cached ID list.
// ScopesExist answers for all scopes at once so the cost of a resolution does not grow with
// the number of grants: the result is aligned with the input (true means every reference of
// that scope exists). Implementations batch their lookups per scope kind.
type ScopeReferences interface {
	ScopesExist(context.Context, platform.TransactionContext, []authz.Scope) ([]bool, error)
}

// Resolver derives a caller's current authority from proof or from a trusted
// principal ID. Create it with NewResolver; it is safe for concurrent use.
type Resolver struct {
	repository AuthorityRepository
	clock      platform.Clock
	references ScopeReferences
}

// Authentication is the result of Resolve: the caller's current authority
// (with Authority.Key set to any access-key restrictions) plus the credential
// used. CredentialID is nil and CSRFToken empty for anonymous proof.
type Authentication struct {
	Authority    authz.Authority
	CredentialID uuid.UUID
	Kind         ProofKind
	CSRFToken    string
	ExpiresAt    *time.Time
}

// LogValue implements slog.LogValuer and logs only principal, credential and
// proof kind, never the CSRF token.
func (a Authentication) LogValue() slog.Value {
	return slog.GroupValue(slog.String("principalId", a.Authority.PrincipalID),
		slog.String("credentialId", a.CredentialID.String()), slog.String("kind", string(a.Kind)))
}

// NewResolver returns a Resolver. It returns a CodeInvalidInput failure if any
// dependency is nil.
func NewResolver(repository AuthorityRepository, clock platform.Clock, references ScopeReferences) (*Resolver, error) {
	if repository == nil || clock == nil || references == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing identity authority dependency"))
	}
	return &Resolver{repository: repository, clock: clock, references: references}, nil
}

// Resolve derives current authority inside the caller's transaction. It takes
// the shared access (revocation) lock, which is held until the transaction
// ends, so an access-changing command commits either before this operation
// resolved its authority or after the operation completed.
// It returns ErrLoginRequired for an unknown, wrong-kind, revoked, consumed or
// expired credential (sessions also expire 30 days after creation or 7 days
// after last use), an inactive account, or an access key restricted to a
// scope that no longer exists. Grants on scopes that no longer exist are
// dropped. On success it updates the credential's last-used time, but only when the stored
// value is at least touchInterval (5 minutes) old; the 7-day session inactivity rule can
// therefore be off by up to that interval.
func (r *Resolver) Resolve(ctx context.Context, tx platform.TransactionContext, proof Proof) (Authentication, error) {
	if proof.Kind != AnonymousProof && proof.Kind != SessionProof && proof.Kind != AccessKeyProof ||
		(proof.Kind == AnonymousProof && proof.Secret != "") {
		return Authentication{}, ErrLoginRequired
	}
	revision, err := r.repository.LockAccess(ctx, tx)
	if err != nil {
		return Authentication{}, err
	}
	now, err := r.clock.Now(ctx, tx)
	if err != nil {
		return Authentication{}, err
	}
	var account AccountRecord
	var credential CredentialRecord
	if proof.Kind == AnonymousProof {
		account, err = r.repository.Guest(ctx, tx)
	} else {
		hash, hashErr := SecretHash(proof.Secret)
		if hashErr != nil {
			return Authentication{}, hashErr
		}
		credential, err = r.repository.Credential(ctx, tx, hash)
		if err != nil {
			return Authentication{}, err
		}
		if !credentialUsable(proof.Kind, credential, now) {
			return Authentication{}, ErrLoginRequired
		}
		account, err = r.repository.Account(ctx, tx, credential.AccountID)
	}
	if err != nil {
		return Authentication{}, err
	}
	if !accountUsable(proof.Kind, account) {
		return Authentication{}, ErrLoginRequired
	}
	authority, err := r.accountAuthority(ctx, tx, account.ID, revision)
	if err != nil {
		return Authentication{}, err
	}
	if proof.Kind == AccessKeyProof {
		if err := authz.ValidateRestrictions(credential.Restrictions); err != nil {
			return Authentication{}, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		existing, err := r.scopesExist(ctx, tx, credential.Restrictions.Scopes)
		if err != nil {
			return Authentication{}, err
		}
		for _, exists := range existing {
			if !exists {
				return Authentication{}, ErrLoginRequired
			}
		}
	}
	if proof.Kind != AnonymousProof && now.Sub(credential.LastUsedAt) >= touchInterval {
		if err := r.repository.TouchCredential(ctx, tx, credential.ID, now); err != nil {
			return Authentication{}, err
		}
	}
	authority.Key = credential.Restrictions
	return Authentication{
		Authority:    authority,
		CredentialID: credential.ID, Kind: proof.Kind, CSRFToken: credential.CSRFToken,
		ExpiresAt: credential.ExpiresAt,
	}, nil
}

// credentialUsable reports whether a stored credential may authenticate a proof of the given kind at
// now: matching kind, not revoked, consumed or expired, and, for sessions, within the absolute and
// inactivity limits. Resolve and Authenticate share it so the early and the transactional check
// cannot drift apart.
func credentialUsable(kind ProofKind, credential CredentialRecord, now time.Time) bool {
	if credential.Kind != kind || credential.RevokedAt != nil || credential.ConsumedAt != nil ||
		(credential.ExpiresAt != nil && !now.Before(*credential.ExpiresAt)) {
		return false
	}
	return kind != SessionProof || (credential.ExpiresAt != nil &&
		now.Before(credential.CreatedAt.Add(30*24*time.Hour)) &&
		now.Before(credential.LastUsedAt.Add(7*24*time.Hour)))
}

// accountUsable reports whether the account is active and of a kind the proof kind may act as.
func accountUsable(kind ProofKind, account AccountRecord) bool {
	return account.State == "active" && account.ID != uuid.Nil &&
		(kind != SessionProof || account.Kind == "human") &&
		(kind == AnonymousProof) == (account.Kind == "guest")
}

// Authenticate is the cheap early check that a session or access-key proof names an existing,
// usable credential of an active account: a hash lookup of the credential and its account only.
// It takes no access lock, resolves no grants and does not write the credential's last-used time.
// The result is advisory: it lets the HTTP layer refuse an unauthenticated request before reading
// its body, while the authoritative decision (current grants, restrictions, CSRF) is still taken
// by Resolve inside the operation's own transaction, where it stays consistent with the data the
// operation changes. An anonymous proof has no credential to check and yields ErrLoginRequired;
// callers skip those (see Preauthenticator).
func (r *Resolver) Authenticate(ctx context.Context, tx platform.TransactionContext, proof Proof) error {
	if proof.Kind != SessionProof && proof.Kind != AccessKeyProof {
		return ErrLoginRequired
	}
	hash, err := SecretHash(proof.Secret)
	if err != nil {
		return err
	}
	now, err := r.clock.Now(ctx, tx)
	if err != nil {
		return err
	}
	credential, err := r.repository.Credential(ctx, tx, hash)
	if err != nil {
		return err
	}
	if !credentialUsable(proof.Kind, credential, now) {
		return ErrLoginRequired
	}
	account, err := r.repository.Account(ctx, tx, credential.AccountID)
	if err != nil {
		return err
	}
	if !accountUsable(proof.Kind, account) {
		return ErrLoginRequired
	}
	return nil
}

// ResolvePrincipalAuthority returns current owner authority for a trusted internal
// execution binding in the caller's transaction; it provides no transport authentication.
func (r *Resolver) ResolvePrincipalAuthority(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (authz.Authority, error) {
	if id == uuid.Nil {
		return authz.Authority{}, authz.ErrInvalidInput
	}
	revision, err := r.repository.LockAccess(ctx, tx)
	if err != nil {
		return authz.Authority{}, err
	}
	account, err := r.repository.Account(ctx, tx, id)
	if err != nil {
		return authz.Authority{}, err
	}
	if account.ID != id || account.State != "active" ||
		(account.Kind != "human" && account.Kind != "technical" && account.Kind != "guest") {
		return authz.Authority{}, ErrLoginRequired
	}
	return r.accountAuthority(ctx, tx, id, revision)
}

// accountAuthority loads validated current grants for an already verified active account.
func (r *Resolver) accountAuthority(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, revision int64) (authz.Authority, error) {
	grants, err := r.repository.Grants(ctx, tx, id)
	if err != nil {
		return authz.Authority{}, err
	}
	scopes := make([]authz.Scope, len(grants))
	for i, grant := range grants {
		if err := authz.ValidateGrant(grant); err != nil {
			return authz.Authority{}, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		scopes[i] = grant.Scope
	}
	existing, err := r.scopesExist(ctx, tx, scopes)
	if err != nil {
		return authz.Authority{}, err
	}
	active := make([]authz.Grant, 0, len(grants))
	for i, grant := range grants {
		if existing[i] {
			active = append(active, grant)
		}
	}
	fields, err := r.repository.FieldPermissions(ctx, tx, id)
	if err != nil {
		return authz.Authority{}, err
	}
	authority := authz.Authority{PrincipalID: id.String(), Revision: revision, Active: true,
		Grants: active, FieldPermissions: fields}
	if err := authz.AuthorizeFields(authority, nil); err != nil {
		return authz.Authority{}, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	return authority, nil
}

// scopesExist resolves all scopes with one batched owner call and rejects an owner that
// answers for a different number of scopes, which would silently misalign grants.
func (r *Resolver) scopesExist(ctx context.Context, tx platform.TransactionContext, scopes []authz.Scope) ([]bool, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	existing, err := r.references.ScopesExist(ctx, tx, scopes)
	if err != nil {
		return nil, err
	}
	if len(existing) != len(scopes) {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("scope owner answered a different number of scopes"))
	}
	return existing, nil
}
