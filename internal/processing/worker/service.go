// Package worker coordinates private pure-compute claims without holding SQL across evaluation.
package worker

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Capture contains owner-validated immutable input and original-position eligibility.
type Capture struct {
	Definition processing.Definition
	Source     *processing.CapturedSource
	// Outputs declares the Attribute fields the definition writes, resolved without strictness
	// so that a deprecation never stops the evaluation of an active version.
	Outputs     processing.Declarations
	Scope       processing.EvaluationScope
	Artifacts   processing.InstalledArtifacts
	Invocations processing.InstalledInvocationOwner
	// Release closes owner resources after the run has drained.
	Release func() error
}

// SourceOwner supplies current unpublished source/Principal/configuration authority under primary locks.
// Check rejects changed source revisions, eligibility, execution grants or instance pins.
type SourceOwner interface {
	CaptureCandidate(context.Context, platform.TransactionContext, processing.Candidate) (Capture, error)
	CheckCandidate(context.Context, platform.TransactionContext, processing.Candidate, Capture) error
}

// WorkSourceOwner reconciles current source/configuration before dispatch, recovery or supersession.
// It returns the actual persisted candidate after any owner-decided reselection/supersession.
type WorkSourceOwner interface {
	SourceOwner
	ReconcileCandidate(context.Context, platform.TransactionContext, processing.Candidate) (processing.Candidate, error)
}

// Publisher attaches completed private evidence to the Message and refreshes the Transmission's
// processing status in the same primary transaction as the candidate transition.
type Publisher interface {
	Publish(context.Context, platform.TransactionContext, uuid.UUID) error
}

// Repository combines existing private claim/failure/result transitions in the caller transaction.
type Repository interface {
	processing.CandidateRepository
	processing.CandidateFailureRepository
	processing.ResultRepository
}

// Service owns one worker identity; callers dispatch distinct due candidates using bounded concurrency.
type Service struct {
	transactions identity.Transactions
	repository   Repository
	source       SourceOwner
	regex        processing.RegexEvaluator
	id           uuid.UUID
	publisher    Publisher
	// renewEvery is the lease renewal period; zero selects processing.CandidateRenewInterval.
	renewEvery time.Duration
}

// WithRenewInterval overrides how often a held candidate lease is renewed. Production uses
// processing.CandidateRenewInterval; shorter values let tests observe periodic renewal without
// waiting. The interval must be positive and leave room for two missed renewals before
// processing.CandidateLeaseDuration expires. It must be called before the Service is shared.
func (s *Service) WithRenewInterval(interval time.Duration) error {
	if interval <= 0 || 3*interval > processing.CandidateLeaseDuration {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("candidate renew interval must be positive and at most a third of the lease duration"))
	}
	s.renewEvery = interval
	return nil
}

// renewInterval returns the effective lease renewal period.
func (s *Service) renewInterval() time.Duration {
	if s.renewEvery == 0 {
		return processing.CandidateRenewInterval
	}
	return s.renewEvery
}

// New requires real owner participants; an absent source cannot be treated as eligible work.
func New(transactions identity.Transactions, repository Repository, source SourceOwner,
	regex processing.RegexEvaluator, id uuid.UUID) (*Service, error) {
	if transactions == nil || repository == nil || source == nil || regex == nil || id == uuid.Nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing worker dependency"))
	}
	return &Service{transactions: transactions, repository: repository, source: source, regex: regex, id: id}, nil
}

// NewWithPublisher requires the atomic publication participant used by production dispatch.
func NewWithPublisher(transactions identity.Transactions, repository Repository, source SourceOwner,
	regex processing.RegexEvaluator, id uuid.UUID, publisher Publisher) (*Service, error) {
	if publisher == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing publication dependency"))
	}
	service, err := New(transactions, repository, source, regex, id)
	if err != nil {
		return nil, err
	}
	service.publisher = publisher
	return service, nil
}

// ExecuteWork reconciles selected metadata before claiming or recovering; completed evidence is never recomputed.
func (s *Service) ExecuteWork(ctx context.Context, selected processing.Candidate) error {
	if err := selected.Validate(); err != nil {
		return err
	}
	owner, ok := s.source.(WorkSourceOwner)
	if !ok {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("current candidate reconciliation owner unavailable"))
	}
	var current processing.Candidate
	var recovered bool
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		// A rerun transaction must not inherit state captured by the previous attempt.
		recovered = false
		var err error
		current, err = owner.ReconcileCandidate(tx.Context(), tx, selected)
		if err != nil {
			return err
		}
		if err := current.Validate(); err != nil {
			return err
		}
		if current.ID != selected.ID || current.TransmissionID != selected.TransmissionID {
			return platform.NewFailure(platform.CodeRevisionConflict, errors.New("reconciled candidate identity changed"))
		}
		if current.State != "running" {
			return nil
		}
		recovered = true
		recovery, ok := s.repository.(processing.CandidateRecoveryRepository)
		if !ok {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("candidate recovery participant unavailable"))
		}
		lease, err := current.Lease()
		if err != nil {
			return err
		}
		current, err = recovery.RecoverCandidate(tx.Context(), tx, lease)
		return err
	})
	if err != nil {
		return err
	}
	if current.State == "superseded" {
		if s.publisher == nil {
			return nil
		}
		return s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			return s.publisher.Publish(tx.Context(), tx, current.TransmissionID)
		})
	}
	if current.State != "pending" {
		return nil
	}
	// Recovery schedules a primary-clock retry; leave that future work to native due selection.
	if recovered {
		return nil
	}
	_, _, err = s.run(ctx, current, s.publisher != nil)
	return err
}

// MissingSourceOwner refuses dispatch until the Original owner supplies current eligibility and reconciliation.
type MissingSourceOwner struct{}

// CaptureCandidate does not substitute a synthetic source for an unavailable Original.
func (MissingSourceOwner) CaptureCandidate(context.Context, platform.TransactionContext, processing.Candidate) (Capture, error) {
	return Capture{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing source owner unavailable"))
}

// CheckCandidate rejects completion/renewal without actual current source authority.
func (MissingSourceOwner) CheckCandidate(context.Context, platform.TransactionContext, processing.Candidate, Capture) error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing source owner unavailable"))
}

// ReconcileCandidate rejects recovery/reselection without the real source owner.
func (MissingSourceOwner) ReconcileCandidate(context.Context, platform.TransactionContext, processing.Candidate) (processing.Candidate, error) {
	return processing.Candidate{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing source owner unavailable"))
}

// releaseCapture releases a held capture once and clears it, so a later call is a no-op.
func releaseCapture(captured *Capture) error {
	release := captured.Release
	*captured = Capture{}
	if release == nil {
		return nil
	}
	return release()
}

type outcome struct {
	evaluation processing.Evaluation
	err        error
}

// Run claims current work, evaluates outside SQL, renews ownership and stores only private evidence.
// Caller cancellation leaves lost work recoverable; it never fabricates publication readiness.
func (s *Service) Run(ctx context.Context, pending processing.Candidate) (
	completed processing.Candidate, evidence *processing.PrivateResult, resultErr error,
) {
	return s.run(ctx, pending, false)
}

func (s *Service) run(ctx context.Context, pending processing.Candidate, publish bool) (
	completed processing.Candidate, evidence *processing.PrivateResult, resultErr error,
) {
	if err := pending.Validate(); err != nil {
		return processing.Candidate{}, nil, err
	}
	var captured Capture
	var current processing.Candidate
	// Taken before the claim so the local lease deadline never exceeds the primary-clock lease.
	leaseStart := time.Now()
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		// A rerun after a serialization failure starts over; the previous attempt's capture is
		// still registered with the owner and would otherwise never be released.
		if err := releaseCapture(&captured); err != nil {
			return err
		}
		current = processing.Candidate{}
		var err error
		captured, err = s.source.CaptureCandidate(tx.Context(), tx, pending)
		if err != nil {
			return err
		}
		identity, err := captured.Source.Identity()
		if err != nil {
			return err
		}
		if identity.Synthetic {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("synthetic source cannot become a durable candidate"))
		}
		if identity.TransmissionID != pending.TransmissionID || identity.SourceRevision != pending.SourceRevision ||
			identity.ContextRevision != pending.ContextRevision {
			return platform.NewFailure(platform.CodeRevisionConflict, errors.New("captured candidate source differs"))
		}
		current, err = s.repository.ClaimCandidate(tx.Context(), tx, pending.ID, pending.Revision, s.id)
		return err
	})
	if err != nil {
		return processing.Candidate{}, nil, errors.Join(err, releaseCapture(&captured))
	}
	defer func() { resultErr = errors.Join(resultErr, releaseCapture(&captured)) }()
	compute, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan outcome, 1)
	go func() {
		var program *processing.WorkflowProgram
		var err error
		if captured.Artifacts == nil {
			program, err = processing.CompileWorkflow(compute, captured.Definition, captured.Source, captured.Outputs, s.regex)
		} else {
			program, err = processing.CompileWorkflowInstalled(compute, captured.Definition, captured.Source, captured.Outputs, s.regex, captured.Artifacts)
		}
		if err != nil {
			results <- outcome{err: err}
			return
		}
		evaluation, err := program.EvaluateScoped(compute, captured.Source, captured.Scope, captured.Invocations)
		results <- outcome{evaluation: evaluation, err: err}
	}()
	ticker := time.NewTicker(s.renewInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cancel()
			<-results
			return processing.Candidate{}, nil, ctx.Err()
		case <-ticker.C:
			attemptStart := time.Now()
			renewed, err := s.renew(ctx, current, captured)
			if err == nil {
				current, leaseStart = renewed, attemptStart
				continue
			}
			// A transient failure is retried at the next tick while the lease still outlives it;
			// anything else, or a lease that would lapse first, ends the claim.
			if transientFailure(err) && platform.FailureCode(err) != platform.CodeRevisionConflict &&
				attemptStart.Add(s.renewInterval()).Before(leaseStart.Add(processing.CandidateLeaseDuration)) {
				continue
			}
			cancel()
			<-results
			return processing.Candidate{}, nil, err
		case result := <-results:
			return s.finish(ctx, current, captured, result, publish)
		}
	}
}

// renew rechecks current source authority and extends the lease in one transaction. The renewed
// candidate is returned only after the transaction committed, so an uncertain commit leaves the
// previous lease reference in place.
func (s *Service) renew(ctx context.Context, current processing.Candidate, captured Capture) (processing.Candidate, error) {
	lease, err := current.Lease()
	if err != nil {
		return processing.Candidate{}, err
	}
	var renewed processing.Candidate
	err = s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		if err := s.source.CheckCandidate(tx.Context(), tx, current, captured); err != nil {
			return err
		}
		var err error
		renewed, err = s.repository.RenewCandidate(tx.Context(), tx, lease)
		return err
	})
	return renewed, err
}

// finish revalidates current source authority before a fenced completion or classified failure;
// with a publisher, either outcome also refreshes the Transmission's processing status.
func (s *Service) finish(ctx context.Context, current processing.Candidate, captured Capture,
	result outcome, publish bool) (processing.Candidate, *processing.PrivateResult, error) {
	lease, err := current.Lease()
	if err != nil {
		return processing.Candidate{}, nil, err
	}
	var completed processing.Candidate
	var evidence *processing.PrivateResult
	err = s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		if err := s.source.CheckCandidate(tx.Context(), tx, current, captured); err != nil {
			return err
		}
		if result.err != nil {
			code, retryable := classifyFailure(result.err)
			step, _ := processing.FailedStep(result.err)
			var err error
			completed, err = s.repository.FailCandidate(tx.Context(), tx, lease, code, step, retryable)
			if err != nil {
				return err
			}
		} else {
			source, err := captured.Source.Identity()
			if err != nil {
				return err
			}
			completed, evidence, err = s.repository.CompleteCandidate(tx.Context(), tx, lease, source.OriginalTargetCount, result.evaluation)
			if err != nil {
				return err
			}
		}
		if !publish {
			return nil
		}
		if s.publisher == nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing publication owner unavailable"))
		}
		return s.publisher.Publish(tx.Context(), tx, completed.TransmissionID)
	})
	if err != nil {
		return processing.Candidate{}, nil, err
	}
	if result.err != nil {
		return completed, nil, result.err
	}
	return completed, evidence, nil
}

// classifyFailure maps actual private errors to a finite stored vocabulary, never guest text.
func classifyFailure(err error) (platform.ErrorCode, bool) {
	code := platform.FailureCode(err)
	switch code {
	case platform.CodeInvalidInput, platform.CodeIncompatibleVersion, platform.CodeResourceLimit:
		return code, false
	case platform.CodeStorageUnavailable, platform.CodeAuthorityUnavailable:
		return code, true
	default:
		return platform.CodeInternalError, false
	}
}
