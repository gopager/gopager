package readstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

var _ readmodel.SavedViewStore = (*Store)(nil)

const savedViewColumns = `id, revision, ownership, owner_principal_id, name, definition, created_at, updated_at`

// scanSavedView reads one saved_views row selected with savedViewColumns.
func scanSavedView(row interface{ Scan(...any) error }) (readmodel.SavedViewRecord, error) {
	var record readmodel.SavedViewRecord
	var owner uuid.NullUUID
	err := row.Scan(&record.ID, &record.Revision, &record.Ownership, &owner, &record.Name, &record.Definition,
		&record.CreatedAt, &record.UpdatedAt)
	if owner.Valid {
		record.OwnerPrincipalID = &owner.UUID
	}
	return record, err
}

// InsertSavedView implements readmodel.SavedViewStore.
func (s *Store) InsertSavedView(ctx context.Context, tx platform.TransactionContext, record readmodel.SavedViewRecord) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO saved_views (`+savedViewColumns+`) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)`,
		record.ID, record.Revision, record.Ownership, record.OwnerPrincipalID, record.Name, record.Definition,
		record.CreatedAt, record.UpdatedAt)
	return err
}

// SavedView implements readmodel.SavedViewStore.
func (s *Store) SavedView(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (readmodel.SavedViewRecord, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return readmodel.SavedViewRecord{}, err
	}
	record, err := scanSavedView(executor.QueryRowContext(ctx, `SELECT `+savedViewColumns+` FROM saved_views WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return readmodel.SavedViewRecord{}, authz.ErrNotFound
	}
	return record, err
}

// ListSavedViews implements readmodel.SavedViewStore.
func (s *Store) ListSavedViews(ctx context.Context, tx platform.TransactionContext, query readmodel.SavedViewListQuery) ([]readmodel.SavedViewRecord, int, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, 0, err
	}
	const selection = ` FROM saved_views WHERE (ownership = 'personal' AND owner_principal_id = $1) OR (ownership = 'global' AND $2)`
	owner := uuid.NullUUID{UUID: query.Owner, Valid: query.Owner != uuid.Nil}
	var total int
	if err := executor.QueryRowContext(ctx, `SELECT count(*)`+selection, owner, query.Global).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT `+savedViewColumns+selection+` ORDER BY name, id OFFSET $3 LIMIT $4`,
		owner, query.Global, query.Offset, query.Limit)
	if err != nil {
		return nil, 0, err
	}
	var records []readmodel.SavedViewRecord
	for rows.Next() {
		record, err := scanSavedView(rows)
		if err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		records = append(records, record)
	}
	return records, total, closeRows(rows)
}

// UpdateSavedView implements readmodel.SavedViewStore.
func (s *Store) UpdateSavedView(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64,
	name string, definition []byte, now time.Time) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `UPDATE saved_views SET name = $3, definition = $4::jsonb, revision = revision + 1,
		updated_at = $5 WHERE id = $1 AND revision = $2`, id, expected, name, definition, now)
	return requireOneRow(result, err)
}

// DeleteSavedView implements readmodel.SavedViewStore.
func (s *Store) DeleteSavedView(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `DELETE FROM saved_views WHERE id = $1 AND revision = $2`, id, expected)
	return requireOneRow(result, err)
}

// requireOneRow reports a statement that matched no row as a revision conflict: the view
// changed or vanished between the read and the write.
func requireOneRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}
