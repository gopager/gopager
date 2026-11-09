// Package readstore implements the PostgreSQL side of the Message read model: eligibility,
// filters, ordering and paging run in one statement per request, so a page and its cutoff
// position share a snapshot.
package readstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/readmodel"
)

// Store reads Messages through raw SQL in the caller's active primary transaction. It holds no
// transaction of its own.
type Store struct{ primary *postgres.Store }

// New binds the store to the primary store whose transactions it joins.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Store{primary: primary}, nil
}

var (
	_ readmodel.Store     = (*Store)(nil)
	_ readmodel.TokenKeys = (*Store)(nil)
)

// TokenKey returns the page token secret, creating it from the system random source when the
// installation has none yet.
func (s *Store) TokenKey(ctx context.Context, tx platform.TransactionContext) ([]byte, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	read := func() ([]byte, error) {
		var secret []byte
		err := executor.QueryRowContext(ctx, `SELECT secret FROM query_token_keys WHERE id = 1`).Scan(&secret)
		return secret, err
	}
	secret, err := read()
	if err == nil {
		return secret, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		return nil, platform.NewFailure(platform.CodeInternalError, err)
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO query_token_keys (id, secret) VALUES (1, $1) ON CONFLICT DO NOTHING`, fresh); err != nil {
		return nil, err
	}
	return read()
}

// WorkflowVersions lists every Workflow definition version ID.
func (s *Store) WorkflowVersions(ctx context.Context, tx platform.TransactionContext) ([]uuid.UUID, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id FROM processing_workflow_versions`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// VersionDefinition returns the stored definition document of a Workflow definition version, or
// authz.ErrNotFound. Definitions are immutable.
func (s *Store) VersionDefinition(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) ([]byte, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	var definition []byte
	err = executor.QueryRowContext(ctx, `SELECT definition FROM processing_workflow_versions WHERE id = $1`, id).Scan(&definition)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, authz.ErrNotFound
	}
	return definition, err
}
