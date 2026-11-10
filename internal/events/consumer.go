package events

import (
	"context"
	"encoding/json"

	"github.com/gopager/gopager/internal/platform"
)

// Support is a consumer's answer to whether it can process an event.
type Support int

const (
	// Unsupported means the consumer does not handle this event type; the runner advances
	// over the record without calling Handle.
	Unsupported Support = iota
	// UnknownVersion means the consumer handles the type but not this schemaVersion; the
	// runner blocks on the record and never skips it.
	UnknownVersion
	// Supported means the consumer handles this type and schemaVersion.
	Supported
)

// Consumer processes the fact log for one named purpose. Delivery is at-least-once: a
// handler may see a record again after a rolled-back batch or a crash before commit, so it
// deduplicates by the record's domain event ID plus its own Name in its own effect. The
// offset is no domain idempotency.
type Consumer interface {
	// Name returns the stable consumer name under which the offset is stored.
	Name() string
	// Supports classifies an event type and envelope schema version.
	Supports(eventType string, schemaVersion int) Support
	// Handle applies the record's effect inside tx, the transaction that also advances the
	// offset. Returning an error rolls the whole batch back and retries it on the next tick.
	Handle(ctx context.Context, tx platform.TransactionContext, record Record) error
}

// BatchPreparer is optionally implemented by a Consumer that must load something outside any
// transaction before it handles records, such as a cache that needs its own connection. The
// Runner calls Prepare before every batch; an error skips the batch until the next tick.
type BatchPreparer interface {
	Prepare(ctx context.Context) error
}

// SchemaVersion returns the envelope's schemaVersion, or 0 when the envelope has none or is
// malformed (0 is never a supported version).
func SchemaVersion(record Record) int {
	var envelope struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(record.Envelope, &envelope); err != nil {
		return 0
	}
	return envelope.SchemaVersion
}
