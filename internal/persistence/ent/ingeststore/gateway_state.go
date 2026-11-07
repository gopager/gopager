package ingeststore

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// recordTransition writes the history row and the gateway.state-changed fact of one
// Gateway state transition in the caller's transaction. The row's unique
// (gateway_id, revision) key rejects a second transition for the same revision.
func (s *Store) recordTransition(ctx context.Context, tx platform.TransactionContext, executor postgres.Executor,
	gateway uuid.UUID, transition ingest.GatewayTransition) error {
	if _, err := executor.ExecContext(ctx, `INSERT INTO gateway_state_history
		(id,gateway_id,revision,old_state,new_state,cause,occurred_at,contact_at,reason_code,detail)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''))`,
		uuid.New(), gateway, transition.Revision, transition.OldState, transition.NewState, transition.Cause,
		transition.OccurredAt, transition.ContactAt, transition.ReasonCode, transition.Detail); err != nil {
		return err
	}
	contact := transition.ContactAt
	return s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.GatewayStateChanged,
		GatewayID: gateway, Revision: transition.Revision, OccurredAt: transition.OccurredAt, ContactTime: &contact,
		OldState: transition.OldState, NewState: transition.NewState,
		ReasonCode: transition.ReasonCode, Detail: transition.Detail})
}

// MarkContactLost implements ingest.ContactRepository with one locking update: rows
// locked by a concurrent report, Submission or scan are skipped, and the qualifying
// condition is re-checked on the latest row version under the lock.
func (s *Store) MarkContactLost(ctx context.Context, tx platform.TransactionContext, now, cutoff time.Time,
	limit int) (int, error) {
	if now.IsZero() || cutoff.After(now) || limit <= 0 {
		return 0, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	rows, err := executor.QueryContext(ctx, `WITH due AS (
			SELECT id, state, last_contact_at FROM gateways
			WHERE enabled AND NOT deleted AND state IN ('active','degraded') AND last_contact_at <= $1
			ORDER BY last_contact_at, id LIMIT $2
			FOR UPDATE SKIP LOCKED)
		UPDATE gateways g SET state='contact-lost', revision=g.revision+1
		FROM due WHERE g.id=due.id
		RETURNING g.id, due.state, g.revision, due.last_contact_at`, cutoff, limit)
	if err != nil {
		return 0, err
	}
	type expired struct {
		gateway     uuid.UUID
		state       string
		revision    int64
		lastContact time.Time
	}
	var changed []expired
	for rows.Next() {
		var row expired
		if err := rows.Scan(&row.gateway, &row.state, &row.revision, &row.lastContact); err != nil {
			_ = rows.Close()
			return 0, err
		}
		changed = append(changed, row)
	}
	if err := closeRows(rows); err != nil {
		return 0, err
	}
	for _, row := range changed {
		if err := s.recordTransition(ctx, tx, executor, row.gateway, ingest.GatewayTransition{
			Revision: row.revision, OldState: row.state, NewState: ingest.StateContactLost, Cause: ingest.CauseTimeout,
			OccurredAt: now, ContactAt: row.lastContact,
		}); err != nil {
			return 0, err
		}
	}
	return len(changed), nil
}

// closeRows closes rows and reports an iteration error before a close error.
func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

var _ ingest.ContactRepository = (*Store)(nil)
