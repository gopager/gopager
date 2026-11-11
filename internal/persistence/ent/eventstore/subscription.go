package eventstore

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
)

var _ events.SubscriptionAppender = (*Store)(nil)

// AppendSubscriptionFact implements events.SubscriptionAppender in the caller's transaction.
func (s *Store) AppendSubscriptionFact(ctx context.Context, tx platform.TransactionContext, fact events.SubscriptionFact) (uuid.UUID, error) {
	return appendEnvelope(ctx, s, tx, fact.ID, fact.Envelope)
}

// AppendSubscriptionHealthFact implements events.SubscriptionAppender in the caller's transaction.
func (s *Store) AppendSubscriptionHealthFact(ctx context.Context, tx platform.TransactionContext, fact events.SubscriptionHealthFact) (uuid.UUID, error) {
	return appendEnvelope(ctx, s, tx, fact.ID, fact.Envelope)
}

// appendEnvelope builds the envelope stamped with the primary clock and records it as fact id.
func appendEnvelope[E any](ctx context.Context, s *Store, tx platform.TransactionContext, id uuid.UUID,
	build func(time.Time) (E, error)) (uuid.UUID, error) {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return uuid.Nil, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return uuid.Nil, err
	}
	envelope, err := build(now)
	if err != nil {
		return uuid.Nil, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return uuid.Nil, err
	}
	client := ent.NewClient(ent.Driver(driver))
	scope := guard.Bind(ctx, guard.Access{Create: map[string]bool{"DomainEvent": true}, Current: driver.Current})
	if err := client.DomainEvent.Create().SetID(id).SetEnvelope(encoded).Exec(scope); err != nil {
		return uuid.Nil, err
	}
	if err := s.notify(ctx, tx, id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
