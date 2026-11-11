// Package layoutstore implements the PostgreSQL side of persisted layouts in raw SQL, inside the
// caller's primary transaction.
package layoutstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/agencyscope"
	"github.com/gopager/gopager/internal/persistence/ent/attributestore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// Store reads and writes layouts. It holds no transaction of its own.
type Store struct {
	primary *postgres.Store
	catalog *attributestore.Store
}

var _ presentation.Store = (*Store)(nil)

// New binds the store to the primary store whose transactions it joins.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	catalog, err := attributestore.New(primary)
	if err != nil {
		return nil, err
	}
	return &Store{primary: primary, catalog: catalog}, nil
}

// currentSelection selects a layout joined with its current version.
const currentSelection = `SELECT l.id, l.revision, l.name, l.output_kind, v.definition, l.created_by, l.created_at, l.updated_at,
	(SELECT COALESCE(json_agg(a.agency_id ORDER BY a.agency_id), '[]'::json) FROM layout_definition_agencies a WHERE a.layout_id = l.id)::text
	FROM layout_definitions l JOIN layout_definition_versions v ON v.layout_id = l.id AND v.version = l.revision`

func scan(row interface{ Scan(...any) error }) (presentation.Record, error) {
	var record presentation.Record
	var agencies string
	if err := row.Scan(&record.ID, &record.Revision, &record.Name, &record.OutputKind, &record.Definition,
		&record.CreatedBy, &record.CreatedAt, &record.UpdatedAt, &agencies); err != nil {
		return record, err
	}
	return record, json.Unmarshal([]byte(agencies), &record.Agencies)
}

// Insert implements presentation.Store.
func (s *Store) Insert(ctx context.Context, tx platform.TransactionContext, record presentation.Record) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, record.ID, record.Revision, record.Name, record.OutputKind,
		record.CreatedBy, record.CreatedAt, record.UpdatedAt); err != nil {
		return err
	}
	if err := insertAgencies(ctx, executor, record.ID, record.Agencies); err != nil {
		return err
	}
	return insertVersion(ctx, executor, record.ID, record.Revision, record.Definition, record.CreatedBy, record.UpdatedAt)
}

// insertAgencies assigns the layout to its Agencies; an unknown Agency is authz.ErrInvalidInput.
func insertAgencies(ctx context.Context, executor execer, id uuid.UUID, agencies []uuid.UUID) error {
	for _, agency := range agencies {
		if _, err := executor.ExecContext(ctx, `INSERT INTO layout_definition_agencies (layout_id, agency_id) VALUES ($1, $2)`,
			id, agency); err != nil {
			return agencyscope.InvalidAgency(err)
		}
	}
	return nil
}

// execer is the statement runner of a resolved transaction.
type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertVersion(ctx context.Context, executor execer, id uuid.UUID, version int64, definition []byte,
	by uuid.UUID, now time.Time) error {
	_, err := executor.ExecContext(ctx, `INSERT INTO layout_definition_versions (layout_id, version, definition, created_by, created_at)
		VALUES ($1, $2, $3::jsonb, $4, $5)`, id, version, definition, by, now)
	return err
}

// Get implements presentation.Store.
func (s *Store) Get(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (presentation.Record, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return presentation.Record{}, err
	}
	record, err := scan(executor.QueryRowContext(ctx, currentSelection+` WHERE l.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return presentation.Record{}, authz.ErrNotFound
	}
	return record, err
}

// visibleLayouts is the condition that one of the layout's Agencies is visible; the arguments
// are extended by the visibility's placeholders.
func visibleLayouts(visibility authz.AgencyVisibility, args []any) (string, []any) {
	condition, args := agencyscope.Condition(visibility, "a.agency_id", args)
	return `EXISTS (SELECT 1 FROM layout_definition_agencies a WHERE a.layout_id = l.id AND ` + condition + `)`, args
}

// Visible implements presentation.Store.
func (s *Store) Visible(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, visibility authz.AgencyVisibility) (bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	condition, args := visibleLayouts(visibility, []any{id})
	var visible bool
	err = executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM layout_definitions l WHERE l.id = $1 AND `+condition+`)`,
		args...).Scan(&visible)
	return visible, err
}

// List implements presentation.Store.
func (s *Store) List(ctx context.Context, tx platform.TransactionContext, visibility authz.AgencyVisibility, offset, limit int) ([]presentation.Record, int, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, 0, err
	}
	condition, args := visibleLayouts(visibility, nil)
	var total int
	if err := executor.QueryRowContext(ctx, `SELECT count(*) FROM layout_definitions l WHERE `+condition, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, offset, limit)
	rows, err := executor.QueryContext(ctx, fmt.Sprintf(currentSelection+` WHERE %s ORDER BY l.name, l.id OFFSET $%d LIMIT $%d`,
		condition, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var records []presentation.Record
	for rows.Next() {
		record, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, record)
	}
	return records, total, rows.Err()
}

// Version implements presentation.Store.
func (s *Store) Version(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, version int64) (presentation.VersionRecord, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return presentation.VersionRecord{}, err
	}
	record := presentation.VersionRecord{LayoutID: id, Version: version}
	err = executor.QueryRowContext(ctx, `SELECT definition, created_at FROM layout_definition_versions
		WHERE layout_id = $1 AND version = $2`, id, version).Scan(&record.Definition, &record.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return presentation.VersionRecord{}, authz.ErrNotFound
	}
	return record, err
}

// Update implements presentation.Store.
func (s *Store) Update(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64,
	name string, agencies []uuid.UUID, definition []byte, by uuid.UUID, now time.Time) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `UPDATE layout_definitions SET name = $3, revision = revision + 1, updated_at = $4
		WHERE id = $1 AND revision = $2`, id, expected, name, now)
	if err := requireOneRow(result, err); err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM layout_definition_agencies WHERE layout_id = $1`, id); err != nil {
		return err
	}
	if err := insertAgencies(ctx, executor, id, agencies); err != nil {
		return err
	}
	return insertVersion(ctx, executor, id, expected+1, definition, by, now)
}

// Delete implements presentation.Store.
func (s *Store) Delete(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	// Locking the layout first makes a concurrent Subscription save that references it finish
	// before the check below, so the reference is seen instead of failing the delete.
	if _, err := executor.ExecContext(ctx, `SELECT 1 FROM layout_definitions WHERE id = $1 FOR UPDATE`, id); err != nil {
		return err
	}
	var referenced bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM subscriptions WHERE layout_id = $1)`, id).Scan(&referenced); err != nil {
		return err
	}
	if referenced {
		return authz.ErrRevisionConflict
	}
	result, err := executor.ExecContext(ctx, `DELETE FROM layout_definitions WHERE id = $1 AND revision = $2`, id, expected)
	return requireOneRow(result, err)
}

// requireOneRow reports a statement that matched no row as a revision conflict.
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

// SchemaAssignments implements presentation.Store.
func (s *Store) SchemaAssignments(ctx context.Context, tx platform.TransactionContext, keys []string) (map[string]presentation.SchemaAssignment, error) {
	found, err := s.catalog.SchemaAssignments(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	result := make(map[string]presentation.SchemaAssignment, len(found))
	for key, assignment := range found {
		result[key] = presentation.SchemaAssignment{Agencies: assignment.Agencies, CreatedAt: assignment.CreatedAt}
	}
	return result, nil
}
