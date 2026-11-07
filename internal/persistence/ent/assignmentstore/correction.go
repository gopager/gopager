package assignmentstore

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
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

// RetryCorrection supersedes a failed correction scope with a new queued scope of the same
// selector at the current Assignment and access revision and returns the new scope. It takes
// the shared Assignment lock. It returns authz.ErrNotFound for an unknown ID and
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
	var columns scopeColumns
	err = executor.QueryRowContext(ctx, `SELECT reason,state,`+scopeSelect+`
		FROM assignment_pending_scopes WHERE id=$1 FOR UPDATE`, id).Scan(append([]any{&old.Reason, &old.State}, columns.targets()...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionStatus{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if old.State != "failed" {
		return assignments.CorrectionStatus{}, authz.ErrRevisionConflict
	}
	old.Scope = columns.scope()
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='superseded'
		WHERE id=$1 AND state='failed'`, id); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	newID := uuid.New()
	if err := insertCorrectionScope(ctx, executor, newID, revision, old.Scope, old.Reason, now); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	return readCorrection(ctx, executor, `id=$1`, newID)
}

// scopeSelect lists the selector columns of assignment_pending_scopes in the order scopeColumns scans them.
const scopeSelect = `network_id,protocol,address_from,address_to,address_text,valid_from,valid_until,unit_id,transmission_id,target_position,gateway_id`

// scopeColumns scans the selector columns of a scope row.
type scopeColumns struct {
	network, unit, transmission, gateway *uuid.UUID
	protocol, addressText                *string
	addressFrom, addressTo               *int64
	validFrom, validUntil                *time.Time
	targetPosition                       *int
}

func (c *scopeColumns) targets() []any {
	return []any{&c.network, &c.protocol, &c.addressFrom, &c.addressTo, &c.addressText, &c.validFrom, &c.validUntil,
		&c.unit, &c.transmission, &c.targetPosition, &c.gateway}
}

func (c scopeColumns) scope() assignments.CorrectionScope {
	scope := assignments.CorrectionScope{NetworkID: c.network, AddressText: c.addressText, ValidFrom: c.validFrom,
		ValidUntil: c.validUntil, UnitID: c.unit, TransmissionID: c.transmission, TargetPosition: c.targetPosition,
		GatewayID: c.gateway}
	if c.protocol != nil {
		scope.Protocol = protocols.Protocol(*c.protocol)
	}
	if c.addressFrom != nil && c.addressTo != nil {
		from, to := uint64(*c.addressFrom), uint64(*c.addressTo)
		scope.AddressFrom, scope.AddressTo = &from, &to
	}
	return scope
}

func readCorrection(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, where string, arg any) (assignments.CorrectionStatus, error) {
	var value assignments.CorrectionStatus
	var columns scopeColumns
	var cursorAt sql.NullTime
	var cursorID sql.NullString
	err := executor.QueryRowContext(ctx, `SELECT id,assignment_revision,reason,state,cursor_accepted_at,
		cursor_transmission_id::text,attempts,COALESCE(failure_code,''),`+scopeSelect+`
		FROM assignment_pending_scopes WHERE `+where, arg).
		Scan(append([]any{&value.ID, &value.AssignmentRev, &value.Reason, &value.State, &cursorAt, &cursorID,
			&value.Attempts, &value.FailureCode}, columns.targets()...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionStatus{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.CorrectionStatus{}, err
	}
	value.Scope = columns.scope()
	if value.CursorID, err = parseCorrectionUUID(cursorID); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if cursorAt.Valid {
		value.CursorAt = &cursorAt.Time
	}
	if value.State == "queued" {
		value.State = "pending"
	}
	if value.Processed, value.Remaining, err = correctionCounts(ctx, executor, value); err != nil {
		return assignments.CorrectionStatus{}, err
	}
	if value.Scope.Kind() == "gateway" {
		if err := executor.QueryRowContext(ctx, `SELECT count(*) FROM transmissions
			WHERE gateway_id=$1 AND protocol=$2 AND network_unresolved`,
			*value.Scope.GatewayID, string(value.Scope.Protocol)).Scan(&value.Unresolved); err != nil {
			return assignments.CorrectionStatus{}, err
		}
	}
	return value, nil
}

// correctionCounts counts the Transmissions inside the scope: remaining are those with a target or,
// for a gateway scope, a Network resolution still below the scope's assignment revision, processed
// the rest.
func correctionCounts(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, value assignments.CorrectionStatus) (int, int, error) {
	const query = `SELECT count(*) FROM transmissions t WHERE ` + inScopeAnySQL
	var total, remaining int
	if err := executor.QueryRowContext(ctx, query, scopeArgs(value.Scope, value.Reason, math.MaxInt64)...).Scan(&total); err != nil {
		return 0, 0, err
	}
	if err := executor.QueryRowContext(ctx, query, scopeArgs(value.Scope, value.Reason, value.AssignmentRev)...).Scan(&remaining); err != nil {
		return 0, 0, err
	}
	return total - remaining, remaining, nil
}

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
