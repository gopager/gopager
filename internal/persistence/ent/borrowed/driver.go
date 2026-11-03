// Package borrowed lends an existing platform transaction to Ent without ownership.
package borrowed

import (
	"context"
	"errors"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/privacy"

	_ "github.com/gopager/gopager/internal/persistence/ent/runtime"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

type Driver struct {
	store      *postgres.Store
	capability platform.TransactionContext
}

// New validates that the capability belongs to the store and is still active.
func New(store *postgres.Store, capability platform.TransactionContext) (*Driver, error) {
	if store == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("nil transaction store"))
	}
	if _, err := store.Resolve(capability); err != nil {
		return nil, err
	}
	return &Driver{store: store, capability: capability}, nil
}

// Current checks transaction lifetime instead of trusting a retained Ent client.
func (d *Driver) Current() bool {
	_, err := d.store.Resolve(d.capability)
	return err == nil
}

func (d *Driver) connection(ctx context.Context) (entsql.Conn, error) {
	if _, present := privacy.DecisionFromContext(ctx); present {
		return entsql.Conn{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("privacy decision overrides are prohibited"))
	}
	executor, err := d.store.Resolve(d.capability)
	if err != nil {
		return entsql.Conn{}, err
	}
	return entsql.Conn{ExecQuerier: executor}, nil
}

func (d *Driver) Exec(ctx context.Context, query string, args, result any) error {
	connection, err := d.connection(ctx)
	if err != nil {
		return err
	}
	return connection.Exec(ctx, query, args, result)
}

func (d *Driver) Query(ctx context.Context, query string, args, result any) error {
	connection, err := d.connection(ctx)
	if err != nil {
		return err
	}
	return connection.Query(ctx, query, args, result)
}

func (d *Driver) Dialect() string { return dialect.Postgres }

// Tx lets Ent compose its statements without opening or completing another transaction.
func (d *Driver) Tx(ctx context.Context) (dialect.Tx, error) {
	if _, err := d.connection(ctx); err != nil {
		return nil, err
	}
	return dialect.NopTx(d), nil
}

// Close rejects ownership transfer; only the platform completes its transaction/pool.
func (d *Driver) Close() error {
	return platform.NewFailure(platform.CodeInvalidInput, errors.New("borrowed transaction cannot close the platform store"))
}
