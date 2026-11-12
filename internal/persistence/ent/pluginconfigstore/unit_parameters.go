package pluginconfigstore

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// UnitExists reports whether the Unit exists.
func (s *Store) UnitExists(ctx context.Context, tx platform.TransactionContext, unitID uuid.UUID) (bool, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return false, err
	}
	var exists bool
	err = executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM units WHERE id = $1)`, unitID).Scan(&exists)
	return exists, err
}

// ListUnitParameters returns a page of the instance's per-Unit values, ordered by Unit ID. A
// non-nil unit restricts the list to that Unit. Total counts the Units with values.
func (s *Store) ListUnitParameters(ctx context.Context, tx platform.TransactionContext, instanceID uuid.UUID,
	unit *uuid.UUID, page processing.PageInput) (processing.Page[processing.UnitParameters], error) {
	result := processing.Page[processing.UnitParameters]{Items: []processing.UnitParameters{}}
	if page.Offset < 0 || page.Limit <= 0 {
		return result, authz.ErrInvalidInput
	}
	executor, err := s.executor(tx)
	if err != nil {
		return result, err
	}
	if err := executor.QueryRowContext(ctx, `SELECT count(DISTINCT unit_id) FROM integration_unit_parameters
		WHERE instance_id = $1 AND ($2::uuid IS NULL OR unit_id = $2)`, instanceID, unit).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT p.unit_id, p.key, p.value::text FROM integration_unit_parameters p
		JOIN (SELECT DISTINCT unit_id FROM integration_unit_parameters
			WHERE instance_id = $1 AND ($2::uuid IS NULL OR unit_id = $2)
			ORDER BY unit_id OFFSET $3 LIMIT $4) u ON u.unit_id = p.unit_id
		WHERE p.instance_id = $1 ORDER BY p.unit_id, p.key`, instanceID, unit, page.Offset, page.Limit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var key, encoded string
		if err := rows.Scan(&id, &key, &encoded); err != nil {
			return result, err
		}
		var value processing.Value
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return result, platform.NewFailure(platform.CodeInternalError, err)
		}
		last := len(result.Items) - 1
		if last < 0 || result.Items[last].UnitID != id {
			result.Items = append(result.Items, processing.UnitParameters{UnitID: id, Values: map[string]processing.Value{}})
			last++
		}
		result.Items[last].Values[key] = value
	}
	return result, rows.Err()
}

// ReplaceUnitParameters replaces all of the Unit's values for the instance with values; an empty map
// removes them. It runs in the caller's transaction.
func (s *Store) ReplaceUnitParameters(ctx context.Context, tx platform.TransactionContext, instanceID, unitID uuid.UUID,
	values map[string]processing.Value) error {
	if err := s.DeleteUnitParameters(ctx, tx, instanceID, unitID); err != nil {
		return err
	}
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	for key, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return platform.NewFailure(platform.CodeInternalError, errors.New("unit parameter encoding"))
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO integration_unit_parameters (instance_id, unit_id, key, value)
			VALUES ($1, $2, $3, $4::jsonb)`, instanceID, unitID, key, string(encoded)); err != nil {
			return referenceGone(err)
		}
	}
	return nil
}

// DeleteUnitParameters removes all of the Unit's values for the instance.
func (s *Store) DeleteUnitParameters(ctx context.Context, tx platform.TransactionContext, instanceID, unitID uuid.UUID) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM integration_unit_parameters WHERE instance_id = $1 AND unit_id = $2`,
		instanceID, unitID)
	return err
}
