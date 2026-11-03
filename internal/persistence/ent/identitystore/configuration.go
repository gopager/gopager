package identitystore

import (
	"context"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
)

// RecordConfigurationChange advances the identity owner's shared revision and safe audit.
// The caller holds LockAccess and commits its domain change/fact in this same transaction.
func (s *Store) RecordConfigurationChange(ctx context.Context, tx platform.TransactionContext, actor, target uuid.UUID, action string, now time.Time) error {
	if actor == uuid.Nil || target == uuid.Nil || !authz.ValidAction(action) || now.IsZero() {
		return authz.ErrInvalidInput
	}
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return err
	}
	client := ent.NewClient(ent.Driver(driver))
	scope := guard.Bind(ctx, guard.Access{Current: driver.Current,
		Predicates: map[string]entql.P{"IdentityControl": entql.FieldEQ("id", 1)},
		Create:     map[string]bool{"AuditRecord": true},
	})
	control, err := client.IdentityControl.Query().Only(scope)
	if err != nil {
		return err
	}
	next, err := commands.NextRevision(control.AccessRevision, control.AccessRevision)
	if err != nil {
		return err
	}
	if _, err := client.IdentityControl.UpdateOneID(1).SetAccessRevision(next).Save(scope); err != nil {
		return err
	}
	return client.AuditRecord.Create().SetActorID(actor).SetTargetID(target).SetAction(action).
		SetOutcome("completed").SetRecordedAt(now).Exec(scope)
}
