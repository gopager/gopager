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
	// ReprocessID is the historical Reprocess that readmitted the candidate and still awaits its
	// replacement of the earlier published results; nil for ordinary work.
	ReprocessID *uuid.UUID
	// Outputs declares the Attribute fields a result may hold, as current in the reading
	// transaction. It is not part of the stored evidence.
	Outputs Declarations `json:"-"`
}

// Publishable reports whether the item holds a completed selected result that has not been
// attached to its Message yet and has not been retracted.
func (i PublicationItem) Publishable() bool {
	return i.Candidate.State == "completed" && i.Candidate.Selected != nil && *i.Candidate.Selected &&
		i.Result != nil && i.Result.PublishedAt == nil && i.Result.RetractedAt == nil
}

// ReplacesPublished reports whether the item is a completed candidate readmitted by a historical
// Reprocess, whose publication replaces the earlier published results of its Transmission and
// Workflow. A failed marked candidate replaces nothing.
func (i PublicationItem) ReplacesPublished() bool {
	return i.ReprocessID != nil && i.Candidate.State == "completed"
}

// PublicationRepository reads and applies current private evidence in the caller's source transaction.
// ReadPublicationItem returns authz.ErrNotFound when no candidate exists for the binding and a
// revision conflict when the binding's Workflow or configuration is no longer current; the
// coordinating source owner supplies the current candidate set and verifies its own fence.
type PublicationRepository interface {
	ReadPublicationItem(context.Context, platform.TransactionContext, CandidateBinding) (PublicationItem, error)
	ApplyPublicationItem(context.Context, platform.TransactionContext, PublicationBinding, PublicationItem) error
	// ReplacePublished retracts every published result of the item's Transmission and Workflow
	// except the item's own result and clears the candidate's Reprocess mark, in the caller's
	// transaction. It applies to items for which ReplacesPublished holds.
	// It returns the retracted results.
	ReplacePublished(context.Context, platform.TransactionContext, PublicationItem) ([]RetractedResult, error)
}
