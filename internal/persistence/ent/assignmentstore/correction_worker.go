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
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// ClaimCorrection leases the oldest queued or lease-expired running correction scope of
// the current Assignment revision for lease (at most one minute) from now. Claiming
// increments the attempt count and fencing token.
// The bool is false when no scope is claimable, including when the chosen scope is locked
// by another worker or has used its five attempts. A scope with no attempt left is marked
// failed with RETRY_LIMIT and reported through the returned lease (Exhausted set, ID,
// Reason and Attempts filled) so the caller records the final failure fact.
func (s *Store) ClaimCorrection(ctx context.Context, tx platform.TransactionContext, now time.Time,
	lease time.Duration,
) (assignments.CorrectionLease, bool, error) {
	if now.IsZero() || lease <= 0 || lease > time.Minute {
		return assignments.CorrectionLease{}, false, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	var id uuid.UUID
	err = executor.QueryRowContext(ctx, `SELECT id FROM assignment_pending_scopes
		WHERE state IN ('queued','running') AND (lease_until IS NULL OR lease_until<=$1)
		ORDER BY recorded_at,id LIMIT 1`, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionLease{}, false, nil
	}
	if err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	err = executor.QueryRowContext(ctx, `SELECT id FROM assignment_pending_scopes
		WHERE id=$1 AND state IN ('queued','running')
		AND (lease_until IS NULL OR lease_until<=$2) FOR UPDATE SKIP LOCKED`, id, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionLease{}, false, nil
	}
	if err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	var attempts int
	var fencing int64
	var reason string
	if err := executor.QueryRowContext(ctx, `SELECT attempts,fencing_token,reason FROM assignment_pending_scopes
		WHERE id=$1`, id).Scan(&attempts, &fencing, &reason); err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	if attempts >= 5 {
		if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes
			SET state='failed',lease_until=NULL,failure_code='RETRY_LIMIT' WHERE id=$1`, id); err != nil {
			return assignments.CorrectionLease{}, false, err
		}
		return assignments.CorrectionLease{ID: id, Reason: reason, Attempts: attempts, Exhausted: true}, false, nil
	}
	if fencing == int64(^uint64(0)>>1) {
		return assignments.CorrectionLease{}, false, platform.NewFailure(platform.CodeResourceLimit, errors.New("correction fencing token exhausted"))
	}
	leaseUntil := now.Add(lease)
	if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='running',
		attempts=attempts+1,fencing_token=fencing_token+1,lease_until=$2,failure_code=NULL
		WHERE id=$1`, id, leaseUntil); err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	value, err := readCorrectionLease(ctx, executor, id)
	return value, err == nil, err
}

func readCorrectionLease(ctx context.Context, executor postgres.Executor, id uuid.UUID) (assignments.CorrectionLease, error) {
	var value assignments.CorrectionLease
	var columns scopeColumns
	var cursorID sql.NullString
	var cursorAt, leaseUntil sql.NullTime
	err := executor.QueryRowContext(ctx, `SELECT id,assignment_revision,reason,cursor_accepted_at,cursor_transmission_id::text,
		fencing_token,lease_until,attempts,`+scopeSelect+`
		FROM assignment_pending_scopes WHERE id=$1 AND state='running'`, id).Scan(
		append([]any{&value.ID, &value.AssignmentRev, &value.Reason, &cursorAt, &cursorID,
			&value.FencingToken, &leaseUntil, &value.Attempts}, columns.targets()...)...)
	if err != nil {
		return assignments.CorrectionLease{}, err
	}
	value.Scope = columns.scope()
	if value.CursorID, err = parseCorrectionUUID(cursorID); err != nil {
		return assignments.CorrectionLease{}, err
	}
	if cursorAt.Valid {
		value.CursorAt = &cursorAt.Time
	}
	if !leaseUntil.Valid {
		return assignments.CorrectionLease{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("running correction has no lease"))
	}
	value.LeaseUntil = leaseUntil.Time
	return value, nil
}

// ReadCorrectionBatch returns up to limit (1..100) Transmissions to re-resolve under a
// held lease: the next Transmissions after the cursor, in acceptance order, that still carry
// a target inside the scope's selector at an older Assignment revision. An empty result
// means the scan is exhausted. A stale fencing token, expired lease or changed revision
// yields authz.ErrRevisionConflict.
func (s *Store) ReadCorrectionBatch(ctx context.Context, tx platform.TransactionContext,
	lease assignments.CorrectionLease, limit int,
) ([]assignments.CorrectionBatchItem, error) {
	if lease.ID == uuid.Nil || lease.FencingToken <= 0 || limit <= 0 || limit > 100 {
		return nil, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	current, err := readCorrectionLease(ctx, executor, lease.ID)
	if err != nil {
		return nil, err
	}
	if current.FencingToken != lease.FencingToken || !current.LeaseUntil.After(now) ||
		current.AssignmentRev != lease.AssignmentRev {
		return nil, authz.ErrRevisionConflict
	}
	rows, err := executor.QueryContext(ctx, `SELECT t.id,t.message_id,t.network_id,t.protocol,
		(SELECT COALESCE(json_agg(tt.original ORDER BY tt.position),'[]'::json) FROM transmission_targets tt WHERE tt.transmission_id=t.id),
		t.observed_at,t.accepted_at,t.source_revision
		FROM transmissions t
		WHERE EXISTS (SELECT 1 FROM transmission_targets target WHERE target.transmission_id=t.id AND `+inScopeSQL+`)
		AND ($12::timestamptz IS NULL OR (t.accepted_at,t.id)>($12,$13))
		ORDER BY t.accepted_at,t.id LIMIT $14`,
		append(scopeArgs(lease.Scope, lease.AssignmentRev), lease.CursorAt, lease.CursorID, limit)...)
	if err != nil {
		return nil, err
	}
	return scanCorrectionItems(rows)
}

func scanCorrectionItems(rows *sql.Rows) ([]assignments.CorrectionBatchItem, error) {
	defer rows.Close()
	items := make([]assignments.CorrectionBatchItem, 0)
	for rows.Next() {
		var item assignments.CorrectionBatchItem
		var protocol string
		var encoded []byte
		if err := rows.Scan(&item.TransmissionID, &item.MessageID, &item.NetworkID, &protocol,
			&encoded, &item.ObservedAt, &item.AcceptedAt, &item.SourceRevision); err != nil {
			return nil, err
		}
		item.Protocol = protocols.Protocol(protocol)
		if err := json.Unmarshal(encoded, &item.Targets); err != nil {
			return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		if item.Targets == nil {
			item.Targets = []protocols.Target{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ApplyCorrectionBatch writes re-resolved targets (1..100 updates) under a held lease and
// returns the target resolutions that actually changed, each with the Message revision
// after the change, and every Transmission whose stamped revision moved. It takes the shared
// Assignment lock and fails with authz.ErrRevisionConflict when the fence or a Transmission's
// source identity changed; a newer Assignment or access revision is expected, the targets are
// stamped with the revision the fresh match was made at. Side effects: the scan cursor advances, every
// touched Message has its message_units rows rebuilt, Messages with changed targets get a
// new revision, and the lease is extended by 30 seconds.
func (s *Store) ApplyCorrectionBatch(ctx context.Context, tx platform.TransactionContext,
	lease assignments.CorrectionLease, updates []assignments.CorrectionUpdate,
) (assignments.CorrectionApplied, error) {
	if lease.ID == uuid.Nil || lease.FencingToken <= 0 || len(updates) == 0 || len(updates) > 100 {
		return assignments.CorrectionApplied{}, authz.ErrInvalidInput
	}
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return assignments.CorrectionApplied{}, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return assignments.CorrectionApplied{}, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return assignments.CorrectionApplied{}, err
	}
	if err := checkCorrectionFence(ctx, executor, lease, now); err != nil {
		return assignments.CorrectionApplied{}, err
	}
	messages := make([]uuid.UUID, 0, len(updates))
	messageSet := make(map[uuid.UUID]bool, len(updates))
	changedMessages := make([]uuid.UUID, 0, len(updates))
	changedMessageSet := make(map[uuid.UUID]bool, len(updates))
	restamped := make(map[uuid.UUID]bool, len(updates))
	var applied assignments.CorrectionApplied
	for _, update := range updates {
		if update.Item.TransmissionID == uuid.Nil || update.Item.MessageID == uuid.Nil ||
			update.Item.NetworkID == uuid.Nil || update.Item.SourceRevision <= 0 ||
			update.Match.Revision < lease.AssignmentRev || len(update.Match.Resolutions) != len(update.Item.Targets) {
			return assignments.CorrectionApplied{}, authz.ErrInvalidInput
		}
		var currentMessage, currentNetwork uuid.UUID
		var sourceRevision int64
		if err := executor.QueryRowContext(ctx, `SELECT message_id,network_id,source_revision
			FROM transmissions WHERE id=$1 FOR UPDATE`, update.Item.TransmissionID).
			Scan(&currentMessage, &currentNetwork, &sourceRevision); err != nil {
			return assignments.CorrectionApplied{}, err
		}
		if currentMessage != update.Item.MessageID || currentNetwork != update.Item.NetworkID ||
			sourceRevision != update.Item.SourceRevision {
			return assignments.CorrectionApplied{}, authz.ErrRevisionConflict
		}
		if !messageSet[update.Item.MessageID] {
			messageSet[update.Item.MessageID] = true
			messages = append(messages, update.Item.MessageID)
		}
		for position, resolution := range update.Match.Resolutions {
			if resolution.TargetPosition != position ||
				(resolution.Status != "assigned" && resolution.Status != "unassigned") ||
				(resolution.Status == "assigned") != (resolution.UnitID != nil) ||
				(resolution.Status == "assigned") != (resolution.AddressRuleID != nil) {
				return assignments.CorrectionApplied{}, authz.ErrInvalidInput
			}
			var before assignments.Resolution
			var unitID, ruleID sql.NullString
			var beforeRevision int64
			err := executor.QueryRowContext(ctx, `SELECT resolution_status,unit_id::text,matched_rule_id::text,assignment_revision
				FROM transmission_targets WHERE transmission_id=$1 AND position=$2 FOR UPDATE`,
				update.Item.TransmissionID, position).Scan(&before.Status, &unitID, &ruleID, &beforeRevision)
			if errors.Is(err, sql.ErrNoRows) {
				return assignments.CorrectionApplied{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("original target missing"))
			}
			if err != nil {
				return assignments.CorrectionApplied{}, err
			}
			before.TargetPosition = position
			if before.UnitID, err = parseCorrectionUUID(unitID); err != nil {
				return assignments.CorrectionApplied{}, err
			}
			if before.AddressRuleID, err = parseCorrectionUUID(ruleID); err != nil {
				return assignments.CorrectionApplied{}, err
			}
			if _, err := executor.ExecContext(ctx, `UPDATE transmission_targets SET resolution_status=$3,
				unit_id=$4,matched_rule_id=$5,assignment_revision=$6
				WHERE transmission_id=$1 AND position=$2`, update.Item.TransmissionID, position,
				resolution.Status, resolution.UnitID, resolution.AddressRuleID, update.Match.Revision); err != nil {
				return assignments.CorrectionApplied{}, err
			}
			if beforeRevision != update.Match.Revision && !restamped[update.Item.TransmissionID] {
				restamped[update.Item.TransmissionID] = true
				applied.Restamped = append(applied.Restamped, update.Item.TransmissionID)
			}
			if resolutionEqual(before, resolution) {
				continue
			}
			if !changedMessageSet[update.Item.MessageID] {
				changedMessageSet[update.Item.MessageID] = true
				changedMessages = append(changedMessages, update.Item.MessageID)
			}
			beforeFact := events.IntakeResolution{Status: before.Status, UnitID: before.UnitID, AddressRuleID: before.AddressRuleID}
			afterFact := events.IntakeResolution{Status: resolution.Status, UnitID: resolution.UnitID, AddressRuleID: resolution.AddressRuleID}
			applied.Changes = append(applied.Changes, assignments.CorrectionChange{TransmissionID: update.Item.TransmissionID,
				MessageID: update.Item.MessageID, NetworkID: update.Item.NetworkID, TargetPosition: position,
				Before: beforeFact, After: afterFact})
		}
		if err := updateCorrectionCursor(ctx, executor, lease.ID, update.Item.AcceptedAt, update.Item.TransmissionID); err != nil {
			return assignments.CorrectionApplied{}, err
		}
	}
	revisionByMessage := make(map[uuid.UUID]int64, len(messages))
	for _, message := range messages {
		if err := refreshMessageUnits(ctx, executor, message); err != nil {
			return assignments.CorrectionApplied{}, err
		}
	}
	for _, message := range changedMessages {
		var revision int64
		if err := executor.QueryRowContext(ctx, `UPDATE messages SET revision=revision+1
			WHERE id=$1 RETURNING revision`, message).Scan(&revision); err != nil {
			return assignments.CorrectionApplied{}, err
		}
		revisionByMessage[message] = revision
	}
	for i := range applied.Changes {
		applied.Changes[i].MessageRev = revisionByMessage[applied.Changes[i].MessageID]
	}
	_, err = executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET lease_until=$3
		WHERE id=$1 AND state='running' AND fencing_token=$2`, lease.ID, lease.FencingToken, now.Add(30*time.Second))
	return applied, err
}

func checkCorrectionFence(ctx context.Context, executor postgres.Executor, lease assignments.CorrectionLease,
	now time.Time,
) error {
	var state string
	var revision, token int64
	var until sql.NullTime
	err := executor.QueryRowContext(ctx, `SELECT state,assignment_revision,fencing_token,lease_until
		FROM assignment_pending_scopes WHERE id=$1 FOR UPDATE`, lease.ID).Scan(
		&state, &revision, &token, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return authz.ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "running" || revision != lease.AssignmentRev || token != lease.FencingToken ||
		!until.Valid || !until.Time.After(now) {
		return authz.ErrRevisionConflict
	}
	return nil
}

func updateCorrectionCursor(ctx context.Context, executor postgres.Executor, id uuid.UUID, accepted time.Time,
	transmission uuid.UUID,
) error {
	_, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET
		cursor_accepted_at=$2,cursor_transmission_id=$3
		WHERE id=$1 AND (cursor_accepted_at IS NULL OR (cursor_accepted_at,cursor_transmission_id)<($2,$3))`,
		id, accepted, transmission)
	return err
}

func resolutionEqual(left, right assignments.Resolution) bool {
	return left.Status == right.Status &&
		(left.UnitID == nil && right.UnitID == nil || left.UnitID != nil && right.UnitID != nil && *left.UnitID == *right.UnitID) &&
		(left.AddressRuleID == nil && right.AddressRuleID == nil ||
			left.AddressRuleID != nil && right.AddressRuleID != nil && *left.AddressRuleID == *right.AddressRuleID)
}

// CompleteCorrection marks the leased scope completed if no target inside its selector still
// carries an older Assignment revision. It returns false, without error, while work remains.
// A stale fence yields authz.ErrRevisionConflict.
func (s *Store) CompleteCorrection(ctx context.Context, tx platform.TransactionContext,
	lease assignments.CorrectionLease,
) (bool, error) {
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return false, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return false, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	if err := checkCorrectionFence(ctx, executor, lease, now); err != nil {
		return false, err
	}
	var stale bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM transmission_targets target WHERE `+inScopeSQL+`)`,
		scopeArgs(lease.Scope, lease.AssignmentRev)...).Scan(&stale); err != nil {
		return false, err
	}
	if stale {
		return false, nil
	}
	_, err = executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='completed',lease_until=NULL,
		failure_code=NULL WHERE id=$1 AND state='running' AND fencing_token=$2`,
		lease.ID, lease.FencingToken)
	return err == nil, err
}

// FailCorrection records a failed attempt with failureCode. A lease that is no longer held
// is ignored (false, nil). The scope is requeued after delay (at most one minute), or marked
// failed once it has used five attempts, which the returned flag reports so the caller can
// record the final failure. It returns authz.ErrNotFound for an unknown scope.
func (s *Store) FailCorrection(ctx context.Context, tx platform.TransactionContext, lease assignments.CorrectionLease,
	failureCode string, delay time.Duration,
) (bool, error) {
	if failureCode == "" || delay < 0 || delay > time.Minute {
		return false, authz.ErrInvalidInput
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return false, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	var state string
	var revision, token int64
	var until sql.NullTime
	if err := executor.QueryRowContext(ctx, `SELECT state,assignment_revision,fencing_token,lease_until
		FROM assignment_pending_scopes WHERE id=$1 FOR UPDATE`, lease.ID).
		Scan(&state, &revision, &token, &until); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, authz.ErrNotFound
		}
		return false, err
	}
	if state != "running" || revision != lease.AssignmentRev || token != lease.FencingToken ||
		!until.Valid || !until.Time.After(now) {
		return false, nil
	}
	next := "queued"
	var retryAt any = now.Add(delay)
	final := lease.Attempts >= 5
	if final {
		next, retryAt = "failed", nil
	}
	_, err = executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state=$3,lease_until=$4,failure_code=$5
		WHERE id=$1 AND fencing_token=$2`, lease.ID, lease.FencingToken, next, retryAt, failureCode)
	return final, err
}

var _ assignments.CorrectionWorkRepository = (*Store)(nil)
