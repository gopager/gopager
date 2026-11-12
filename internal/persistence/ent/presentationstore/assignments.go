package presentationstore

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/presentation"
)

const assignmentColumns = `scope_kind, scope_id, output_kind, layout_id, revision, updated_at`

func scanAssignment(row interface{ Scan(...any) error }) (presentation.Assignment, error) {
	var assignment presentation.Assignment
	err := row.Scan(&assignment.ScopeKind, &assignment.ScopeID, &assignment.OutputKind, &assignment.LayoutID,
		&assignment.Revision, &assignment.UpdatedAt)
	return assignment, err
}

// List implements presentation.AssignmentStore.
func (s *Store) List(ctx context.Context, tx platform.TransactionContext, scopeKind string, scopeID uuid.UUID) ([]presentation.Assignment, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	var kind any
	if scopeKind != "" {
		kind = scopeKind
	}
	var scope any
	if scopeID != uuid.Nil {
		scope = scopeID
	}
	rows, err := executor.QueryContext(ctx, `SELECT `+assignmentColumns+` FROM layout_assignments
		WHERE ($1::text IS NULL OR scope_kind = $1) AND ($2::uuid IS NULL OR scope_id = $2)
		ORDER BY scope_kind, scope_id, output_kind`, kind, scope)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []presentation.Assignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, assignment)
	}
	return result, rows.Err()
}

// Get implements presentation.AssignmentStore.
func (s *Store) Get(ctx context.Context, tx platform.TransactionContext, scopeKind string, scopeID uuid.UUID, outputKind string) (presentation.Assignment, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return presentation.Assignment{}, err
	}
	assignment, err := scanAssignment(executor.QueryRowContext(ctx, `SELECT `+assignmentColumns+` FROM layout_assignments
		WHERE scope_kind = $1 AND scope_id = $2 AND output_kind = $3`, scopeKind, scopeID, outputKind))
	if errors.Is(err, sql.ErrNoRows) {
		return presentation.Assignment{}, authz.ErrNotFound
	}
	return assignment, err
}

// Put implements presentation.AssignmentStore.
func (s *Store) Put(ctx context.Context, tx platform.TransactionContext, assignment presentation.Assignment, expected int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	// A share lock keeps a concurrent Layout delete from removing the Layout after this check.
	var found int
	err = executor.QueryRowContext(ctx, `SELECT 1 FROM layout_definitions WHERE id = $1 FOR SHARE`, assignment.LayoutID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return authz.InvalidField("layoutId")
	}
	if err != nil {
		return err
	}
	if expected == 0 {
		result, err := executor.ExecContext(ctx, `INSERT INTO layout_assignments (`+assignmentColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, assignment.ScopeKind, assignment.ScopeID,
			assignment.OutputKind, assignment.LayoutID, assignment.Revision, assignment.UpdatedAt)
		return requireOneRow(result, err)
	}
	result, err := executor.ExecContext(ctx, `UPDATE layout_assignments SET layout_id = $4, revision = revision + 1, updated_at = $5
		WHERE scope_kind = $1 AND scope_id = $2 AND output_kind = $3 AND revision = $6`, assignment.ScopeKind,
		assignment.ScopeID, assignment.OutputKind, assignment.LayoutID, assignment.UpdatedAt, expected)
	return requireOneRow(result, err)
}

// Delete implements presentation.AssignmentStore.
func (s *Store) Delete(ctx context.Context, tx platform.TransactionContext, scopeKind string, scopeID uuid.UUID, outputKind string, expected int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `DELETE FROM layout_assignments
		WHERE scope_kind = $1 AND scope_id = $2 AND output_kind = $3 AND revision = $4`, scopeKind, scopeID, outputKind, expected)
	if err := requireOneRow(result, err); err != nil {
		if !errors.Is(err, authz.ErrRevisionConflict) {
			return err
		}
		if _, getErr := s.Get(ctx, tx, scopeKind, scopeID, outputKind); errors.Is(getErr, authz.ErrNotFound) {
			return authz.ErrNotFound
		}
		return err
	}
	return nil
}

// ReadUnit implements presentation.AssignmentStore.
func (s *Store) ReadUnit(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (presentation.UnitContext, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return presentation.UnitContext{}, err
	}
	var unit presentation.UnitContext
	err = executor.QueryRowContext(ctx, `SELECT agency_id FROM units WHERE id = $1 AND NOT deleted`, id).Scan(&unit.AgencyID)
	if errors.Is(err, sql.ErrNoRows) {
		return presentation.UnitContext{}, authz.ErrNotFound
	}
	if err != nil {
		return presentation.UnitContext{}, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT collection_id FROM unit_collection_memberships WHERE unit_id = $1 ORDER BY collection_id`, id)
	if err != nil {
		return presentation.UnitContext{}, err
	}
	defer func() { _ = rows.Close() }()
	unit.CollectionIDs = []uuid.UUID{}
	for rows.Next() {
		var collection uuid.UUID
		if err := rows.Scan(&collection); err != nil {
			return presentation.UnitContext{}, err
		}
		unit.CollectionIDs = append(unit.CollectionIDs, collection)
	}
	return unit, rows.Err()
}

// OrganizationExists implements presentation.AssignmentStore.
func (s *Store) OrganizationExists(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	var exists bool
	err = executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE id = $1 AND NOT deleted)`, id).Scan(&exists)
	return exists, err
}

// UnitLayouts implements presentation.LayerStore.
func (s *Store) UnitLayouts(ctx context.Context, tx platform.TransactionContext, unitIDs []uuid.UUID, outputKind string) (map[uuid.UUID]uuid.UUID, error) {
	result := map[uuid.UUID]uuid.UUID{}
	if len(unitIDs) == 0 {
		return result, nil
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(unitIDs))
	for i, id := range unitIDs {
		ids[i] = id.String()
	}
	slices.Sort(ids)
	// The Unit's own assignment wins over its Organization's; an assignment counts only while
	// its Layout is still assigned to the Unit's Agency.
	rows, err := executor.QueryContext(ctx, `SELECT u.id, (
			SELECT a.layout_id FROM layout_assignments a
			JOIN layout_definition_agencies g ON g.layout_id = a.layout_id AND g.agency_id = u.agency_id
			WHERE a.output_kind = $2 AND (
				(a.scope_kind = 'unit' AND a.scope_id = u.id)
				OR (a.scope_kind = 'organization' AND a.scope_id = u.organization_id AND NOT o.deleted))
			ORDER BY CASE a.scope_kind WHEN 'unit' THEN 0 ELSE 1 END LIMIT 1)
		FROM units u LEFT JOIN organizations o ON o.id = u.organization_id
		WHERE u.id = ANY($1::uuid[]) AND NOT u.deleted ORDER BY u.id`, ids, outputKind)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var unit uuid.UUID
		var layout uuid.NullUUID
		if err := rows.Scan(&unit, &layout); err != nil {
			return nil, err
		}
		if layout.Valid {
			result[unit] = layout.UUID
		}
	}
	return result, rows.Err()
}

// InstallationDefault implements presentation.LayerStore.
func (s *Store) InstallationDefault(ctx context.Context, tx platform.TransactionContext) (presentation.DefaultLayout, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return presentation.DefaultLayout{}, err
	}
	var result presentation.DefaultLayout
	err = executor.QueryRowContext(ctx, `SELECT l.id, l.output_kind FROM installation_settings i
		JOIN layout_definitions l ON l.id = i.default_layout_id WHERE i.id = 1`).Scan(&result.ID, &result.OutputKind)
	if errors.Is(err, sql.ErrNoRows) {
		return presentation.DefaultLayout{}, nil
	}
	return result, err
}
