package management

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// RetrySource verifies current source eligibility and source/execution-Principal pins, and
// refreshes the Transmission's processing status after the retry through Publish. Missing or
// stale owners must return an explicit error, never assume an eligible original.
type RetrySource interface {
	CheckRetrySource(context.Context, platform.TransactionContext, processing.Candidate) error
	Publish(context.Context, platform.TransactionContext, uuid.UUID) error
}

// RetryService performs operator-requested manual retries of failed processing
// candidates. Each call runs in its own primary transaction with a fresh proof check.
type RetryService struct {
	management *Service
	repository processing.CandidateRetryRepository
	source     RetrySource
}

// NewRetry composes current proof/configuration management with required work/source owners.
func NewRetry(management *Service, repository processing.CandidateRetryRepository, source RetrySource) (*RetryService, error) {
	if management == nil || repository == nil || source == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing retry dependency"))
	}
	return &RetryService{management: management, repository: repository, source: source}, nil
}

// RetryFailed authorizes a protected current-source manual retry without granting Payload access.
func (s *RetryService) RetryFailed(ctx context.Context, proof identity.Proof, parent, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (processing.Candidate, error) {
	if parent == uuid.Nil || id == uuid.Nil || expected <= 0 {
		return processing.Candidate{}, authz.ErrInvalidInput
	}
	var result processing.Candidate
	err := s.management.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadRetryCandidate(tx.Context(), tx, authority, parent, id, now)
		if err != nil {
			return err
		}
		if current.Revision != expected {
			return authz.ErrRevisionConflict
		}
		if err := s.source.CheckRetrySource(tx.Context(), tx, current); err != nil {
			return err
		}
		result, err = s.repository.RetryCandidate(tx.Context(), tx, authority, parent, id, expected)
		if err != nil {
			return err
		}
		return s.source.Publish(tx.Context(), tx, result.TransmissionID)
	})
	return result, err
}
