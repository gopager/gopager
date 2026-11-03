package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	wmsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// FactTopic is the internal reference stream, not a public subscription API.
const FactTopic = "domain_facts"

// FactReference locates the canonical classified envelope without copying its contents.
type FactReference struct {
	SchemaVersion int       `json:"schemaVersion"`
	DomainEventID uuid.UUID `json:"domainEventId"`
}

// transactionExecutor adapts SQL result interfaces without granting transaction ownership.
type transactionExecutor struct {
	primary    *postgres.Store
	capability platform.TransactionContext
}

// ExecContext executes a publisher statement only while the borrowed capability is live.
func (e transactionExecutor) ExecContext(ctx context.Context, query string, args ...any) (wmsql.Result, error) {
	executor, err := e.primary.Resolve(e.capability)
	if err != nil {
		return nil, err
	}
	return executor.ExecContext(ctx, query, args...)
}

// QueryContext adapts result rows while preserving the same active transaction.
func (e transactionExecutor) QueryContext(ctx context.Context, query string, args ...any) (wmsql.Rows, error) {
	executor, err := e.primary.Resolve(e.capability)
	if err != nil {
		return nil, err
	}
	return executor.QueryContext(ctx, query, args...)
}

// publishReference adds transport work to the caller's canonical fact transaction.
func (s *Store) publishReference(tx platform.TransactionContext, id uuid.UUID) (result error) {
	payload, err := json.Marshal(FactReference{SchemaVersion: 1, DomainEventID: id})
	if err != nil {
		return err
	}
	// Publisher errors are returned; suppress its SQL-argument trace logging.
	publisher, err := wmsql.NewPublisher(transactionExecutor{s.primary, tx}, wmsql.PublisherConfig{
		SchemaAdapter: wmsql.DefaultPostgreSQLSchema{},
	}, watermill.NopLogger{})
	if err != nil {
		return fmt.Errorf("create fact publisher: %w", err)
	}
	defer func() { result = errors.Join(result, publisher.Close()) }()
	msg := message.NewMessage(id.String(), payload)
	msg.SetContext(tx.Context())
	if err := publisher.Publish(FactTopic, msg); err != nil {
		return fmt.Errorf("publish fact reference: %w", err)
	}
	return nil
}

var _ wmsql.ContextExecutor = transactionExecutor{}
