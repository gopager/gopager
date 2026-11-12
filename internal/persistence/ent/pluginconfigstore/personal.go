package pluginconfigstore

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ReadPersonalConfiguration implements processing.PersonalConfigurations; a missing row is
// authz.ErrNotFound.
func (s *Store) ReadPersonalConfiguration(ctx context.Context, tx platform.TransactionContext,
	instanceID, ownerID uuid.UUID) (processing.PersonalConfiguration, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return processing.PersonalConfiguration{}, err
	}
	personal := processing.PersonalConfiguration{InstanceID: instanceID, OwnerID: ownerID}
	var encoded string
	err = executor.QueryRowContext(ctx, `SELECT revision, configuration::text, updated_at
		FROM plugin_personal_configurations WHERE instance_id = $1 AND owner_id = $2`, instanceID, ownerID).
		Scan(&personal.Revision, &encoded, &personal.UpdatedAt)
	if err != nil {
		return processing.PersonalConfiguration{}, notFound(err)
	}
	if err := json.Unmarshal([]byte(encoded), &personal.Configuration); err != nil {
		return processing.PersonalConfiguration{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	return personal, nil
}

// SavePersonalConfiguration creates (expected 0) or replaces (expected equals the stored revision)
// the owner's configuration and returns the new revision. A mismatch is authz.ErrRevisionConflict.
func (s *Store) SavePersonalConfiguration(ctx context.Context, tx platform.TransactionContext,
	instanceID, ownerID uuid.UUID, expected int64, configuration processing.Value) (int64, error) {
	if expected < 0 || instanceID == uuid.Nil || ownerID == uuid.Nil {
		return 0, authz.ErrInvalidInput
	}
	executor, err := s.executor(tx)
	if err != nil {
		return 0, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return 0, platform.NewFailure(platform.CodeInternalError, err)
	}
	if expected == 0 {
		result, err := executor.ExecContext(ctx, `INSERT INTO plugin_personal_configurations
			(instance_id, owner_id, revision, configuration, updated_at) VALUES ($1, $2, 1, $3::jsonb, $4)
			ON CONFLICT DO NOTHING`, instanceID, ownerID, string(encoded), now)
		if err != nil {
			return 0, referenceGone(err)
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return 0, authz.ErrRevisionConflict
		}
		return 1, nil
	}
	result, err := executor.ExecContext(ctx, `UPDATE plugin_personal_configurations
		SET revision = revision + 1, configuration = $4::jsonb, updated_at = $5
		WHERE instance_id = $1 AND owner_id = $2 AND revision = $3`, instanceID, ownerID, expected, string(encoded), now)
	if err != nil {
		return 0, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return 0, authz.ErrRevisionConflict
	}
	return expected + 1, nil
}

// DeletePersonalConfiguration removes the owner's configuration at the expected revision; a
// mismatch or a missing row is authz.ErrRevisionConflict. The owner's Secret rows of the instance
// are removed with it.
func (s *Store) DeletePersonalConfiguration(ctx context.Context, tx platform.TransactionContext,
	instanceID, ownerID uuid.UUID, expected int64) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `DELETE FROM plugin_personal_configurations
		WHERE instance_id = $1 AND owner_id = $2 AND revision = $3`, instanceID, ownerID, expected)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return authz.ErrRevisionConflict
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM configuration_secrets WHERE instance_id = $1 AND owner_id = $2`,
		instanceID, ownerID)
	return err
}

// DeleteAccountData removes the account's personal configurations and its Secret rows. It is meant
// for account deletion and runs in the caller's transaction.
func (s *Store) DeleteAccountData(ctx context.Context, tx platform.TransactionContext, accountID uuid.UUID) error {
	if accountID == uuid.Nil {
		return authz.ErrInvalidInput
	}
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM plugin_personal_configurations WHERE owner_id = $1`, accountID); err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM configuration_secrets WHERE owner_id = $1`, accountID)
	return err
}
