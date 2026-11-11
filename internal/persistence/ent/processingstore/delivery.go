package processingstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// CurrentInstance returns the current revision of a processing instance without an
// administrator's authority, for the Delivery side's target checks, which authorize the
// Subscription and not the instance configuration. It fails with authz.ErrNotFound for an
// unknown instance.
func (s *Store) CurrentInstance(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (processing.InstanceConfiguration, error) {
	if id == uuid.Nil {
		return processing.InstanceConfiguration{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	var revision int64
	err = executor.QueryRowContext(ctx, `SELECT revision FROM processing_instances WHERE id = $1`, id).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return processing.InstanceConfiguration{}, authz.ErrNotFound
	}
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	return s.ReadInstance(ctx, tx, id, revision)
}
