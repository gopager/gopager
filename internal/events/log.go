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

// Position is a read position in the fact log. It orders by (TxID, Seq), the order in which
// Log.ReadAfter delivers records. The zero Position precedes every record.
//
// Seq alone is not a safe position: PostgreSQL assigns a transaction ID at the transaction's
// first write, not at the event insert, so a transaction with a higher TxID can append a lower
// Seq than a transaction with a lower TxID. Delivery in Seq order after a snapshot cut would
// then skip the lower Seq once the higher transaction commits. TxID order has no such hole:
// a reader only returns rows whose TxID is below the oldest in-flight transaction, so no row
// at or below a returned TxID can still appear later.
type Position struct {
	// TxID is the PostgreSQL xid8 of the transaction that appended the record.
	TxID uint64
	// Seq is the record's Seq; it breaks ties between records of one transaction.
	Seq int64
}

// Compare returns -1, 0 or 1 when p orders before, equal to or after other.
func (p Position) Compare(other Position) int {
	switch {
	case p.TxID < other.TxID:
		return -1
	case p.TxID > other.TxID:
		return 1
	case p.Seq < other.Seq:
		return -1
	case p.Seq > other.Seq:
		return 1
	}
	return 0
}

// Record is one fact read back from the sequenced event log.
type Record struct {
	// Seq is the database-assigned event number; it is unique, strictly increasing per row
	// and the human-facing event ID. It is not a safe read position on its own, see Position.
	Seq int64
	// TxID is the PostgreSQL xid8 of the transaction that appended the record.
	TxID uint64
	// ID is the domain event ID, equal to the envelope's domainEventId.
	ID uuid.UUID
	// Type is the envelope event type. It is empty for rows without a type in their envelope.
	Type string
	// OccurredAt is the envelope's occurredAt. It is the zero time for rows without one.
	OccurredAt time.Time
	// Envelope is the canonical classified envelope exactly as stored.
	Envelope json.RawMessage
}

// Position returns the read position of this record; reading after it resumes with the next.
func (r Record) Position() Position { return Position{TxID: r.TxID, Seq: r.Seq} }

// Log reads the sequenced fact log and keeps per-consumer read positions. Every method runs
// in the caller's active transaction and fails with an invalid-input failure once that
// transaction has ended, so a consumer can commit its effect and Advance atomically.
//
// A consumer combines ReadAfter with a wake-up source (the NotifyChannel listener or a
// periodic poll): after each wake-up it loops ReadAfter from its Offset until a page comes
// back empty, then Advances to the last Seq it processed.
type Log interface {
	// ReadAfter returns up to limit records positioned after the given Position, ordered by
	// (TxID, Seq). It returns only records whose writing transaction is older than every
	// transaction still in flight, so no record at or below a returned Position can appear
	// later. Consumers can therefore resume from the Position of the last record they saw
	// without skipping facts, even when a later-started transaction appended a lower Seq.
	// limit must be positive and after.Seq must not be negative.
	ReadAfter(ctx context.Context, tx platform.TransactionContext, after Position, limit int) ([]Record, error)
	// Offset returns the Position the named consumer has advanced to, or the zero Position if
	// it has none yet.
	Offset(ctx context.Context, tx platform.TransactionContext, consumer string) (Position, error)
	// Advance moves the named consumer's offset forward to position. It never moves an offset
	// backwards in (TxID, Seq) order: a position at or below the stored one is a successful
	// no-op, so concurrent or replayed acknowledgements cannot cause reprocessing.
	// position.Seq must not be negative.
	Advance(ctx context.Context, tx platform.TransactionContext, consumer string, position Position) error
}
