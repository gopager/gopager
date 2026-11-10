package processing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// PublicationBinding comes from the current source-publication owner, never a compute worker.
// Its source fence is rechecked when the source owner refreshes the Transmission's processing status.
type PublicationBinding struct {
	Source    SourceIdentity
	MessageID uuid.UUID
	// MessageTime is the Message's time (first observation) at capture; it travels with the
	// completion fact so consumers can place the fact on the Message timeline.
	MessageTime time.Time
	SourceFence int64
}

// Validate rejects invented empty publication identities; actual source ownership is checked separately.
func (b PublicationBinding) Validate() error {
	if b.Source.Synthetic || b.Source.TransmissionID == uuid.Nil || b.Source.NetworkID == uuid.Nil ||
		b.Source.SourceRevision <= 0 || b.Source.ContextRevision <= 0 ||
		b.Source.OriginalTargetCount > MaxArguments || b.MessageID == uuid.Nil || b.MessageTime.IsZero() || b.SourceFence <= 0 {
		return invalid(errors.New("invalid source publication binding"))
	}
	return nil
}

// PublicationItem carries one candidate's current state, its actually saved definition and, for a
// selected completion, its private result. A candidate that is not completed, not selected or
// whose result is already published has nothing left to publish.
type PublicationItem struct {
	Candidate  Candidate
	Definition Definition
	Result     *PrivateResult
}

// Publishable reports whether the item holds a completed selected result that has not been
// attached to its Message yet.
func (i PublicationItem) Publishable() bool {
	return i.Candidate.State == "completed" && i.Candidate.Selected != nil && *i.Candidate.Selected &&
		i.Result != nil && i.Result.PublishedAt == nil
}

// PublicationRepository reads and applies current private evidence in the caller's source transaction.
// ReadPublicationItem returns authz.ErrNotFound when no candidate exists for the binding and a
// revision conflict when the binding's Workflow or configuration is no longer current; the
// coordinating source owner supplies the current candidate set and verifies its own fence.
type PublicationRepository interface {
	ReadPublicationItem(context.Context, platform.TransactionContext, CandidateBinding) (PublicationItem, error)
	ApplyPublicationItem(context.Context, platform.TransactionContext, PublicationBinding, PublicationItem) error
}
