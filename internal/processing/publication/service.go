// Package publication attaches completed private Processing evidence to its Message and refreshes
// the Transmission's processing status in the source owner's transaction.
package publication

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// SourceOwner captures the current selected Workflow set and validates actual source fencing.
// CheckItem checks current Principal/scope/static result mode/target mask/Network dependencies.
// ApplyProcessingStatus rechecks the source fence and derives the Transmission's processing
// status from its current candidates.
type SourceOwner interface {
	CapturePublication(context.Context, platform.TransactionContext, uuid.UUID) (processing.PublicationBinding, []processing.CandidateBinding, error)
	CheckItem(context.Context, platform.TransactionContext, processing.PublicationBinding, processing.PublicationItem) error
	ApplyProcessingStatus(context.Context, platform.TransactionContext, processing.PublicationBinding) error
}

// Service participates in the source owner's live transaction; it never creates Message identities.
type Service struct {
	repository processing.PublicationRepository
	source     SourceOwner
	facts      events.ProcessingAppender
	clock      platform.Clock
}

// New requires all publication participants; a missing source owner is never a successful no-op.
func New(repository processing.PublicationRepository, source SourceOwner,
	facts events.ProcessingAppender, clock platform.Clock) (*Service, error) {
	if repository == nil || source == nil || facts == nil || clock == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing publication dependency"))
	}
	return &Service{repository: repository, source: source, facts: facts, clock: clock}, nil
}

// Publish attaches every completed, selected and not yet published result of the Transmission's
// current candidates to its Message, appends their completion facts and refreshes the
// Transmission's processing status, all in the caller's transaction. Candidates that are still
// pending, running, failed or not selected are left alone; a candidate missing for a current
// Workflow is not admitted here. A completed candidate that a historical Reprocess readmitted
// additionally retracts the earlier published results of its Workflow, also when it is not
// selected, and its completion fact is marked historical so that Delivery does not fan it out.
// Any failure requires rollback by the transaction owner.
func (s *Service) Publish(ctx context.Context, tx platform.TransactionContext, transmission uuid.UUID) error {
	if transmission == uuid.Nil {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("missing publication source"))
	}
	binding, candidates, err := s.source.CapturePublication(ctx, tx, transmission)
	if err != nil {
		return err
	}
	if err := binding.Validate(); err != nil {
		return err
	}
	if binding.Source.TransmissionID != transmission {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("publication source substituted"))
	}
	detached := slices.Clone(candidates)
	seen := make(map[uuid.UUID]bool)
	now, err := s.clock.Now(ctx, tx)
	if err != nil {
		return err
	}
	for _, candidate := range detached {
		if err := candidate.Validate(); err != nil {
			return err
		}
		if seen[candidate.WorkflowID] || candidate.TransmissionID != transmission ||
			candidate.SourceRevision != binding.Source.SourceRevision ||
			candidate.ContextRevision != binding.Source.ContextRevision {
			return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid current publication candidate set"))
		}
		seen[candidate.WorkflowID] = true
		item, err := s.repository.ReadPublicationItem(ctx, tx, candidate)
		if errors.Is(err, authz.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if !item.Publishable() {
			if item.ReplacesPublished() {
				// Nothing new to attach, but the Reprocess still withdraws the earlier results.
				retracted, err := s.repository.ReplacePublished(ctx, tx, item)
				if err != nil {
					return err
				}
				// No completion fact accompanies this withdrawal, so announce it.
				if err := s.announceRetraction(ctx, tx, binding, item, retracted, now); err != nil {
					return err
				}
			}
			continue
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			return err
		}
		var check processing.PublicationItem
		// Copy trusted stored components without imposing one request limit on their combined size.
		if err := json.Unmarshal(encoded, &check); err != nil {
			return err
		}
		if err := s.source.CheckItem(ctx, tx, binding, check); err != nil {
			return err
		}
		if err := s.repository.ApplyPublicationItem(ctx, tx, binding, item); err != nil {
			return err
		}
		replaces := item.ReplacesPublished()
		if replaces {
			// Every withdrawal continues its result's aggregate with a fact, also when a
			// replacement is published in the same transaction.
			retracted, err := s.repository.ReplacePublished(ctx, tx, item)
			if err != nil {
				return err
			}
			if err := s.announceRetraction(ctx, tx, binding, item, retracted, now); err != nil {
				return err
			}
		}
		_, err = s.facts.AppendProcessingCompletion(ctx, tx, events.ProcessingCompletionFact{
			ID: uuid.New(), TransmissionID: transmission, MessageID: binding.MessageID, MessageTime: binding.MessageTime,
			WorkflowID: item.Candidate.WorkflowID, DefinitionVersionID: item.Candidate.DefinitionVersionID,
			ResultID: item.Result.ID, ResultRevision: item.Result.Revision,
			SourceRevision: binding.Source.SourceRevision, ContextRevision: binding.Source.ContextRevision,
			OccurredAt: now, Outputs: item.Outputs, Result: item.Result.Evaluation, Historical: replaces,
		})
		if err != nil {
			return err
		}
	}
	return s.source.ApplyProcessingStatus(ctx, tx, binding)
}

// announceRetraction appends the retraction fact of one Transmission and Workflow when results were
// withdrawn.
func (s *Service) announceRetraction(ctx context.Context, tx platform.TransactionContext, binding processing.PublicationBinding,
	item processing.PublicationItem, retracted []processing.RetractedResult, now time.Time) error {
	if len(retracted) == 0 {
		return nil
	}
	_, err := s.facts.AppendProcessingRetraction(ctx, tx, events.NewProcessingRetraction(item.Candidate.TransmissionID,
		binding.MessageID, binding.MessageTime, item.Candidate.WorkflowID, retracted, now))
	return err
}

// MissingSourceOwner is a placeholder publication source owner for deployments without the real
// ingest source participant; every operation fails as unavailable.
type MissingSourceOwner struct{}

// CapturePublication rejects missing source ownership without inventing Message IDs or a status.
func (MissingSourceOwner) CapturePublication(context.Context, platform.TransactionContext, uuid.UUID) (processing.PublicationBinding, []processing.CandidateBinding, error) {
	return processing.PublicationBinding{}, nil, unavailable()
}

// CheckItem rejects missing current source/Principal publication authority.
func (MissingSourceOwner) CheckItem(context.Context, platform.TransactionContext, processing.PublicationBinding, processing.PublicationItem) error {
	return unavailable()
}

// ApplyProcessingStatus never substitutes a no-op for the actual source status refresh.
func (MissingSourceOwner) ApplyProcessingStatus(context.Context, platform.TransactionContext, processing.PublicationBinding) error {
	return unavailable()
}

func unavailable() error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("source publication owner unavailable"))
}
