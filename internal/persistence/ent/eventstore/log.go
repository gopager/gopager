package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
)

// notify announces the freshly appended row on events.NotifyChannel in the caller's
// transaction. PostgreSQL delivers the notification only if and when the transaction commits,
// so listeners never hear about a rolled-back fact. The seq is read back from the row in the
// same statement because Ent does not return database-generated columns after an insert.
func (s *Store) notify(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx,
		"SELECT pg_notify($1, seq::text) FROM domain_events WHERE id = $2", events.NotifyChannel, id)
	return err
}

// readAfterQuery selects log rows past a position. Sequence values are assigned at insert
// time but become visible in commit order, so a plain "seq > last" read could return a higher
// seq and then miss a lower one that commits later. Restricting rows to transactions older
// than the oldest still-running transaction (the snapshot's xmin) guarantees that every row
// below a returned seq is already final.
const readAfterQuery = `SELECT seq, id, coalesce(type, ''), occurred_at, envelope
	FROM domain_events
	WHERE seq > $1 AND tx_id < pg_snapshot_xmin(pg_current_snapshot())
	ORDER BY seq
	LIMIT $2`

// ReadAfter implements events.Log. A transaction never reads facts it appended itself,
// because its own transaction is still in flight; a consumer that appends facts while
// consuming sees them on a later read.
func (s *Store) ReadAfter(ctx context.Context, tx platform.TransactionContext, lastSeq int64, limit int) (records []events.Record, err error) {
	if lastSeq < 0 || limit <= 0 {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("event log read needs a non-negative position and a positive limit"))
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, readAfterQuery, lastSeq, limit)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var (
			record     events.Record
			occurredAt sql.NullTime
			envelope   []byte
		)
		if err := rows.Scan(&record.Seq, &record.ID, &record.Type, &occurredAt, &envelope); err != nil {
			return nil, err
		}
		record.OccurredAt = occurredAt.Time
		record.Envelope = json.RawMessage(envelope)
		records = append(records, record)
	}
	return records, rows.Err()
}

// Offset implements events.Log.
func (s *Store) Offset(ctx context.Context, tx platform.TransactionContext, consumer string) (int64, error) {
	if consumer == "" {
		return 0, platform.NewFailure(platform.CodeInvalidInput, errors.New("empty event consumer name"))
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var last int64
	err = executor.QueryRowContext(ctx, "SELECT last_seq FROM consumer_offsets WHERE consumer = $1", consumer).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return last, err
}

// advanceQuery upserts an offset monotonically. GREATEST makes a stale or replayed
// acknowledgement a no-op instead of a rewind, and updated_at only changes when the position
// does, so it records the last real progress.
const advanceQuery = `INSERT INTO consumer_offsets (consumer, last_seq, updated_at)
	VALUES ($1, $2, $3)
	ON CONFLICT (consumer) DO UPDATE SET
		last_seq = GREATEST(consumer_offsets.last_seq, EXCLUDED.last_seq),
		updated_at = CASE WHEN EXCLUDED.last_seq > consumer_offsets.last_seq
			THEN EXCLUDED.updated_at ELSE consumer_offsets.updated_at END`

// Advance implements events.Log. It stamps updated_at with primary database time.
func (s *Store) Advance(ctx context.Context, tx platform.TransactionContext, consumer string, seq int64) error {
	if consumer == "" || seq < 0 {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("event consumer offset needs a name and a non-negative seq, got seq "+strconv.FormatInt(seq, 10)))
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, advanceQuery, consumer, seq, now)
	return err
}

var _ events.Log = (*Store)(nil)
