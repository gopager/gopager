// Package identitystore implements identity ports using transaction-bound Ent clients.
package identitystore

import (
	"context"
	"errors"
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
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

type Store struct{ primary *postgres.Store }

func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("nil identity primary"))
	}
	return &Store{primary: primary}, nil
}

func (s *Store) client(ctx context.Context, tx platform.TransactionContext, entity string, predicate entql.P) (*ent.Client, context.Context, error) {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return nil, nil, err
	}
	scope := guard.Bind(ctx, guard.Access{
		Predicates: map[string]entql.P{entity: predicate}, Current: driver.Current,
	})
	return ent.NewClient(ent.Driver(driver)), scope, nil
}

// LockAccess serializes private authority decisions with all access-changing commands.
func (s *Store) LockAccess(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	client, scope, err := s.client(ctx, tx, "IdentityControl", entql.FieldEQ("id", 1))
	if err != nil {
		return 0, err
	}
	control, err := client.IdentityControl.Query().ForUpdate().Only(scope)
	if err != nil {
		return 0, err
	}
	return control.AccessRevision, nil
}

// Account loads only the account identified by an already resolved credential.
func (s *Store) Account(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (identity.AccountRecord, error) {
	client, scope, err := s.client(ctx, tx, "Account", entql.FieldEQ("id", id))
	if err != nil {
		return identity.AccountRecord{}, err
	}
	record, err := client.Account.Query().Select(account.FieldID, account.FieldKind, account.FieldState).Only(scope)
	if ent.IsNotFound(err) {
		return identity.AccountRecord{}, identity.ErrLoginRequired
	}
	if err != nil {
		return identity.AccountRecord{}, err
	}
	return identity.AccountRecord{ID: record.ID, Kind: string(record.Kind), State: string(record.State)}, nil
}

// Guest resolves the persisted undeletable subject, never a substitute for a bad proof.
func (s *Store) Guest(ctx context.Context, tx platform.TransactionContext) (identity.AccountRecord, error) {
	client, scope, err := s.client(ctx, tx, "Account", entql.FieldEQ("kind", "guest"))
	if err != nil {
		return identity.AccountRecord{}, err
	}
	record, err := client.Account.Query().Select(account.FieldID, account.FieldKind, account.FieldState).Only(scope)
	if ent.IsNotFound(err) {
		return identity.AccountRecord{}, identity.ErrLoginRequired
	}
	if err != nil {
		return identity.AccountRecord{}, err
	}
	return identity.AccountRecord{ID: record.ID, Kind: string(record.Kind), State: string(record.State)}, nil
}

// Credential uses an exact storage hash and never returns that hash or a raw entity.
func (s *Store) Credential(ctx context.Context, tx platform.TransactionContext, hash [32]byte) (identity.CredentialRecord, error) {
	client, scope, err := s.client(ctx, tx, "Credential", entql.FieldEQ(credential.FieldSecretHash, hash[:]))
	if err != nil {
		return identity.CredentialRecord{}, err
	}
	record, err := client.Credential.Query().Only(scope)
	if ent.IsNotFound(err) {
		return identity.CredentialRecord{}, identity.ErrLoginRequired
	}
	if err != nil {
		return identity.CredentialRecord{}, err
	}
	csrf := ""
	if record.CsrfToken != nil {
		csrf = *record.CsrfToken
	}
	return identity.CredentialRecord{
		ID: record.ID, AccountID: record.AccountID, Kind: identity.ProofKind(record.Kind),
		CreatedAt: record.CreatedAt, LastUsedAt: record.LastUsedAt, ExpiresAt: record.ExpiresAt,
		RevokedAt: record.RevokedAt, ConsumedAt: record.ConsumedAt, CSRFToken: csrf, Restrictions: record.Restrictions,
	}, nil
}

// subjectPredicate resolves direct and current flat-group authority for one account.
func (s *Store) subjectPredicate(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (entql.P, error) {
	client, scope, err := s.client(ctx, tx, "Membership", entql.FieldEQ("account_id", id))
	if err != nil {
		return nil, err
	}
	memberships, err := client.Membership.Query().All(scope)
	if err != nil {
		return nil, err
	}
	predicate := entql.And(entql.FieldEQ("subject_kind", "principal"), entql.FieldEQ("subject_id", id))
	if len(memberships) > 0 {
		ids := make([]any, len(memberships))
		for i, membership := range memberships {
			ids[i] = membership.GroupID
		}
		predicate = entql.Or(predicate, entql.And(entql.FieldEQ("subject_kind", "userGroup"), entql.FieldIn("subject_id", ids...)))
	}
	return predicate, nil
}

// Grants loads additive direct and flat-group grants for exactly one server-derived subject.
func (s *Store) Grants(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) ([]authz.Grant, error) {
	predicate, err := s.subjectPredicate(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	client, scope, err := s.client(ctx, tx, "Grant", predicate)
	if err != nil {
		return nil, err
	}
	records, err := client.Grant.Query().All(scope)
	if err != nil {
		return nil, err
	}
	grants := make([]authz.Grant, len(records))
	for i, record := range records {
		grants[i] = authz.Grant{
			ID: record.ID.String(), SubjectID: record.SubjectID.String(), SubjectKind: string(record.SubjectKind),
			Actions: record.Actions, Scope: record.Scope,
			MaxAgeSeconds: record.MaxAgeSeconds, Delegable: record.Delegable,
		}
	}
	return grants, nil
}

// FieldPermissions loads only global direct and current flat-group field records.
func (s *Store) FieldPermissions(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) ([]authz.FieldPermission, error) {
	predicate, err := s.subjectPredicate(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	client, scope, err := s.client(ctx, tx, "FieldPermission", predicate)
	if err != nil {
		return nil, err
	}
	records, err := client.FieldPermission.Query().All(scope)
	if err != nil {
		return nil, err
	}
	permissions := make([]authz.FieldPermission, len(records))
	for i, record := range records {
		permissions[i] = authz.FieldPermission{ID: record.ID.String(), Revision: record.Revision,
			SubjectID: record.SubjectID.String(), SubjectKind: string(record.SubjectKind),
			Fields: record.FieldNames, Delegable: record.Delegable}
	}
	return permissions, nil
}

// TouchCredential updates only the already resolved credential's safe last-use timestamp.
func (s *Store) TouchCredential(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, now time.Time) error {
	client, scope, err := s.client(ctx, tx, "Credential", entql.And(
		entql.FieldEQ("id", id), entql.FieldNil("revoked_at"), entql.FieldNil("consumed_at")))
	if err != nil {
		return err
	}
	updated, err := client.Credential.Update().SetLastUsedAt(now).Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return identity.ErrLoginRequired
	}
	return nil
}

// ScopeExists supplies the identity owner's references; other domain kinds require composition.
// An unwired owner is an explicit authority failure, not an unrestricted or empty substitute.
func (s *Store) ScopeExists(ctx context.Context, tx platform.TransactionContext, target authz.Scope) (bool, error) {
	if !authz.ValidScope(target) {
		return false, authz.ErrInvalidInput
	}
	if target.Kind == "all" {
		return true, nil
	}
	if target.Kind != "account" {
		return false, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("scope owner is not wired"))
	}
	for _, reference := range target.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return false, authz.ErrInvalidInput
		}
		client, scope, err := s.client(ctx, tx, "Account", entql.FieldEQ("id", id))
		if err != nil {
			return false, err
		}
		exists, err := client.Account.Query().
			Where(account.StateNotIn(account.StateDeleting, account.StateDeleted)).Exist(scope)
		if err != nil || !exists {
			return false, err
		}
	}
	return true, nil
}

var _ identity.AuthorityRepository = (*Store)(nil)
var _ identity.ScopeReferences = (*Store)(nil)
