package identitystore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

var _ identity.PreferenceRepository = (*Store)(nil)

// ReadPreferences implements identity.PreferenceRepository.
func (s *Store) ReadPreferences(ctx context.Context, tx platform.TransactionContext, account uuid.UUID) (identity.Preferences, bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return identity.Preferences{}, false, err
	}
	var preferences identity.Preferences
	err = executor.QueryRowContext(ctx, `SELECT locale, time_zone, time_gap_minutes, revision FROM account_preferences
		WHERE account_id = $1`, account).Scan(&preferences.Locale, &preferences.TimeZone, &preferences.TimeGapMinutes, &preferences.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.Preferences{}, false, nil
	}
	return preferences, err == nil, err
}

// SavePreferences implements identity.PreferenceRepository.
func (s *Store) SavePreferences(ctx context.Context, tx platform.TransactionContext, account uuid.UUID, expected int64,
	input identity.PreferencesInput, now time.Time) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var result sql.Result
	if expected == 0 {
		result, err = executor.ExecContext(ctx, `INSERT INTO account_preferences
			(account_id, revision, locale, time_zone, time_gap_minutes, updated_at) VALUES ($1, 1, $2, $3, $4, $5)
			ON CONFLICT DO NOTHING`, account, input.Locale, input.TimeZone, input.TimeGapMinutes, now)
	} else {
		result, err = executor.ExecContext(ctx, `UPDATE account_preferences SET locale = $3, time_zone = $4,
			time_gap_minutes = $5, revision = revision + 1, updated_at = $6 WHERE account_id = $1 AND revision = $2`,
			account, expected, input.Locale, input.TimeZone, input.TimeGapMinutes, now)
	}
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

// DeleteAccountPreferences removes the account's preferences in the caller's transaction, for the
// account deletion workflow, which is not implemented yet; it succeeds when the account has none.
func (s *Store) DeleteAccountPreferences(ctx context.Context, tx platform.TransactionContext, accountID uuid.UUID) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM account_preferences WHERE account_id = $1`, accountID)
	return err
}
