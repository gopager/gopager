package processingstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
)

var _ synthetic.StoredTransmissionReader = (*Store)(nil)

// ReadStoredTransmission returns the evidence of an accepted Transmission for a preview: its
// Payload document, Protocol, observation time, Network and original targets in order and the Unit each target is persisted as assigned to. It only
// reads and takes no lock. A missing Transmission is authz.ErrNotFound; who may read it is for the
// caller to decide.
func (s *Store) ReadStoredTransmission(ctx context.Context, tx platform.TransactionContext,
	id uuid.UUID) (synthetic.StoredTransmission, error) {
	if id == uuid.Nil {
		return synthetic.StoredTransmission{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return synthetic.StoredTransmission{}, err
	}
	var record synthetic.StoredTransmission
	var protocol string
	var payload, targets, units []byte
	var observed time.Time
	err = executor.QueryRowContext(ctx, `SELECT t.message_id, t.protocol, t.payload, t.observed_at, t.network_id,
		(SELECT COALESCE(json_agg(tt.original ORDER BY tt.position), '[]'::json)
			FROM transmission_targets tt WHERE tt.transmission_id = t.id),
		(SELECT COALESCE(json_agg(tt.unit_id ORDER BY tt.position), '[]'::json)
			FROM transmission_targets tt WHERE tt.transmission_id = t.id)
		FROM transmissions t WHERE t.id = $1`, id).Scan(&record.MessageID, &protocol, &payload, &observed,
		&record.NetworkID, &targets, &units)
	if errors.Is(err, sql.ErrNoRows) {
		return synthetic.StoredTransmission{}, authz.ErrNotFound
	}
	if err != nil {
		return synthetic.StoredTransmission{}, err
	}
	record.Protocol, record.Payload, record.ObservedAt = protocols.Protocol(protocol), json.RawMessage(payload), observed.UTC()
	if err := json.Unmarshal(targets, &record.Targets); err != nil {
		return synthetic.StoredTransmission{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	if err := json.Unmarshal(units, &record.AssignedUnits); err != nil {
		return synthetic.StoredTransmission{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	return record, nil
}
