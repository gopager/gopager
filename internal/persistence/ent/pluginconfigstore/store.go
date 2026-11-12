// Package pluginconfigstore implements the PostgreSQL side of plugin configuration with direct SQL
// in the caller's primary transaction: encrypted Secret rows, personal configurations and per-Unit
// input values of Delivery instances.
package pluginconfigstore

import (
	"database/sql"
	"errors"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
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
	_ processing.SecretStore            = (*Store)(nil)
	_ processing.PersonalConfigurations = (*Store)(nil)
)

// executor resolves the caller's transaction.
func (s *Store) executor(tx platform.TransactionContext) (postgres.Executor, error) {
	return s.primary.Resolve(tx)
}

// referenceGone maps a foreign-key violation to authz.ErrNotFound: the instance, account or Unit
// was removed after it was checked.
func referenceGone(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23503" {
		return authz.ErrNotFound
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
