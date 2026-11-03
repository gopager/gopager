package identitystore

import (
	"context"

	"entgo.io/ent/entql"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/fieldpermission"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
)

// InitializeIdentity serializes the one-time local bootstrap on the existing control row.
func (s *Store) InitializeIdentity(ctx context.Context, tx platform.TransactionContext, input identity.BootstrapAccount) error {
	client, scope, err := s.client(ctx, tx, "IdentityControl", entql.FieldEQ("id", 1))
	if err != nil {
		return err
	}
	control, err := client.IdentityControl.Query().ForUpdate().Only(scope)
	if err != nil {
		return err
	}
	if control.Bootstrapped {
		return authz.ErrRevisionConflict
	}
	client, accounts, err := s.client(ctx, tx, "Account", entql.FieldNotNil("id"))
	if err != nil {
		return err
	}
	exists, err := client.Account.Query().Exist(accounts)
	if err != nil {
		return err
	}
	if exists {
		return authz.ErrRevisionConflict
	}
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return err
	}
	client = ent.NewClient(ent.Driver(driver))
	create := guard.Bind(ctx, guard.Access{Current: driver.Current,
		Create: map[string]bool{"Account": true, "Grant": true, "FieldPermission": true, "AuditRecord": true}})
	if err := client.Account.Create().SetID(input.ID).SetKind(account.KindHuman).
		SetUsername(input.Username).SetUsernameKey(input.UsernameKey).SetPasswordHash(input.PasswordHash).
		SetCreatedAt(input.CreatedAt).Exec(create); err != nil {
		return err
	}
	if err := client.Account.Create().SetID(input.GuestID).SetKind(account.KindGuest).
		SetCreatedAt(input.CreatedAt).Exec(create); err != nil {
		return err
	}
	if err := client.Grant.Create().SetSubjectID(input.ID).SetSubjectKind(grant.SubjectKindPrincipal).
		SetActions(input.Permissions.Actions).SetScope(authz.Scope{Kind: "all"}).
		SetDelegable(input.Permissions.Delegable).Exec(create); err != nil {
		return err
	}
	if len(input.Permissions.Fields) > 0 {
		if err := client.FieldPermission.Create().SetSubjectID(input.ID).
			SetSubjectKind(fieldpermission.SubjectKindPrincipal).SetFieldNames(input.Permissions.Fields).
			SetDelegable(input.Permissions.Delegable).Exec(create); err != nil {
			return err
		}
	}
	if err := client.AuditRecord.Create().SetActorID(input.ID).SetTargetID(input.ID).
		SetAction("identity.bootstrap").SetOutcome("accepted").SetRecordedAt(input.CreatedAt).Exec(create); err != nil {
		return err
	}
	return client.IdentityControl.UpdateOneID(1).SetBootstrapped(true).AddAccessRevision(1).Exec(scope)
}

var _ identity.BootstrapRepository = (*Store)(nil)
