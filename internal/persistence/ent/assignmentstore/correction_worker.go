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
// increments the attempt count and fencing token and pins the current access revision.
// The bool is false when no scope is claimable, including when the chosen scope is locked
// by another worker or has used its five attempts (it is then marked failed with
// RETRY_LIMIT).
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
		WHERE assignment_revision=(SELECT revision FROM assignment_control WHERE id=1)
		AND state IN ('queued','running') AND (lease_until IS NULL OR lease_until<=$1)
		ORDER BY recorded_at,id LIMIT 1`, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionLease{}, false, nil
	}
	if err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	currentRevision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	accessRevision, err := s.accessRevision(ctx, tx)
	if err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	err = executor.QueryRowContext(ctx, `SELECT id FROM assignment_pending_scopes
		WHERE id=$1 AND assignment_revision=$2 AND state IN ('queued','running')
		AND (lease_until IS NULL OR lease_until<=$3) FOR UPDATE SKIP LOCKED`, id, currentRevision, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return assignments.CorrectionLease{}, false, nil
	}
	if err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	var attempts int
	var fencing int64
	if err := executor.QueryRowContext(ctx, `SELECT attempts,fencing_token FROM assignment_pending_scopes
		WHERE id=$1`, id).Scan(&attempts, &fencing); err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	if attempts >= 5 {
		if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes
			SET state='failed',lease_until=NULL,failure_code='RETRY_LIMIT' WHERE id=$1`, id); err != nil {
			return assignments.CorrectionLease{}, false, err
		}
		return assignments.CorrectionLease{}, false, nil
	}
	if fencing == int64(^uint64(0)>>1) {
		return assignments.CorrectionLease{}, false, platform.NewFailure(platform.CodeResourceLimit, errors.New("correction fencing token exhausted"))
	}
	leaseUntil := now.Add(lease)
	if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='running',
		attempts=attempts+1,fencing_token=fencing_token+1,lease_until=$2,failure_code=NULL,access_revision=$3
		WHERE id=$1`, id, leaseUntil, accessRevision); err != nil {
		return assignments.CorrectionLease{}, false, err
	}
	value, err := readCorrectionLease(ctx, executor, id)
	return value, err == nil, err
}

func readCorrectionLease(ctx context.Context, executor postgres.Executor, id uuid.UUID) (assignments.CorrectionLease, error) {
	var value assignments.CorrectionLease
	var encoded []byte
	var unitID, highWaterID, cursorID sql.NullString
	var highWaterAt, cursorAt, leaseUntil sql.NullTime
	err := executor.QueryRowContext(ctx, `SELECT id,assignment_revision,network_ids,unit_id::text,reason,
		high_water_accepted_at,high_water_transmission_id::text,cursor_accepted_at,cursor_transmission_id::text,
		access_revision,fencing_token,lease_until,attempts
		FROM assignment_pending_scopes WHERE id=$1 AND state='running'`, id).Scan(
		&value.ID, &value.AssignmentRev, &encoded, &unitID, &value.Reason, &highWaterAt,
		&highWaterID, &cursorAt, &cursorID, &value.AccessRevision, &value.FencingToken, &leaseUntil, &value.Attempts)
	if err != nil {
		return assignments.CorrectionLease{}, err
	}
	if err := json.Unmarshal(encoded, &value.Networks); err != nil {
		return assignments.CorrectionLease{}, err
	}
	if value.UnitID, err = parseCorrectionUUID(unitID); err != nil {
		return assignments.CorrectionLease{}, err
	}
	if value.HighWaterID, err = parseCorrectionUUID(highWaterID); err != nil {
		return assignments.CorrectionLease{}, err
	}
	if value.CursorID, err = parseCorrectionUUID(cursorID); err != nil {
		return assignments.CorrectionLease{}, err
	}
	if highWaterAt.Valid {
		value.HighWaterAt = &highWaterAt.Time
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
// held lease: explicitly dirtied Transmissions first, and only when none remain, the next
// in-scope Transmissions after the cursor and up to the high-water mark that still carry
// targets resolved at an older Assignment revision. An empty result means the scan is
// exhausted. A stale fencing token, expired lease or changed revision yields
// authz.ErrRevisionConflict.
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
	rows, err := executor.QueryContext(ctx, `SELECT t.id,t.message_id,t.network_id,t.protocol,t.targets,
		t.observed_at,t.accepted_at,t.source_revision,true
		FROM correction_dirty_transmissions dirty JOIN transmissions t ON t.id=dirty.transmission_id
		WHERE dirty.scope_id=$1 ORDER BY dirty.accepted_at,dirty.transmission_id LIMIT $2`, lease.ID, limit)
	if err != nil {
		return nil, err
	}
	items, err := scanCorrectionItems(rows)
	if err != nil || len(items) > 0 {
		return items, err
	}
	if lease.HighWaterAt == nil || lease.HighWaterID == nil {
		return []assignments.CorrectionBatchItem{}, nil
	}
	rows, err = executor.QueryContext(ctx, `SELECT t.id,t.message_id,t.network_id,t.protocol,t.targets,
		t.observed_at,t.accepted_at,t.source_revision,false
		FROM transmissions t
		WHERE (cardinality($1::uuid[])=0 OR t.network_id=ANY($1::uuid[]))
		AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM transmission_targets scoped
			WHERE scoped.transmission_id=t.id AND scoped.unit_id=$2))
		AND (t.accepted_at,t.id)<=($3,$4)
		AND ($5::timestamptz IS NULL OR (t.accepted_at,t.id)>($5,$6))
		AND EXISTS (SELECT 1 FROM transmission_targets stale
			WHERE stale.transmission_id=t.id AND stale.assignment_revision<$7)
		ORDER BY t.accepted_at,t.id LIMIT $8`,
		lease.Networks, lease.UnitID, lease.HighWaterAt, lease.HighWaterID,
		lease.CursorAt, lease.CursorID, lease.AssignmentRev, limit)
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
			&encoded, &item.ObservedAt, &item.AcceptedAt, &item.SourceRevision, &item.Dirty); err != nil {
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
// after the change. It takes the Assignment and access locks and fails with
// authz.ErrRevisionConflict when either revision, the fence, or a Transmission's source
// identity changed. Side effects: dirty entries are cleared or the scan cursor advances,
// every touched Message has its contributions rebuilt and is reset to unpublished
// (pending), Messages with changed targets get a new revision, and the lease is extended
// by 30 seconds.
func (s *Store) ApplyCorrectionBatch(ctx context.Context, tx platform.TransactionContext,
	lease assignments.CorrectionLease, updates []assignments.CorrectionUpdate,
) ([]assignments.CorrectionChange, error) {
	if lease.ID == uuid.Nil || lease.FencingToken <= 0 || len(updates) == 0 || len(updates) > 100 {
		return nil, authz.ErrInvalidInput
	}
	currentRevision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return nil, err
	}
	accessRevision, err := s.accessRevision(ctx, tx)
	if err != nil {
		return nil, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	if currentRevision != lease.AssignmentRev || accessRevision != lease.AccessRevision {
		return nil, authz.ErrRevisionConflict
	}
	if err := checkCorrectionFence(ctx, executor, lease, now); err != nil {
		return nil, err
	}
	messages := make([]uuid.UUID, 0, len(updates))
	messageSet := make(map[uuid.UUID]bool, len(updates))
	changedMessages := make([]uuid.UUID, 0, len(updates))
	changedMessageSet := make(map[uuid.UUID]bool, len(updates))
	var changes []assignments.CorrectionChange
	for _, update := range updates {
		if update.Item.TransmissionID == uuid.Nil || update.Item.MessageID == uuid.Nil ||
			update.Item.NetworkID == uuid.Nil || update.Item.SourceRevision <= 0 ||
			update.Match.Revision != lease.AssignmentRev || len(update.Match.Resolutions) != len(update.Item.Targets) {
			return nil, authz.ErrInvalidInput
		}
		var currentMessage, currentNetwork uuid.UUID
		var sourceRevision int64
		if err := executor.QueryRowContext(ctx, `SELECT message_id,network_id,source_revision
			FROM transmissions WHERE id=$1 FOR UPDATE`, update.Item.TransmissionID).
			Scan(&currentMessage, &currentNetwork, &sourceRevision); err != nil {
			return nil, err
		}
		if currentMessage != update.Item.MessageID || currentNetwork != update.Item.NetworkID ||
			sourceRevision != update.Item.SourceRevision {
			return nil, authz.ErrRevisionConflict
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
				return nil, authz.ErrInvalidInput
			}
			var before assignments.Resolution
			var unitID, ruleID sql.NullString
			err := executor.QueryRowContext(ctx, `SELECT resolution_status,unit_id::text,matched_rule_id::text
				FROM transmission_targets WHERE transmission_id=$1 AND position=$2 FOR UPDATE`,
				update.Item.TransmissionID, position).Scan(&before.Status, &unitID, &ruleID)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("original target missing"))
			}
			if err != nil {
				return nil, err
			}
			before.TargetPosition = position
			if before.UnitID, err = parseCorrectionUUID(unitID); err != nil {
				return nil, err
			}
			if before.AddressRuleID, err = parseCorrectionUUID(ruleID); err != nil {
				return nil, err
			}
			if _, err := executor.ExecContext(ctx, `UPDATE transmission_targets SET resolution_status=$3,
				unit_id=$4,matched_rule_id=$5,assignment_revision=$6
				WHERE transmission_id=$1 AND position=$2`, update.Item.TransmissionID, position,
				resolution.Status, resolution.UnitID, resolution.AddressRuleID, lease.AssignmentRev); err != nil {
				return nil, err
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
			changes = append(changes, assignments.CorrectionChange{TransmissionID: update.Item.TransmissionID,
				MessageID: update.Item.MessageID, NetworkID: update.Item.NetworkID, TargetPosition: position,
				Before: beforeFact, After: afterFact})
		}
		if update.Item.Dirty {
			if _, err := executor.ExecContext(ctx, `DELETE FROM correction_dirty_transmissions
				WHERE scope_id=$1 AND transmission_id=$2`, lease.ID, update.Item.TransmissionID); err != nil {
				return nil, err
			}
		} else if err := updateCorrectionCursor(ctx, executor, lease.ID, update.Item.AcceptedAt, update.Item.TransmissionID); err != nil {
			return nil, err
		}
	}
	revisionByMessage := make(map[uuid.UUID]int64, len(messages))
	for _, message := range messages {
		if err := rebuildMessageContributions(ctx, executor, message); err != nil {
			return nil, err
		}
	}
	for _, message := range changedMessages {
		var revision int64
		if err := executor.QueryRowContext(ctx, `UPDATE messages SET revision=revision+1
			WHERE id=$1 RETURNING revision`, message).Scan(&revision); err != nil {
			return nil, err
		}
		revisionByMessage[message] = revision
	}
	for i := range changes {
		changes[i].MessageRev = revisionByMessage[changes[i].MessageID]
	}
	_, err = executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET lease_until=$3
		WHERE id=$1 AND state='running' AND fencing_token=$2`, lease.ID, lease.FencingToken, now.Add(30*time.Second))
	return changes, err
}

func checkCorrectionFence(ctx context.Context, executor postgres.Executor, lease assignments.CorrectionLease,
	now time.Time,
) error {
	var state string
	var revision, token, accessRevision int64
	var until sql.NullTime
	err := executor.QueryRowContext(ctx, `SELECT state,assignment_revision,fencing_token,access_revision,lease_until
		FROM assignment_pending_scopes WHERE id=$1 FOR UPDATE`, lease.ID).Scan(
		&state, &revision, &token, &accessRevision, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return authz.ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "running" || revision != lease.AssignmentRev || token != lease.FencingToken ||
		accessRevision != lease.AccessRevision || !until.Valid || !until.Time.After(now) {
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

func rebuildMessageContributions(ctx context.Context, executor postgres.Executor, message uuid.UUID) error {
	if _, err := executor.ExecContext(ctx, `DELETE FROM contribution_supports support USING transmissions t
		WHERE support.transmission_id=t.id AND t.message_id=$1`, message); err != nil {
		return err
	}
	rows, err := executor.QueryContext(ctx, `SELECT target.unit_id,MIN(t.observed_at),
		MIN(LEAST(t.observed_at,t.accepted_at))
		FROM transmissions t JOIN transmission_targets target ON target.transmission_id=t.id
		WHERE t.message_id=$1 AND target.resolution_status='assigned' AND target.unit_id IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM target_suppressions suppression
			WHERE suppression.transmission_id=target.transmission_id
			AND suppression.target_position=target.position AND suppression.removed_at IS NULL)
		GROUP BY target.unit_id ORDER BY target.unit_id`, message)
	if err != nil {
		return err
	}
	type unitAggregate struct {
		unit      uuid.UUID
		observed  time.Time
		ageAnchor time.Time
	}
	var aggregates []unitAggregate
	for rows.Next() {
		var value unitAggregate
		if err := rows.Scan(&value.unit, &value.observed, &value.ageAnchor); err != nil {
			rows.Close()
			return err
		}
		aggregates = append(aggregates, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, value := range aggregates {
		if _, err := executor.ExecContext(ctx, `INSERT INTO message_contributions
			(id,message_id,unit_id,contribution_time,age_anchor)
			VALUES($1,$2,$3,$4,$5)
			ON CONFLICT (message_id,unit_id) WHERE unit_id IS NOT NULL DO UPDATE SET
				contribution_time=EXCLUDED.contribution_time,age_anchor=EXCLUDED.age_anchor`,
			uuid.New(), message, value.unit, value.observed, value.ageAnchor); err != nil {
			return err
		}
	}
	var targetless bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM transmissions t
		WHERE t.message_id=$1 AND jsonb_array_length(t.targets::jsonb)=0)`, message).Scan(&targetless); err != nil {
		return err
	}
	if targetless {
		if _, err := executor.ExecContext(ctx, `INSERT INTO message_contributions
			(id,message_id,unit_id,contribution_time,age_anchor)
			SELECT $1,$2,NULL,MIN(t.observed_at),MIN(LEAST(t.observed_at,t.accepted_at))
			FROM transmissions t WHERE t.message_id=$2 AND jsonb_array_length(t.targets::jsonb)=0
			ON CONFLICT (message_id) WHERE unit_id IS NULL DO UPDATE SET
				contribution_time=EXCLUDED.contribution_time,age_anchor=EXCLUDED.age_anchor`,
			uuid.New(), message); err != nil {
			return err
		}
	}
	rows, err = executor.QueryContext(ctx, `SELECT contribution.id,t.id,target.position,t.observed_at,t.accepted_at
		FROM transmissions t JOIN transmission_targets target ON target.transmission_id=t.id
		JOIN message_contributions contribution ON contribution.message_id=t.message_id
			AND contribution.unit_id=target.unit_id
		WHERE t.message_id=$1 AND target.resolution_status='assigned' AND target.unit_id IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM target_suppressions suppression
			WHERE suppression.transmission_id=target.transmission_id
			AND suppression.target_position=target.position AND suppression.removed_at IS NULL)`,
		message)
	if err != nil {
		return err
	}
	type support struct {
		contribution uuid.UUID
		transmission uuid.UUID
		position     int
		observed     time.Time
		accepted     time.Time
	}
	var supports []support
	for rows.Next() {
		var value support
		if err := rows.Scan(&value.contribution, &value.transmission, &value.position, &value.observed, &value.accepted); err != nil {
			rows.Close()
			return err
		}
		supports = append(supports, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, value := range supports {
		if _, err := executor.ExecContext(ctx, `INSERT INTO contribution_supports
			(id,contribution_id,transmission_id,target_position,observed_at,accepted_at)
			VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), value.contribution, value.transmission,
			value.position, value.observed, value.accepted); err != nil {
			return err
		}
	}
	rows, err = executor.QueryContext(ctx, `SELECT contribution.id,t.id,t.observed_at,t.accepted_at
		FROM transmissions t JOIN message_contributions contribution ON contribution.message_id=t.message_id
			AND contribution.unit_id IS NULL
		WHERE t.message_id=$1 AND jsonb_array_length(t.targets::jsonb)=0`, message)
	if err != nil {
		return err
	}
	var targetlessSupports []struct {
		contribution uuid.UUID
		transmission uuid.UUID
		observed     time.Time
		accepted     time.Time
	}
	for rows.Next() {
		var value struct {
			contribution uuid.UUID
			transmission uuid.UUID
			observed     time.Time
			accepted     time.Time
		}
		if err := rows.Scan(&value.contribution, &value.transmission, &value.observed, &value.accepted); err != nil {
			rows.Close()
			return err
		}
		targetlessSupports = append(targetlessSupports, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, value := range targetlessSupports {
		if _, err := executor.ExecContext(ctx, `INSERT INTO contribution_supports
			(id,contribution_id,transmission_id,target_position,observed_at,accepted_at)
			VALUES($1,$2,$3,NULL,$4,$5)`, uuid.New(), value.contribution, value.transmission,
			value.observed, value.accepted); err != nil {
			return err
		}
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM message_contributions contribution
		WHERE contribution.message_id=$1 AND NOT EXISTS (
			SELECT 1 FROM contribution_supports support WHERE support.contribution_id=contribution.id)`, message)
	return err
}

// CompleteCorrection marks the leased scope completed if nothing remains: no dirty
// Transmissions and no in-scope Transmission up to the high-water mark with targets at an
// older Assignment revision. It returns false, without error, while work remains. A stale
// fence or changed Assignment/access revision yields authz.ErrRevisionConflict.
func (s *Store) CompleteCorrection(ctx context.Context, tx platform.TransactionContext,
	lease assignments.CorrectionLease,
) (bool, error) {
	revision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return false, err
	}
	accessRevision, err := s.accessRevision(ctx, tx)
	if err != nil {
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
	if revision != lease.AssignmentRev || accessRevision != lease.AccessRevision {
		return false, authz.ErrRevisionConflict
	}
	var dirty bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM correction_dirty_transmissions WHERE scope_id=$1)`,
		lease.ID).Scan(&dirty); err != nil {
		return false, err
	}
	if dirty {
		return false, nil
	}
	if lease.HighWaterAt != nil && lease.HighWaterID != nil {
		var stale bool
		if err := executor.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM transmissions t
			WHERE (cardinality($1::uuid[])=0 OR t.network_id=ANY($1::uuid[]))
			AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM transmission_targets scoped
				WHERE scoped.transmission_id=t.id AND scoped.unit_id=$2))
			AND (t.accepted_at,t.id)<=($3,$4)
			AND EXISTS (SELECT 1 FROM transmission_targets target
				WHERE target.transmission_id=t.id AND target.assignment_revision<$5))`,
			lease.Networks, lease.UnitID, lease.HighWaterAt, lease.HighWaterID, lease.AssignmentRev).Scan(&stale); err != nil {
			return false, err
		}
		if stale {
			return false, nil
		}
	}
	_, err = executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='completed',lease_until=NULL,
		failure_code=NULL WHERE id=$1 AND state='running' AND fencing_token=$2`,
		lease.ID, lease.FencingToken)
	return err == nil, err
}

// FailCorrection records a failed attempt with failureCode. A lease that is no longer held
// is ignored (nil). If the Assignment revision moved on, the scope is superseded;
// otherwise it is requeued after delay (at most one minute), or marked failed once it has
// used five attempts. It returns authz.ErrNotFound for an unknown scope.
func (s *Store) FailCorrection(ctx context.Context, tx platform.TransactionContext, lease assignments.CorrectionLease,
	failureCode string, delay time.Duration,
) error {
	if failureCode == "" || delay < 0 || delay > time.Minute {
		return authz.ErrInvalidInput
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	currentRevision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var state string
	var revision, token int64
	var until sql.NullTime
	if err := executor.QueryRowContext(ctx, `SELECT state,assignment_revision,fencing_token,lease_until
		FROM assignment_pending_scopes WHERE id=$1 FOR UPDATE`, lease.ID).
		Scan(&state, &revision, &token, &until); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authz.ErrNotFound
		}
		return err
	}
	if state != "running" || revision != lease.AssignmentRev || token != lease.FencingToken ||
		!until.Valid || !until.Time.After(now) {
		return nil
	}
	if currentRevision != lease.AssignmentRev {
		_, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='superseded',lease_until=NULL
			WHERE id=$1 AND fencing_token=$2`, lease.ID, lease.FencingToken)
		return err
	}
	next := "queued"
	var retryAt any = now.Add(delay)
	if lease.Attempts >= 5 {
		next, retryAt = "failed", nil
	}
	_, err = executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state=$3,lease_until=$4,failure_code=$5
		WHERE id=$1 AND fencing_token=$2`, lease.ID, lease.FencingToken, next, retryAt, failureCode)
	return err
}

var _ assignments.CorrectionWorkRepository = (*Store)(nil)
