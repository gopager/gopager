package processing

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// PublicationBinding comes from the current source-publication owner, never a compute worker.
// Its source fence must be rechecked atomically when the source owner applies readiness.
type PublicationBinding struct {
	Source      SourceIdentity
	MessageID   uuid.UUID
	SourceFence int64
}

// Validate rejects invented empty publication identities; actual source ownership is checked separately.
func (b PublicationBinding) Validate() error {
	if b.Source.Synthetic || b.Source.TransmissionID == uuid.Nil || b.Source.NetworkID == uuid.Nil ||
		b.Source.SourceRevision <= 0 || b.Source.ContextRevision <= 0 ||
		b.Source.OriginalTargetCount > MaxArguments || b.MessageID == uuid.Nil || b.SourceFence <= 0 {
		return invalid(errors.New("invalid source publication binding"))
	}
	return nil
}

// PublicationItem carries completed private work and its actually saved definition.
// A false selection has no Result, but still participates in the source barrier.
type PublicationItem struct {
	Candidate  Candidate
	Definition Definition
	Result     *PrivateResult
}

// PublicationRepository reads and applies current private evidence in the caller's source transaction.
// The coordinating source owner supplies the complete current eligible set and verifies its own fence.
type PublicationRepository interface {
	ReadPublicationItem(context.Context, platform.TransactionContext, CandidateBinding) (PublicationItem, error)
	ApplyPublicationItem(context.Context, platform.TransactionContext, PublicationBinding, PublicationItem) error
}
