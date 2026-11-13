package presentationstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/rowcheck"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// ReadInstallation implements presentation.SettingsStore.
func (s *Store) ReadInstallation(ctx context.Context, tx platform.TransactionContext) (presentation.Installation, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return presentation.Installation{}, err
	}
	var result presentation.Installation
	var banner, severity sql.NullString
	var layout uuid.NullUUID
	if err := executor.QueryRowContext(ctx, `SELECT title, banner, severity, default_layout_id, revision
		FROM installation_settings WHERE id = 1`).Scan(&result.Title, &banner, &severity, &layout, &result.Revision); err != nil {
		return presentation.Installation{}, err
	}
	if banner.Valid {
		result.Banner, result.Severity = &banner.String, &severity.String
	}
	if layout.Valid {
		result.DefaultLayoutID = &layout.UUID
	}
	return result, nil
}

// UpdateInstallation implements presentation.SettingsStore.
func (s *Store) UpdateInstallation(ctx context.Context, tx platform.TransactionContext, expected int64,
	input presentation.InstallationInput, now time.Time) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if input.DefaultLayoutID != nil {
		// A share lock keeps a concurrent Layout delete from removing the default after this check.
		var found int
		err := executor.QueryRowContext(ctx, `SELECT 1 FROM layout_definitions WHERE id = $1 FOR SHARE`, *input.DefaultLayoutID).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return authz.InvalidField("defaultLayoutId")
		}
		if err != nil {
			return err
		}
	}
	result, err := executor.ExecContext(ctx, `UPDATE installation_settings SET title = $2, banner = $3, severity = $4,
		default_layout_id = $5, revision = revision + 1, updated_at = $6 WHERE id = 1 AND revision = $1`,
		expected, input.Title, input.Banner, input.Severity, input.DefaultLayoutID, now)
	return rowcheck.RequireOne(result, err)
}

// LayoutReferenced implements presentation.ReferenceGuard.
func (s *Store) LayoutReferenced(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	// Locking the Layout first makes a concurrent assignment or default change finish before the
	// check below, so its reference is seen instead of failing the delete on the foreign key.
	if _, err := executor.ExecContext(ctx, `SELECT 1 FROM layout_definitions WHERE id = $1 FOR UPDATE`, id); err != nil {
		return false, err
	}
	var referenced bool
	err = executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM layout_assignments WHERE layout_id = $1)
		OR EXISTS (SELECT 1 FROM installation_settings WHERE default_layout_id = $1)`, id).Scan(&referenced)
	return referenced, err
}
