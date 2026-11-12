package processingstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.DeliveryInstances = (*Store)(nil)

const currentInstanceSQL = `SELECT v.configuration::text FROM processing_instances i
	JOIN processing_instance_versions v ON v.instance_id = i.id AND v.revision = i.revision`

// ListCurrentInstances returns the current revision of every instance, ordered by ID. It carries
// no administrator authority and is meant for the Delivery host's placement and checks, which
// authorize through the instance's own target use.
func (s *Store) ListCurrentInstances(ctx context.Context, tx platform.TransactionContext) ([]processing.InstanceConfiguration, error) {
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return nil, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, currentInstanceSQL+` ORDER BY i.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	instances := []processing.InstanceConfiguration{}
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		instance, err := decodeInstance(encoded)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

// ReadCurrentConfiguration returns the current revision of one instance without administrator
// authority, for owners that authorize through the instance's target use. A missing instance is
// authz.ErrNotFound.
func (s *Store) ReadCurrentConfiguration(ctx context.Context, tx platform.TransactionContext,
	id uuid.UUID) (processing.InstanceConfiguration, error) {
	if id == uuid.Nil {
		return processing.InstanceConfiguration{}, authz.ErrInvalidInput
	}
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return processing.InstanceConfiguration{}, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	var encoded string
	if err := executor.QueryRowContext(ctx, currentInstanceSQL+` WHERE i.id = $1`, id).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return processing.InstanceConfiguration{}, authz.ErrNotFound
		}
		return processing.InstanceConfiguration{}, err
	}
	return decodeInstance(encoded)
}

func decodeInstance(encoded string) (processing.InstanceConfiguration, error) {
	var instance processing.InstanceConfiguration
	if err := json.Unmarshal([]byte(encoded), &instance); err != nil {
		return processing.InstanceConfiguration{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	return instance, instance.Validate()
}
