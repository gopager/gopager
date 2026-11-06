package eventstore

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
)

// AppendIntakeFact records a typed ingest intake state change as a DomainEvent and announces it with
// NOTIFY, both in the caller's transaction. It returns the fact ID.
func (s *Store) AppendIntakeFact(ctx context.Context, tx platform.TransactionContext, fact events.IntakeFact) (uuid.UUID, error) {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return uuid.Nil, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return uuid.Nil, err
	}
	envelope, err := fact.Envelope(now)
	if err != nil {
		return uuid.Nil, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return uuid.Nil, err
	}
	client := ent.NewClient(ent.Driver(driver))
	scope := guard.Bind(ctx, guard.Access{Create: map[string]bool{"DomainEvent": true}, Current: driver.Current})
	if err := client.DomainEvent.Create().SetID(fact.ID).SetEnvelope(encoded).Exec(scope); err != nil {
		return uuid.Nil, err
	}
	if err := s.notify(ctx, tx, fact.ID); err != nil {
		return uuid.Nil, err
	}
	return fact.ID, nil
}

var _ events.IntakeAppender = (*Store)(nil)
