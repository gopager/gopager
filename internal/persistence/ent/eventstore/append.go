// Package eventstore atomically records classified facts and their transport references.
package eventstore

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

type Store struct{ primary *postgres.Store }

func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("nil event primary"))
	}
	return &Store{primary: primary}, nil
}

// AppendIdentityFact writes only through the caller's active primary transaction.
// Command receipts in the producing domain own operation-level retry deduplication.
func (s *Store) AppendIdentityFact(ctx context.Context, tx platform.TransactionContext, fact events.IdentityFact) (uuid.UUID, error) {
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
	if err := s.publishReference(tx, fact.ID); err != nil {
		return uuid.Nil, err
	}
	return fact.ID, nil
}

var _ events.Appender = (*Store)(nil)

// AppendConfigurationFact keeps the new producer vocabulary separate from identity facts.
func (s *Store) AppendConfigurationFact(ctx context.Context, tx platform.TransactionContext, fact events.ConfigurationFact) (uuid.UUID, error) {
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
	if err := s.publishReference(tx, fact.ID); err != nil {
		return uuid.Nil, err
	}
	return fact.ID, nil
}

var _ events.ConfigurationAppender = (*Store)(nil)

func (s *Store) AppendMasterDataFact(ctx context.Context, tx platform.TransactionContext, fact events.MasterDataFact) (uuid.UUID, error) {
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
	if err := s.publishReference(tx, fact.ID); err != nil {
		return uuid.Nil, err
	}
	return fact.ID, nil
}

var _ events.MasterDataAppender = (*Store)(nil)

func (s *Store) AppendCorrectionFact(ctx context.Context, tx platform.TransactionContext, fact events.CorrectionFact) (uuid.UUID, error) {
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
	if err := s.publishReference(tx, fact.ID); err != nil {
		return uuid.Nil, err
	}
	return fact.ID, nil
}

var _ events.CorrectionAppender = (*Store)(nil)
