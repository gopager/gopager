package assignmentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/platform"
)

// ReadTargetContext returns the stored resolution of one target of a Transmission together
// with its Message, Network and timestamps. It returns authz.ErrInvalidInput for a nil ID
// or negative position and authz.ErrNotFound when the target does not exist.
func (s *Store) ReadTargetContext(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID, position int,
) (assignments.TargetContext, error) {
	if transmission == uuid.Nil || position < 0 {
		return assignments.TargetContext{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.TargetContext{}, err
	}
	var result assignments.TargetContext
	var status string
	var unitID, ruleID sql.NullString
	err = executor.QueryRowContext(ctx, `SELECT target.transmission_id,t.message_id,t.network_id,target.position,
		target.resolution_status,target.unit_id::text,target.matched_rule_id::text,t.observed_at,t.accepted_at
		FROM transmission_targets target JOIN transmissions t ON t.id=target.transmission_id
		WHERE target.transmission_id=$1 AND target.position=$2`,
		transmission, position).Scan(&result.TransmissionID, &result.MessageID, &result.NetworkID,
		&result.Position, &status, &unitID, &ruleID, &result.ObservedAt, &result.AcceptedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.TargetContext{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.TargetContext{}, err
	}
	result.Resolution = assignments.Resolution{TargetPosition: position, Status: status}
	result.Resolution.UnitID, err = parseCorrectionUUID(unitID)
	if err != nil {
		return assignments.TargetContext{}, err
	}
	result.Resolution.AddressRuleID, err = parseCorrectionUUID(ruleID)
	if err != nil {
		return assignments.TargetContext{}, err
	}
	return result, nil
}

// ReadSuppression returns a target suppression, active or lifted. It returns
// authz.ErrNotFound for an unknown ID.
func (s *Store) ReadSuppression(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (assignments.Suppression, error) {
	if id == uuid.Nil {
		return assignments.Suppression{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.Suppression{}, err
	}
	var value assignments.Suppression
	var liftedAt sql.NullTime
	err = executor.QueryRowContext(ctx, `SELECT id,transmission_id,target_position,revision,created_at,removed_at
		FROM target_suppressions WHERE id=$1`, id).Scan(&value.ID, &value.TransmissionID,
		&value.TargetPosition, &value.Revision, &value.CreatedAt, &liftedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.Suppression{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.Suppression{}, err
	}
	value.Active = !liftedAt.Valid
	if liftedAt.Valid {
		value.LiftedAt = &liftedAt.Time
	}
	return value, nil
}

// FindActiveSuppression returns the unlifted suppression of one Transmission target, if
// any; the bool is false when none exists.
func (s *Store) FindActiveSuppression(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID, position int,
) (assignments.Suppression, bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.Suppression{}, false, err
	}
	var value assignments.Suppression
	err = executor.QueryRowContext(ctx, `SELECT id,transmission_id,target_position,revision,created_at
		FROM target_suppressions WHERE transmission_id=$1 AND target_position=$2 AND removed_at IS NULL`,
		transmission, position).Scan(&value.ID, &value.TransmissionID,
		&value.TargetPosition, &value.Revision, &value.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.Suppression{}, false, nil
	}
	if err != nil {
		return assignments.Suppression{}, false, err
	}
	value.Active = true
	return value, true, nil
}

// ListSuppressions returns every target suppression, active and lifted, ordered by ID.
// The result is unpaginated.
func (s *Store) ListSuppressions(ctx context.Context, tx platform.TransactionContext) ([]assignments.Suppression, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id,transmission_id,target_position,revision,created_at,removed_at
		FROM target_suppressions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]assignments.Suppression, 0)
	for rows.Next() {
		var value assignments.Suppression
		var liftedAt sql.NullTime
		if err := rows.Scan(&value.ID, &value.TransmissionID, &value.TargetPosition, &value.Revision,
			&value.CreatedAt, &liftedAt); err != nil {
			return nil, err
		}
		value.Active = !liftedAt.Valid
		if liftedAt.Valid {
			value.LiftedAt = &liftedAt.Time
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// CreateSuppression inserts a new active suppression. value must have non-nil IDs, a
// non-negative target position, Revision 1 and a CreatedAt; actor must be non-nil. A
// second active suppression for the same target is rejected by the database.
func (s *Store) CreateSuppression(ctx context.Context, tx platform.TransactionContext,
	value assignments.Suppression, actor uuid.UUID,
) error {
	if value.ID == uuid.Nil || value.TransmissionID == uuid.Nil || value.TargetPosition < 0 ||
		value.Revision != 1 || actor == uuid.Nil || value.CreatedAt.IsZero() {
		return authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO target_suppressions
		(id,transmission_id,target_position,revision,created_at,created_by)
		VALUES($1,$2,$3,1,$4,$5)`, value.ID, value.TransmissionID, value.TargetPosition, value.CreatedAt, actor)
	return err
}

// LiftSuppression marks an active suppression as lifted at now by actor and increments its
// revision. It returns authz.ErrNotFound for an unknown ID and authz.ErrRevisionConflict
// when expected is stale or the suppression is already lifted.
func (s *Store) LiftSuppression(ctx context.Context, tx platform.TransactionContext, id uuid.UUID,
	expected int64, actor uuid.UUID, now time.Time,
) (assignments.Suppression, error) {
	if id == uuid.Nil || expected <= 0 || actor == uuid.Nil || now.IsZero() {
		return assignments.Suppression{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.Suppression{}, err
	}
	var value assignments.Suppression
	var liftedAt sql.NullTime
	err = executor.QueryRowContext(ctx, `UPDATE target_suppressions SET revision=revision+1,removed_at=$4,removed_by=$3
		WHERE id=$1 AND revision=$2 AND removed_at IS NULL
		RETURNING id,transmission_id,target_position,revision,created_at,removed_at`,
		id, expected, actor, now).Scan(&value.ID, &value.TransmissionID, &value.TargetPosition,
		&value.Revision, &value.CreatedAt, &liftedAt)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if queryErr := executor.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM target_suppressions WHERE id=$1)`, id).Scan(&exists); queryErr != nil {
			return assignments.Suppression{}, queryErr
		}
		if !exists {
			return assignments.Suppression{}, authz.ErrNotFound
		}
		return assignments.Suppression{}, authz.ErrRevisionConflict
	}
	if err != nil {
		return assignments.Suppression{}, err
	}
	value.Active = !liftedAt.Valid
	if liftedAt.Valid {
		value.LiftedAt = &liftedAt.Time
	}
	return value, nil
}

// ReadCorrection returns the status and progress counts of a correction scope. Internal
// state "queued" is reported as "pending". It returns authz.ErrNotFound for an unknown ID.
func (s *Store) ReadCorrection(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (assignments.CorrectionStatus, error) {
	if id == uuid.Nil {
		return assignments.CorrectionStatus{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	return readCorrection(ctx, executor, `id=$1`, id)
}

// CorrectionForRevision returns the most recently recorded correction scope for an
// Assignment revision, or authz.ErrNotFound when that revision recorded none.
func (s *Store) CorrectionForRevision(ctx context.Context, tx platform.TransactionContext,
	revision int64,
) (assignments.CorrectionStatus, error) {
	if revision <= 0 {
		return assignments.CorrectionStatus{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	return readCorrection(ctx, executor, `assignment_revision=$1 ORDER BY recorded_at DESC LIMIT 1`, revision)
}

// RetryCorrection supersedes a failed correction scope with a new queued scope covering
// the same Networks/Unit at the current Assignment revision, access revision and
// Transmission high-water mark, and returns the new scope. It takes the Assignment and
// access locks. It returns authz.ErrNotFound for an unknown ID and
// authz.ErrRevisionConflict when the scope is not in state "failed".
func (s *Store) RetryCorrection(ctx context.Context, tx platform.TransactionContext,
	id uuid.UUID,
) (assignments.CorrectionStatus, error) {
	if id == uuid.Nil {
		return assignments.CorrectionStatus{}, authz.ErrInvalidInput
	}
	revision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	var old assignments.CorrectionStatus
	var networks []byte
	var unitID sql.NullString
	err = executor.QueryRowContext(ctx, `SELECT assignment_revision,network_ids,unit_id,reason,state
		FROM assignment_pending_scopes WHERE id=$1 FOR UPDATE`, id).
		Scan(&old.AssignmentRev, &networks, &unitID, &old.Reason, &old.State)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionStatus{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if old.State != "failed" {
		return assignments.CorrectionStatus{}, authz.ErrRevisionConflict
	}
	if err := json.Unmarshal(networks, &old.Networks); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	old.UnitID, err = parseCorrectionUUID(unitID)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	access, err := identitystore.New(s.primary)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	accessRevision, err := access.LockAccess(ctx, tx)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	highWaterAt, highWaterID, err := correctionHighWater(ctx, executor, old.Networks, old.UnitID)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='superseded'
		WHERE id=$1 AND state='failed'`, id); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	newID := uuid.New()
	if _, err := executor.ExecContext(ctx, `INSERT INTO assignment_pending_scopes
		(id,assignment_revision,network_ids,unit_id,reason,recorded_at,state,
		 high_water_accepted_at,high_water_transmission_id,access_revision)
		VALUES($1,$2,$3::jsonb,$4,$5,$6,'queued',$7,$8,$9)`,
		newID, revision, string(networks), old.UnitID, old.Reason, now,
		highWaterAt, highWaterID, accessRevision); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	return readCorrection(ctx, executor, `id=$1`, newID)
}

func readCorrection(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, where string, arg any) (assignments.CorrectionStatus, error) {
	var value assignments.CorrectionStatus
	var networks []byte
	var highWaterAt, cursorAt sql.NullTime
	var highWaterID, cursorID, unitID sql.NullString
	err := executor.QueryRowContext(ctx, `SELECT id,assignment_revision,network_ids,unit_id::text,reason,state,
		high_water_accepted_at,high_water_transmission_id::text,cursor_accepted_at,cursor_transmission_id::text,
		attempts,COALESCE(failure_code,'') FROM assignment_pending_scopes WHERE `+where, arg).
		Scan(&value.ID, &value.AssignmentRev, &networks, &unitID, &value.Reason, &value.State,
			&highWaterAt, &highWaterID, &cursorAt, &cursorID, &value.Attempts, &value.FailureCode)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionStatus{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if err := json.Unmarshal(networks, &value.Networks); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if value.UnitID, err = parseCorrectionUUID(unitID); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if value.HighWaterID, err = parseCorrectionUUID(highWaterID); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if value.CursorID, err = parseCorrectionUUID(cursorID); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if highWaterAt.Valid {
		value.HighWaterAt = &highWaterAt.Time
	}
	if cursorAt.Valid {
		value.CursorAt = &cursorAt.Time
	}
	if value.Networks == nil {
		value.Networks = []uuid.UUID{}
	}
	value.NetworkCount = len(value.Networks)
	if value.UnitID != nil {
		value.UnitCount = 1
	}
	if value.State == "queued" {
		value.State = "pending"
	}
	if value.Processed, value.Remaining, err = correctionCounts(ctx, executor, value); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	return value, nil
}

func correctionCounts(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, value assignments.CorrectionStatus) (int, int, error) {
	networkValues := make([]string, len(value.Networks))
	for i, id := range value.Networks {
		networkValues[i] = id.String()
	}
	var processed, remaining int
	if value.HighWaterAt == nil || value.HighWaterID == nil {
		return 0, 0, nil
	}
	scope := `cardinality($1::text[]) = 0 OR t.network_id::text=ANY($1::text[])`
	targetScope := `($2::uuid IS NULL OR EXISTS (SELECT 1 FROM transmission_targets target
		WHERE target.transmission_id=t.id AND target.unit_id=$2))`
	upper := `(t.accepted_at,t.id) <= ($3,$4)`
	var cursorAt, cursorID any
	if value.CursorAt != nil && value.CursorID != nil {
		cursorAt, cursorID = value.CursorAt, value.CursorID
	}
	// Without a cursor nothing has been processed yet, so processed stays 0.
	if cursorAt != nil {
		err := executor.QueryRowContext(ctx, `SELECT count(*) FROM transmissions t WHERE (`+scope+`)
			AND `+targetScope+` AND `+upper+` AND (t.accepted_at,t.id) <= ($5,$6)`,
			networkValues, value.UnitID, value.HighWaterAt, value.HighWaterID, cursorAt, cursorID).Scan(&processed)
		if err != nil {
			return 0, 0, err
		}
	}
	err := executor.QueryRowContext(ctx, `SELECT count(*) FROM transmissions t WHERE (`+scope+`)
		AND `+targetScope+` AND `+upper+` AND ($5::timestamptz IS NULL OR (t.accepted_at,t.id) > ($5,$6))`,
		networkValues, value.UnitID, value.HighWaterAt, value.HighWaterID, cursorAt, cursorID).Scan(&remaining)
	if err != nil {
		return 0, 0, err
	}
	var dirty int
	if err := executor.QueryRowContext(ctx, `SELECT count(*) FROM correction_dirty_transmissions WHERE scope_id=$1`,
		value.ID).Scan(&dirty); err != nil {
		return 0, 0, err
	}
	remaining += dirty
	return processed, remaining, nil
}

var _ assignments.CorrectionRepository = (*Store)(nil)

func parseCorrectionUUID(value sql.NullString) (*uuid.UUID, error) {
	if !value.Valid {
		return nil, nil
	}
	id, err := uuid.Parse(value.String)
	if err != nil {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	return &id, nil
}
