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

func (s *Store) appendProcessing(ctx context.Context, tx platform.TransactionContext, makeEnvelope func() (events.ProcessingEnvelope, error)) (uuid.UUID, error) {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return uuid.Nil, err
	}
	envelope, err := makeEnvelope()
	if err != nil {
		return uuid.Nil, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return uuid.Nil, err
	}
	client := ent.NewClient(ent.Driver(driver))
	scope := guard.Bind(ctx, guard.Access{Create: map[string]bool{"DomainEvent": true}, Current: driver.Current})
	if err := client.DomainEvent.Create().SetID(envelope.DomainEventID).SetEnvelope(encoded).Exec(scope); err != nil {
		return uuid.Nil, err
	}
	if err := s.publishReference(tx, envelope.DomainEventID); err != nil {
		return uuid.Nil, err
	}
	return envelope.DomainEventID, nil
}

// AppendProcessingConfiguration records a Workflow configuration change fact, stamped with primary
// time, in the caller's transaction without widening older producer vocabularies.
func (s *Store) AppendProcessingConfiguration(ctx context.Context, tx platform.TransactionContext, fact events.ProcessingConfigurationFact) (uuid.UUID, error) {
	return s.appendProcessing(ctx, tx, func() (events.ProcessingEnvelope, error) {
		now, err := s.primary.Now(ctx, tx)
		if err != nil {
			return events.ProcessingEnvelope{}, err
		}
		return fact.Envelope(now)
	})
}

// AppendProcessingCompletion participates in source publication, not private result storage.
func (s *Store) AppendProcessingCompletion(ctx context.Context, tx platform.TransactionContext, fact events.ProcessingCompletionFact) (uuid.UUID, error) {
	return s.appendProcessing(ctx, tx, func() (events.ProcessingEnvelope, error) {
		now, err := s.primary.Now(ctx, tx)
		if err != nil {
			return events.ProcessingEnvelope{}, err
		}
		return fact.Envelope(now)
	})
}

var _ events.ProcessingAppender = (*Store)(nil)

// AppendProcessingInstance records a nonsecret administrative pure-processing instance change.
func (s *Store) AppendProcessingInstance(ctx context.Context, tx platform.TransactionContext, fact events.ProcessingInstanceFact) (uuid.UUID, error) {
	return s.appendProcessing(ctx, tx, func() (events.ProcessingEnvelope, error) {
		now, err := s.primary.Now(ctx, tx)
		if err != nil {
			return events.ProcessingEnvelope{}, err
		}
		return fact.Envelope(now)
	})
}

var _ events.ProcessingInstanceAppender = (*Store)(nil)
