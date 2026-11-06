package identitystore

import (
	"context"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
)

// RecordConfigurationChange advances the identity owner's shared access revision and writes
// the safe audit record for a configuration change; the caller commits its domain change
// and fact in this same transaction. The plain update waits only for other writers of the
// control row, never for in-flight authority readers, so concurrent configuration changes
// do not conflict with each other.
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
	if err := client.IdentityControl.UpdateOneID(1).AddAccessRevision(1).Exec(scope); err != nil {
		return err
	}
	return client.AuditRecord.Create().SetActorID(actor).SetTargetID(target).SetAction(action).
		SetOutcome("completed").SetRecordedAt(now).Exec(scope)
}
