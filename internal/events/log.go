package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// NotifyChannel is the PostgreSQL NOTIFY channel on which every append announces the seq of
// its new row. The payload is only a wake-up hint: it may be lost or arrive before the row is
// readable, so consumers must always read through Log.ReadAfter and never trust the payload.
const NotifyChannel = "gopager_domain_events"

// Record is one fact read back from the sequenced event log.
type Record struct {
	// Seq is the database-assigned log position; it is unique and strictly increasing per row.
	Seq int64
	// ID is the domain event ID, equal to the envelope's domainEventId.
	ID uuid.UUID
	// Type is the envelope event type. It is empty for rows without a type in their envelope.
	Type string
	// OccurredAt is the envelope's occurredAt. It is the zero time for rows without one.
	OccurredAt time.Time
	// Envelope is the canonical classified envelope exactly as stored.
	Envelope json.RawMessage
}

// Log reads the sequenced fact log and keeps per-consumer read positions. Every method runs
// in the caller's active transaction and fails with an invalid-input failure once that
// transaction has ended, so a consumer can commit its effect and Advance atomically.
//
// A consumer combines ReadAfter with a wake-up source (the NotifyChannel listener or a
// periodic poll): after each wake-up it loops ReadAfter from its Offset until a page comes
// back empty, then Advances to the last Seq it processed.
type Log interface {
	// ReadAfter returns up to limit records with Seq greater than lastSeq, ordered by Seq. It
	// returns only records whose writing transaction is older than every transaction still in
	// flight, so a record is never returned before a lower-seq record that is not yet
	// committed. Consumers can therefore resume from the last Seq they saw without skipping
	// facts. limit must be positive.
	ReadAfter(ctx context.Context, tx platform.TransactionContext, lastSeq int64, limit int) ([]Record, error)
	// Offset returns the last seq the named consumer has advanced to, or 0 if it has none yet.
	Offset(ctx context.Context, tx platform.TransactionContext, consumer string) (int64, error)
	// Advance moves the named consumer's offset forward to seq. It never moves an offset
	// backwards: a seq at or below the stored one is a successful no-op, so concurrent or
	// replayed acknowledgements cannot cause reprocessing. seq must not be negative.
	Advance(ctx context.Context, tx platform.TransactionContext, consumer string, seq int64) error
}
