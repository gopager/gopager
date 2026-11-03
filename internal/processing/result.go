package processing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// PrivateResult is immutable completed evaluation evidence, not a ready Message or completion fact.
type PrivateResult struct {
	ID             uuid.UUID
	CandidateID    uuid.UUID
	Revision       int64
	ProducingFence int64
	Evaluation     Evaluation
	CompletedAt    time.Time
}

// ResultRepository atomically commits a fenced outcome and optional private result in the caller transaction.
// The worker's current source owner supplies the actual original target count and validates current authority.
type ResultRepository interface {
	CompleteCandidate(context.Context, platform.TransactionContext, CandidateLease, uint32, Evaluation) (Candidate, *PrivateResult, error)
}
