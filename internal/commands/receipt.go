// Package commands owns nonsecret actor-bound administrative creation receipts.
package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// Result is the replayable outcome stored in a Receipt: the created or changed
// item's ID and resulting revision, plus per-item results for batch commands.
type Result struct {
	ID       uuid.UUID    `json:"id"`
	Revision int64        `json:"revision,string"`
	Items    []ResultItem `json:"items,omitempty"`
}

// ResultItem is one item's outcome within a batch Result; Status is optional
// and command-specific.
type ResultItem struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision,string"`
	Status   string    `json:"status,omitempty"`
}

// Receipt retains no input snapshot or secret and is not a login receipt.
type Receipt struct {
	ActorID     uuid.UUID
	CommandID   uuid.UUID
	Operation   string
	Fingerprint [32]byte
	Result      Result
	RecordedAt  time.Time
}

// Repository participates in the same transaction as the command's domain change.
// Compact receipts remain while referenced; this slice performs no receipt expiry/pruning.
type Repository interface {
	ReadCommandReceipt(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID) (Receipt, bool, error)
	SaveCommandReceipt(context.Context, platform.TransactionContext, Receipt) error
}

// Fingerprint encodes validated typed values; callers canonicalize set-valued lists first.
func Fingerprint(input any) ([32]byte, error) {
	value, err := json.Marshal(input)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(value), nil
}

// ErrIdempotencyConflict reports that an actor reused a command ID for a
// different operation or different input. It is not retryable: repeating the
// same request cannot succeed, and the client must choose a new command ID.
var ErrIdempotencyConflict = errors.New("command ID already used with different input")

// Check distinguishes the same operation/input from conflicting reuse by one
// actor. It returns the stored Result for an identical replay and
// ErrIdempotencyConflict when the operation or input fingerprint differs.
func (r Receipt) Check(operation string, fingerprint [32]byte) (Result, error) {
	if r.Operation != operation || r.Fingerprint != fingerprint {
		return Result{}, ErrIdempotencyConflict
	}
	return r.Result, nil
}

// NextRevision distinguishes stale state from exhaustion before incrementing a current revision.
func NextRevision(current, expected int64) (int64, error) {
	if current <= 0 || current != expected {
		return 0, authz.ErrRevisionConflict
	}
	if current == math.MaxInt64 {
		return 0, platform.NewFailure(platform.CodeResourceLimit, errors.New("configuration revision exhausted"))
	}
	return current + 1, nil
}
