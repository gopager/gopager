// Package publication coordinates private Processing evidence with the real source owner's barrier.
package publication

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// SourceOwner captures the complete current eligible set and validates actual source-publication fencing.
// CheckItem checks current Principal/scope/static result mode/target mask/Network dependencies.
// ApplyReadiness must atomically CAS the actual Message/target/contribution barrier and source fence.
type SourceOwner interface {
	CapturePublication(context.Context, platform.TransactionContext, uuid.UUID) (processing.PublicationBinding, []processing.CandidateBinding, error)
	CheckItem(context.Context, platform.TransactionContext, processing.PublicationBinding, processing.PublicationItem) error
	ApplyReadiness(context.Context, platform.TransactionContext, processing.PublicationBinding) error
}

// Service participates in the source owner's live transaction; it never creates Message identities.
type Service struct {
	repository processing.PublicationRepository
	source     SourceOwner
	facts      events.ProcessingAppender
	clock      platform.Clock
}

// New requires all publication participants; missing originals are not a successful no-op barrier.
func New(repository processing.PublicationRepository, source SourceOwner,
	facts events.ProcessingAppender, clock platform.Clock) (*Service, error) {
	if repository == nil || source == nil || facts == nil || clock == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing publication dependency"))
	}
	return &Service{repository: repository, source: source, facts: facts, clock: clock}, nil
}

// Publish applies all current evidence/facts and actual source readiness in the same caller transaction.
// Any failure requires rollback by the transaction owner; private completion alone is never sufficient.
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
		if err != nil {
			return err
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
		if item.Result == nil {
			continue
		}
		_, err = s.facts.AppendProcessingCompletion(ctx, tx, events.ProcessingCompletionFact{
			ID: uuid.New(), TransmissionID: transmission, MessageID: binding.MessageID,
			WorkflowID: item.Candidate.WorkflowID, DefinitionVersionID: item.Candidate.DefinitionVersionID,
			ResultID: item.Result.ID, ResultRevision: item.Result.Revision,
			SourceRevision: binding.Source.SourceRevision, ContextRevision: binding.Source.ContextRevision,
			OccurredAt: now, OutputSchema: item.Definition.OutputSchema, Result: item.Result.Evaluation,
		})
		if err != nil {
			return err
		}
	}
	return s.source.ApplyReadiness(ctx, tx, binding)
}

// MissingSourceOwner is a placeholder publication source owner for deployments without the real
// ingest source/readiness-barrier participant; every operation fails as unavailable.
type MissingSourceOwner struct{}

// CapturePublication rejects missing source ownership without inventing Message IDs or readiness.
func (MissingSourceOwner) CapturePublication(context.Context, platform.TransactionContext, uuid.UUID) (processing.PublicationBinding, []processing.CandidateBinding, error) {
	return processing.PublicationBinding{}, nil, unavailable()
}

// CheckItem rejects missing current source/Principal publication authority.
func (MissingSourceOwner) CheckItem(context.Context, platform.TransactionContext, processing.PublicationBinding, processing.PublicationItem) error {
	return unavailable()
}

// ApplyReadiness never substitutes a no-op for the actual source barrier.
func (MissingSourceOwner) ApplyReadiness(context.Context, platform.TransactionContext, processing.PublicationBinding) error {
	return unavailable()
}

func unavailable() error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("source publication owner unavailable"))
}
