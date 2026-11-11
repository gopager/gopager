// Package deliverystore implements the PostgreSQL side of Subscriptions and Deliveries with
// direct SQL in the caller's primary transaction: Subscription rows and targets, Delivery jobs
// with fenced claims, attempts, sending admission windows and the read helpers Delivery needs.
package deliverystore

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

// Store holds no transaction of its own; every method joins the caller's transaction.
type Store struct{ primary *postgres.Store }

// New binds the store to the primary store whose transactions it joins.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Store{primary: primary}, nil
}

var (
	_ delivery.Store = (*Store)(nil)
)

// executor resolves the caller's transaction.
func (s *Store) executor(tx platform.TransactionContext) (postgres.Executor, error) {
	return s.primary.Resolve(tx)
}

// uuidList decodes a JSON array of UUIDs produced by array_to_json.
func uuidList(encoded string) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	if encoded == "" {
		return ids, nil
	}
	if err := json.Unmarshal([]byte(encoded), &ids); err != nil {
		return nil, platform.NewFailure(platform.CodeInternalError, err)
	}
	return ids, nil
}

// referenceGone maps a foreign-key violation to authz.ErrInvalidInput: a layout or instance a
// Subscription names was removed by a concurrent change after it was checked.
func referenceGone(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23503" {
		return authz.ErrInvalidInput
	}
	return err
}

// notFound maps a missing row to authz.ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return authz.ErrNotFound
	}
	return err
}
