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

// readAfterQuery selects log rows past a (tx_id, seq) position. Both values are assigned at
// different times: seq at the event insert, tx_id at the transaction's first write, which may
// come earlier. A transaction with a higher tx_id can therefore hold a lower seq, so reading
// by seq alone can deliver a higher seq and later lose a lower one that commits afterwards.
// Ordering by (tx_id, seq) and restricting rows to transactions older than the oldest
// still-running transaction (the snapshot's xmin) guarantees that no row at or below a
// returned position can still appear. The xid8 position travels as text because the driver
// has no native xid8 type.
const readAfterQuery = `SELECT seq, tx_id::text, id, coalesce(type, ''), occurred_at, envelope
	FROM domain_events
	WHERE (tx_id, seq) > ($1::text::xid8, $2::bigint)
		AND tx_id < pg_snapshot_xmin(pg_current_snapshot())
	ORDER BY tx_id, seq
	LIMIT $3`

// ReadAfter implements events.Log. A transaction never reads facts it appended itself,
// because its own transaction is still in flight; a consumer that appends facts while
// consuming sees them on a later read.
func (s *Store) ReadAfter(ctx context.Context, tx platform.TransactionContext, after events.Position, limit int) (records []events.Record, err error) {
	if after.Seq < 0 || limit <= 0 {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("event log read needs a non-negative position and a positive limit"))
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, readAfterQuery, strconv.FormatUint(after.TxID, 10), after.Seq, limit)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var (
			record     events.Record
			txID       string
			occurredAt sql.NullTime
			envelope   []byte
		)
		if err := rows.Scan(&record.Seq, &txID, &record.ID, &record.Type, &occurredAt, &envelope); err != nil {
			return nil, err
		}
		if record.TxID, err = strconv.ParseUint(txID, 10, 64); err != nil {
			return nil, err
		}
		record.OccurredAt = occurredAt.Time
		record.Envelope = json.RawMessage(envelope)
		records = append(records, record)
	}
	return records, rows.Err()
}

// Offset implements events.Log.
func (s *Store) Offset(ctx context.Context, tx platform.TransactionContext, consumer string) (events.Position, error) {
	if consumer == "" {
		return events.Position{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("empty event consumer name"))
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return events.Position{}, err
	}
	var (
		position events.Position
		txID     string
	)
	err = executor.QueryRowContext(ctx, "SELECT last_tx_id::text, last_seq FROM consumer_offsets WHERE consumer = $1", consumer).Scan(&txID, &position.Seq)
	if errors.Is(err, sql.ErrNoRows) {
		return events.Position{}, nil
	}
	if err != nil {
		return events.Position{}, err
	}
	position.TxID, err = strconv.ParseUint(txID, 10, 64)
	return position, err
}

// advanceQuery upserts an offset monotonically in (last_tx_id, last_seq) order. The WHERE
// clause makes a stale or replayed acknowledgement a no-op instead of a rewind, and updated_at
// only changes when the position does, so it records the last real progress.
const advanceQuery = `INSERT INTO consumer_offsets (consumer, last_tx_id, last_seq, updated_at)
	VALUES ($1, $2::text::xid8, $3, $4)
	ON CONFLICT (consumer) DO UPDATE SET
		last_tx_id = EXCLUDED.last_tx_id,
		last_seq = EXCLUDED.last_seq,
		updated_at = EXCLUDED.updated_at
	WHERE (EXCLUDED.last_tx_id, EXCLUDED.last_seq) > (consumer_offsets.last_tx_id, consumer_offsets.last_seq)`

// Advance implements events.Log. It stamps updated_at with primary database time.
func (s *Store) Advance(ctx context.Context, tx platform.TransactionContext, consumer string, position events.Position) error {
	if consumer == "" || position.Seq < 0 {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("event consumer offset needs a name and a non-negative seq, got seq "+strconv.FormatInt(position.Seq, 10)))
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, advanceQuery, consumer, strconv.FormatUint(position.TxID, 10), position.Seq, now)
	return err
}

var _ events.Log = (*Store)(nil)
