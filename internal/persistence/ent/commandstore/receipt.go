// Package commandstore implements compact administrative receipts in the borrowed transaction.
package commandstore

import (
	"context"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

type Store struct{ primary *postgres.Store }

// New binds receipt participation to the existing Primary, without an extra pool.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Store{primary: primary}, nil
}

// ReadCommandReceipt uses exactly the resolved actor/command pair, never a global command ID.
func (s *Store) ReadCommandReceipt(ctx context.Context, tx platform.TransactionContext, actor, command uuid.UUID) (commands.Receipt, bool, error) {
	if actor == uuid.Nil || command == uuid.Nil {
		return commands.Receipt{}, false, authz.ErrInvalidInput
	}
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return commands.Receipt{}, false, err
	}
	client := ent.NewClient(ent.Driver(driver))
	scope := guard.Bind(ctx, guard.Access{Current: driver.Current, Predicates: map[string]entql.P{
		"CommandReceipt": entql.And(entql.FieldEQ("actor_id", actor), entql.FieldEQ("command_id", command)),
	}})
	row, err := client.CommandReceipt.Query().Only(scope)
	if ent.IsNotFound(err) {
		return commands.Receipt{}, false, nil
	}
	if err != nil {
		return commands.Receipt{}, false, err
	}
	if len(row.Fingerprint) != 32 {
		return commands.Receipt{}, false, platform.NewFailure(platform.CodeAuthorityUnavailable, authz.ErrInvalidInput)
	}
	var hash [32]byte
	copy(hash[:], row.Fingerprint)
	return commands.Receipt{ActorID: row.ActorID, CommandID: row.CommandID, Operation: row.Operation,
		Fingerprint: hash, Result: commands.Result{ID: row.ResultID, Revision: row.ResultRevision, Items: row.ResultItems},
		RecordedAt: row.RecordedAt}, true, nil
}

// SaveCommandReceipt commits only the nonsecret result with the command's state and fact.
func (s *Store) SaveCommandReceipt(ctx context.Context, tx platform.TransactionContext, receipt commands.Receipt) error {
	if receipt.ActorID == uuid.Nil || receipt.CommandID == uuid.Nil || receipt.Operation == "" ||
		receipt.Result.ID == uuid.Nil || receipt.Result.Revision <= 0 || receipt.RecordedAt.IsZero() {
		return authz.ErrInvalidInput
	}
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return err
	}
	client := ent.NewClient(ent.Driver(driver))
	scope := guard.Bind(ctx, guard.Access{Current: driver.Current, Create: map[string]bool{"CommandReceipt": true}})
	return client.CommandReceipt.Create().SetActorID(receipt.ActorID).SetCommandID(receipt.CommandID).
		SetOperation(receipt.Operation).SetFingerprint(receipt.Fingerprint[:]).
		SetResultID(receipt.Result.ID).SetResultRevision(receipt.Result.Revision).SetResultItems(receipt.Result.Items).
		SetRecordedAt(receipt.RecordedAt).Exec(scope)
}

var _ commands.Repository = (*Store)(nil)
