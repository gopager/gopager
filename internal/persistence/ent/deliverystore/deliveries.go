package deliverystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

const deliveryColumns = `d.id, d.revision, d.origin, d.redelivery_of, d.subscription_id, d.subscription_revision,
	d.subscriber_id, d.domain_event_id, d.event_type, d.correlation_id, d.causation_id, d.target_instance_id, d.split_key,
	d.state, d.outcome_code, d.attempts, d.layout_id, d.layout_version, d.input::text, d.context::text, d.message_id,
	array_to_json(d.unit_ids)::text, array_to_json(d.agency_ids)::text, d.age_anchor, d.access_revision,
	d.claim_token, d.claim_expires_at, d.next_attempt_at, d.expires_at, d.created_at, d.updated_at, d.finished_at`

// deliveryContext is the private materialization context stored with a Delivery.
type deliveryContext struct {
	InstanceRevision int64 `json:"instanceRevision,string"`
}

// scanDelivery reads one row of deliveryColumns.
func scanDelivery(row interface{ Scan(...any) error }) (delivery.DeliveryRecord, error) {
	var record delivery.DeliveryRecord
	var redelivery, correlation, layout, message uuid.NullUUID
	var outcome, input sql.NullString
	var context, units, agencies string
	var claimExpires, finished sql.NullTime
	if err := row.Scan(&record.ID, &record.Revision, &record.Origin, &redelivery, &record.SubscriptionID,
		&record.SubscriptionRevision, &record.SubscriberID, &record.DomainEventID, &record.EventType, &correlation,
		&record.CausationID, &record.TargetInstanceID, &record.SplitKey, &record.State, &outcome, &record.AttemptCount,
		&layout, &record.LayoutVersion, &input, &context, &message, &units, &agencies, &record.AgeAnchor,
		&record.AccessRevision, &record.ClaimToken, &claimExpires, &record.Due, &record.ExpiresAt, &record.CreatedAt,
		&record.UpdatedAt, &finished); err != nil {
		return delivery.DeliveryRecord{}, err
	}
	optional := func(value uuid.NullUUID) *uuid.UUID {
		if !value.Valid {
			return nil
		}
		id := value.UUID
		return &id
	}
	record.RedeliveryOf, record.CorrelationID = optional(redelivery), optional(correlation)
	record.LayoutID, record.MessageID = optional(layout), optional(message)
	if outcome.Valid {
		code := outcome.String
		record.OutcomeCode = &code
	}
	if input.Valid {
		var decoded delivery.DeliveryInput
		if err := json.Unmarshal([]byte(input.String), &decoded); err != nil {
			return delivery.DeliveryRecord{}, platform.NewFailure(platform.CodeInternalError, err)
		}
		record.Input = &decoded
	}
	var stored deliveryContext
	if err := json.Unmarshal([]byte(context), &stored); err != nil {
		return delivery.DeliveryRecord{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	record.InstanceRevision = stored.InstanceRevision
	var err error
	if record.UnitIDs, err = uuidList(units); err != nil {
		return delivery.DeliveryRecord{}, err
	}
	if record.AgencyIDs, err = uuidList(agencies); err != nil {
		return delivery.DeliveryRecord{}, err
	}
	if claimExpires.Valid {
		at := claimExpires.Time.UTC()
		record.ClaimExpiresAt = &at
	}
	if finished.Valid {
		at := finished.Time.UTC()
		record.FinishedAt = &at
	}
	record.AgeAnchor, record.Due, record.ExpiresAt = record.AgeAnchor.UTC(), record.Due.UTC(), record.ExpiresAt.UTC()
	record.CreatedAt, record.UpdatedAt = record.CreatedAt.UTC(), record.UpdatedAt.UTC()
	return record, nil
}

// encodedInput returns the input column value.
func encodedInput(input *delivery.DeliveryInput) (any, error) {
	if input == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, platform.NewFailure(platform.CodeInternalError, err)
	}
	return string(encoded), nil
}

// encodedContext returns the context column value.
func encodedContext(record delivery.DeliveryRecord) (string, error) {
	encoded, err := json.Marshal(deliveryContext{InstanceRevision: record.InstanceRevision})
	if err != nil {
		return "", platform.NewFailure(platform.CodeInternalError, err)
	}
	return string(encoded), nil
}

// nullable returns nil for a missing optional ID.
func nullable(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return *id
}

// InsertDelivery implements delivery.Store.
func (s *Store) InsertDelivery(ctx context.Context, tx platform.TransactionContext, record delivery.DeliveryRecord) (bool, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return false, err
	}
	input, err := encodedInput(record.Input)
	if err != nil {
		return false, err
	}
	contextValue, err := encodedContext(record)
	if err != nil {
		return false, err
	}
	var outcome any
	if record.OutcomeCode != nil {
		outcome = *record.OutcomeCode
	}
	var finished any
	if record.FinishedAt != nil {
		finished = record.FinishedAt.UTC()
	}
	units, agencies := record.UnitIDs, record.AgencyIDs
	if units == nil {
		units = []uuid.UUID{}
	}
	if agencies == nil {
		agencies = []uuid.UUID{}
	}
	result, err := executor.ExecContext(ctx, `INSERT INTO deliveries (id, revision, origin, redelivery_of, subscription_id,
		subscription_revision, subscriber_id, domain_event_id, event_type, correlation_id, causation_id, target_instance_id,
		split_key, state, outcome_code, attempts, layout_id, layout_version, input, context, message_id, unit_ids, agency_ids,
		age_anchor, access_revision, claim_token, claim_expires_at, next_attempt_at, expires_at, created_at, updated_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22::uuid[],
		$23::uuid[], $24, $25, 0, NULL, $26, $27, $28, $28, $29)
		ON CONFLICT (subscription_id, domain_event_id, target_instance_id, split_key) WHERE origin = 'automatic' DO NOTHING`,
		record.ID, record.Revision, record.Origin, nullable(record.RedeliveryOf), record.SubscriptionID,
		record.SubscriptionRevision, record.SubscriberID, record.DomainEventID, record.EventType, nullable(record.CorrelationID),
		record.CausationID, record.TargetInstanceID, record.SplitKey, record.State, outcome, record.AttemptCount,
		nullable(record.LayoutID), record.LayoutVersion, input, contextValue, nullable(record.MessageID), units, agencies,
		record.AgeAnchor.UTC(), record.AccessRevision, record.Due.UTC(), record.ExpiresAt.UTC(), record.CreatedAt.UTC(), finished)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	return inserted == 1, err
}

// CancelOpen implements delivery.Store. Claimed Deliveries are left to their runner, whose start
// check cancels them.
func (s *Store) CancelOpen(ctx context.Context, tx platform.TransactionContext, subscription uuid.UUID, code string, now time.Time) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `UPDATE deliveries SET state = 'cancelled', outcome_code = $2, input = NULL,
		finished_at = $3, updated_at = $3, revision = revision + 1
		WHERE subscription_id = $1 AND state IN ('prepared', 'retry-pending')
		AND (claim_expires_at IS NULL OR claim_expires_at <= $3)`, subscription, code, now.UTC())
	return err
}

// ClaimDue implements delivery.Store.
func (s *Store) ClaimDue(ctx context.Context, tx platform.TransactionContext, now time.Time, lease time.Duration, limit int,
	instances []uuid.UUID) ([]delivery.DeliveryRecord, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return nil, err
	}
	if instances == nil {
		instances = []uuid.UUID{}
	}
	rows, err := executor.QueryContext(ctx, `WITH due AS (
			SELECT id FROM deliveries WHERE state IN ('prepared', 'retry-pending') AND next_attempt_at <= $1
			AND (claim_expires_at IS NULL OR claim_expires_at <= $1)
			AND (cardinality($4::uuid[]) = 0 OR target_instance_id = ANY($4::uuid[]))
			ORDER BY next_attempt_at, id LIMIT $3 FOR UPDATE SKIP LOCKED)
		UPDATE deliveries d SET claim_token = d.claim_token + 1, claim_expires_at = $2
		FROM due WHERE d.id = due.id RETURNING `+deliveryColumns,
		now.UTC(), now.Add(lease).UTC(), limit, instances)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []delivery.DeliveryRecord
	for rows.Next() {
		record, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ReclaimLost implements delivery.Store.
func (s *Store) ReclaimLost(ctx context.Context, tx platform.TransactionContext, now time.Time, limit int) (int, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return 0, err
	}
	rows, err := executor.QueryContext(ctx, `WITH lost AS (
			SELECT id FROM deliveries WHERE state = 'started' AND claim_expires_at <= $1
			ORDER BY claim_expires_at, id LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE deliveries d SET state = 'uncertain', outcome_code = 'lease-lost', input = NULL, finished_at = $1,
			updated_at = $1, revision = d.revision + 1, claim_expires_at = NULL
		FROM lost WHERE d.id = lost.id RETURNING d.id`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(ids) == 0 {
		return 0, nil
	}
	_, err = executor.ExecContext(ctx, `UPDATE delivery_attempts SET state = 'uncertain', outcome_code = 'lease-lost',
		finished_at = $2 WHERE delivery_id = ANY($1::uuid[]) AND state = 'started'`, ids, now.UTC())
	return len(ids), err
}

// requireOne maps a fenced write that changed no row to delivery.ErrLeaseLost.
func requireOne(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return delivery.ErrLeaseLost
	}
	return nil
}

// Renew implements delivery.Store.
func (s *Store) Renew(ctx context.Context, tx platform.TransactionContext, claim delivery.Claim, now time.Time, lease time.Duration) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	return requireOne(executor.ExecContext(ctx, `UPDATE deliveries SET claim_expires_at = $3
		WHERE id = $1 AND claim_token = $2 AND claim_expires_at > $4`, claim.DeliveryID, claim.Token,
		now.Add(lease).UTC(), now.UTC()))
}

// SaveClaimed implements delivery.Store.
func (s *Store) SaveClaimed(ctx context.Context, tx platform.TransactionContext, claim delivery.Claim,
	record delivery.DeliveryRecord, now time.Time) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	input, err := encodedInput(record.Input)
	if err != nil {
		return err
	}
	contextValue, err := encodedContext(record)
	if err != nil {
		return err
	}
	var outcome, finished any
	if record.OutcomeCode != nil {
		outcome = *record.OutcomeCode
	}
	if record.FinishedAt != nil {
		finished = record.FinishedAt.UTC()
	}
	return requireOne(executor.ExecContext(ctx, `UPDATE deliveries SET state = $3, outcome_code = $4, attempts = $5,
		next_attempt_at = $6, input = $7, context = $8, finished_at = $9, access_revision = $10, layout_id = $11,
		layout_version = $12, revision = revision + 1, updated_at = $13,
		claim_expires_at = CASE WHEN $3::varchar = 'started' THEN claim_expires_at ELSE NULL END
		WHERE id = $1 AND claim_token = $2 AND claim_expires_at > $13`,
		claim.DeliveryID, claim.Token, record.State, outcome, record.AttemptCount, record.Due.UTC(), input, contextValue,
		finished, record.AccessRevision, nullable(record.LayoutID), record.LayoutVersion, now.UTC()))
}

// claimHeld is the fencing condition for attempt writes.
const claimHeld = `EXISTS (SELECT 1 FROM deliveries WHERE id = $1::uuid AND claim_token = $2::bigint AND claim_expires_at > $3::timestamptz)`

// InsertAttempt implements delivery.Store.
func (s *Store) InsertAttempt(ctx context.Context, tx platform.TransactionContext, claim delivery.Claim, id uuid.UUID,
	attempt delivery.AttemptRecord, now time.Time) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	return requireOne(executor.ExecContext(ctx, `INSERT INTO delivery_attempts (delivery_id, number, id, state,
		instance_revision, access_revision, started_at)
		SELECT $1::uuid, $4::integer, $5::uuid, 'started', $6::bigint, $7::bigint, $8::timestamptz WHERE `+claimHeld,
		id, claim.Token, now.UTC(), attempt.Number, attempt.ID, attempt.InstanceRevision, attempt.AccessRevision,
		attempt.StartedAt.UTC()))
}

// FinishAttempt implements delivery.Store.
func (s *Store) FinishAttempt(ctx context.Context, tx platform.TransactionContext, claim delivery.Claim, id uuid.UUID,
	number int, state, code string, receipt json.RawMessage, now time.Time) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	var stored any
	if len(receipt) > 0 {
		stored = string(receipt)
	}
	return requireOne(executor.ExecContext(ctx, `UPDATE delivery_attempts SET state = $5, outcome_code = $6, receipt = $7,
		finished_at = $3 WHERE delivery_id = $1 AND number = $4 AND state = 'started' AND `+claimHeld,
		id, claim.Token, now.UTC(), number, state, code, stored))
}

// SetRetry implements delivery.Store.
func (s *Store) SetRetry(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64, now time.Time) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `UPDATE deliveries SET state = 'retry-pending', outcome_code = NULL,
		finished_at = NULL, next_attempt_at = $3, revision = revision + 1, updated_at = $3, claim_expires_at = NULL
		WHERE id = $1 AND revision = $2 AND state = 'failed'`, id, expected, now.UTC())
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return authz.ErrRevisionConflict
	}
	return nil
}

// Delivery implements delivery.Store.
func (s *Store) Delivery(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, lock bool) (delivery.DeliveryRecord, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return delivery.DeliveryRecord{}, err
	}
	query := `SELECT ` + deliveryColumns + ` FROM deliveries d WHERE d.id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	record, err := scanDelivery(executor.QueryRowContext(ctx, query, id))
	return record, notFound(err)
}

// Attempts implements delivery.Store.
func (s *Store) Attempts(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) ([]delivery.Attempt, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return nil, err
	}
	return attempts(ctx, executor, id)
}

// attempts reads the attempts of one Delivery in order.
func attempts(ctx context.Context, executor postgres.Executor, id uuid.UUID) ([]delivery.Attempt, error) {
	rows, err := executor.QueryContext(ctx, `SELECT number, id, state, outcome_code, instance_revision, started_at,
		finished_at, receipt::text FROM delivery_attempts WHERE delivery_id = $1 ORDER BY number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []delivery.Attempt{}
	for rows.Next() {
		var attempt delivery.Attempt
		var code, receipt sql.NullString
		var finished sql.NullTime
		if err := rows.Scan(&attempt.Number, &attempt.ID, &attempt.State, &code, &attempt.InstanceRevision,
			&attempt.StartedAt, &finished, &receipt); err != nil {
			return nil, err
		}
		attempt.StartedAt = attempt.StartedAt.UTC()
		if code.Valid {
			value := code.String
			attempt.OutcomeCode = &value
		}
		if finished.Valid {
			at := finished.Time.UTC()
			attempt.FinishedAt = &at
		}
		if receipt.Valid {
			attempt.Receipt = json.RawMessage(receipt.String)
		}
		items = append(items, attempt)
	}
	return items, rows.Err()
}
