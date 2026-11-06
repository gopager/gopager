package identitystore

import (
	"context"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/credential"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/loginattempt"
	"github.com/gopager/gopager/internal/platform"
)

// loginAccountingLockKey serializes attempt accounting across all API processes for the
// duration of the caller's transaction without touching the access lock.
const loginAccountingLockKey int64 = 0x47504D4947520002

// TakeLoginAttempt accounts for login attempts across all API processes. It serializes the
// count-and-insert with a transaction advisory lock so that concurrent attempts cannot slip
// past the limits; the instance limit bounds persisted rows, including attempts against
// unknown accounts. The buckets are checked from the widest (instance) to the narrowest
// (account), and a rejected attempt is not stored, so a throttled client cannot extend its
// own wait.
func (s *Store) TakeLoginAttempt(ctx context.Context, tx platform.TransactionContext, keys identity.LoginAttemptKeys, now time.Time, limits identity.LoginRateLimits) (time.Duration, error) {
	if limits.AccountAttempts <= 0 || limits.SourceAttempts <= 0 || limits.InstanceAttempts <= 0 {
		return 0, identity.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	if _, err := executor.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", loginAccountingLockKey); err != nil {
		return 0, err
	}
	client, scope, err := s.client(ctx, tx, "LoginAttempt", entql.FieldLTE("attempted_at", now.Add(-time.Minute)))
	if err != nil {
		return 0, err
	}
	if _, err := client.LoginAttempt.Delete().Exec(scope); err != nil {
		return 0, err
	}
	client, scope, err = s.client(ctx, tx, "LoginAttempt", entql.FieldGT("attempted_at", now.Add(-time.Minute)))
	if err != nil {
		return 0, err
	}
	query := client.LoginAttempt.Query().Order(ent.Asc(loginattempt.FieldAttemptedAt))
	for _, bucket := range []struct {
		query *ent.LoginAttemptQuery
		limit int
	}{
		{query.Clone(), limits.InstanceAttempts},
		{query.Clone().Where(loginattempt.SourceKeyEQ(keys.Source[:])), limits.SourceAttempts},
		{query.Clone().Where(loginattempt.AccountKeyEQ(keys.Account[:])), limits.AccountAttempts},
	} {
		retry, err := bucketRetryAfter(scope, bucket.query, bucket.limit, now)
		if err != nil || retry > 0 {
			return retry, err
		}
	}
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return 0, err
	}
	scope = guard.Bind(ctx, guard.Access{Current: driver.Current, Create: map[string]bool{"LoginAttempt": true}})
	err = ent.NewClient(ent.Driver(driver)).LoginAttempt.Create().
		SetAccountKey(keys.Account[:]).SetSourceKey(keys.Source[:]).SetAttemptedAt(now).Exec(scope)
	return 0, err
}

// bucketRetryAfter returns how long until the oldest attempt in the bucket (ordered by
// time, already restricted to the last minute) leaves the window, or zero while the
// bucket is below its limit.
func bucketRetryAfter(ctx context.Context, bucket *ent.LoginAttemptQuery, limit int, now time.Time) (time.Duration, error) {
	count, err := bucket.Clone().Count(ctx)
	if err != nil || count < limit {
		return 0, err
	}
	first, err := bucket.First(ctx)
	if err != nil {
		return 0, err
	}
	return first.AttemptedAt.Add(time.Minute).Sub(now), nil
}

// LoginAccount returns only password-verification state for an exact normalized username key.
func (s *Store) LoginAccount(ctx context.Context, tx platform.TransactionContext, key string) (identity.LoginAccount, error) {
	client, scope, err := s.client(ctx, tx, "Account", entql.FieldEQ("username_key", key))
	if err != nil {
		return identity.LoginAccount{}, err
	}
	record, err := client.Account.Query().
		Select(account.FieldID, account.FieldKind, account.FieldState, account.FieldPasswordHash).Only(scope)
	if ent.IsNotFound(err) {
		return identity.LoginAccount{}, identity.ErrLoginRequired
	}
	if err != nil {
		return identity.LoginAccount{}, err
	}
	result := identity.LoginAccount{AccountRecord: identity.AccountRecord{
		ID: record.ID, Kind: string(record.Kind), State: string(record.State),
	}}
	if record.PasswordHash != nil {
		result.PasswordHash = *record.PasswordHash
	}
	return result, nil
}

// CreateSession persists the new hash-only Session and its nonsecret audit atomically.
func (s *Store) CreateSession(ctx context.Context, tx platform.TransactionContext, session identity.SessionCreation) error {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return err
	}
	client := ent.NewClient(ent.Driver(driver))
	scope := guard.Bind(ctx, guard.Access{Current: driver.Current,
		Create: map[string]bool{"Credential": true, "AuditRecord": true}})
	if err := client.Credential.Create().SetID(session.ID).SetAccountID(session.AccountID).
		SetKind(credential.KindSession).SetSecretHash(session.SecretHash[:]).SetCsrfToken(session.CSRFToken).
		SetCreatedAt(session.CreatedAt).SetLastUsedAt(session.CreatedAt).SetExpiresAt(session.ExpiresAt).Exec(scope); err != nil {
		return err
	}
	return client.AuditRecord.Create().SetActorID(session.AccountID).SetTargetID(session.ID).
		SetAction("session.created").SetOutcome("accepted").SetRecordedAt(session.CreatedAt).Exec(scope)
}

var _ identity.LoginRepository = (*Store)(nil)

// RevokeSession invalidates exactly the resolved owner's Session, revision and audit together.
func (s *Store) RevokeSession(ctx context.Context, tx platform.TransactionContext, owner, id uuid.UUID, now time.Time) (int64, error) {
	predicate := entql.And(entql.FieldEQ("id", id), entql.FieldEQ("account_id", owner),
		entql.FieldEQ("kind", "session"), entql.FieldNil("revoked_at"))
	client, scope, err := s.client(ctx, tx, "Credential", predicate)
	if err != nil {
		return 0, err
	}
	record, err := client.Credential.Query().Only(scope)
	if ent.IsNotFound(err) {
		return 0, identity.ErrLoginRequired
	}
	if err != nil {
		return 0, err
	}
	updated, err := client.Credential.Update().SetRevokedAt(now).AddRevision(1).Save(scope)
	if err != nil {
		return 0, err
	}
	if updated != 1 {
		return 0, authz.ErrRevisionConflict
	}
	client, scope, err = s.client(ctx, tx, "IdentityControl", entql.FieldEQ("id", 1))
	if err != nil {
		return 0, err
	}
	if _, err := client.IdentityControl.Query().ForUpdate().Only(scope); err != nil {
		return 0, err
	}
	if err := client.IdentityControl.UpdateOneID(1).AddAccessRevision(1).Exec(scope); err != nil {
		return 0, err
	}
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return 0, err
	}
	scope = guard.Bind(ctx, guard.Access{Current: driver.Current, Create: map[string]bool{"AuditRecord": true}})
	if err := ent.NewClient(ent.Driver(driver)).AuditRecord.Create().
		SetActorID(owner).SetTargetID(id).SetAction("session.revoked").SetOutcome("accepted").SetRecordedAt(now).Exec(scope); err != nil {
		return 0, err
	}
	return record.Revision + 1, nil
}
