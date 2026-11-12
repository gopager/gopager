package processing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// PrivateResult is immutable completed evaluation evidence. PublishedAt and MessageID are set
// once the result has been attached to its Transmission's Message and its completion fact appended.
// RetractedAt is set when a historical Reprocess withdrew the published result; a retracted
// result is detached from its Message and never publishes again.
type PrivateResult struct {
	ID             uuid.UUID
	CandidateID    uuid.UUID
	Revision       int64
	ProducingFence int64
	Evaluation     Evaluation
	CompletedAt    time.Time
	PublishedAt    *time.Time
	MessageID      *uuid.UUID
	RetractedAt    *time.Time
}

// ResultRepository atomically commits a fenced outcome and optional private result in the caller transaction.
// The worker's current source owner supplies the actual original target count and validates current authority.
type ResultRepository interface {
	CompleteCandidate(context.Context, platform.TransactionContext, CandidateLease, uint32, Evaluation) (Candidate, *PrivateResult, error)
}
