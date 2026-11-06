package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
)

type lifecycleSourceOwner struct {
	capture      Capture
	captureErr   error
	afterCapture func()
}

func (s lifecycleSourceOwner) CaptureCandidate(context.Context, platform.TransactionContext,
	processing.Candidate) (Capture, error) {
	if s.afterCapture != nil {
		s.afterCapture()
	}
	return s.capture, s.captureErr
}

func (lifecycleSourceOwner) CheckCandidate(context.Context, platform.TransactionContext,
	processing.Candidate, Capture) error {
	return nil
}

type lifecycleRepository struct {
	pending       processing.Candidate
	completeCalls int
	failCalls     int
}

func (r *lifecycleRepository) AdmitCandidate(context.Context, platform.TransactionContext,
	processing.CandidateBinding) (processing.Candidate, error) {
	return processing.Candidate{}, errors.New("unexpected candidate admission")
}

func (r *lifecycleRepository) ClaimCandidate(_ context.Context, _ platform.TransactionContext,
	_ uuid.UUID, _ int64, worker uuid.UUID) (processing.Candidate, error) {
	candidate := r.pending
	leaseUntil := candidate.UpdatedAt.Add(processing.CandidateLeaseDuration)
	candidate.Revision++
	candidate.State, candidate.FencingToken = "running", 1
	candidate.WorkerID, candidate.LeaseUntil = &worker, &leaseUntil
	return candidate, nil
}

func (r *lifecycleRepository) RenewCandidate(context.Context, platform.TransactionContext,
	processing.CandidateLease) (processing.Candidate, error) {
	return processing.Candidate{}, errors.New("unexpected candidate renewal")
}

func (r *lifecycleRepository) FailCandidate(context.Context, platform.TransactionContext,
	processing.CandidateLease, platform.ErrorCode, bool) (processing.Candidate, error) {
	r.failCalls++
	return processing.Candidate{}, nil
}

func (r *lifecycleRepository) CompleteCandidate(_ context.Context, _ platform.TransactionContext,
	_ processing.CandidateLease, _ uint32, result processing.Evaluation) (processing.Candidate, *processing.PrivateResult, error) {
	r.completeCalls++
	selected := result.Selected
	candidate := r.pending
	candidate.State, candidate.Selected = "completed", &selected
	return candidate, &processing.PrivateResult{ID: uuid.New()}, nil
}

func TestRunReleasesCapturedResourcesAcrossOutcomes(t *testing.T) {
	t.Parallel()
	releaseErr := errors.New("release failed")
	for _, test := range []struct {
		name       string
		failed     bool
		canceled   bool
		releaseErr error
	}{
		{name: "success"},
		{name: "evaluation failure", failed: true},
		{name: "canceled", canceled: true},
		{name: "release failure", releaseErr: releaseErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC()
			candidate := processing.Candidate{ID: uuid.New(), Revision: 1, State: "pending",
				CandidateBinding: processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: uuid.New(),
					DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1,
					WorkflowRevision: 1, ConfigurationRevision: 1}, CreatedAt: now, UpdatedAt: now, DueAt: now}
			source, err := lifecycleCapturedSource(t, candidate)
			if err != nil {
				t.Fatal(err)
			}
			definition := lifecycleDefinition()
			if test.failed {
				definition = processing.Definition{}
			}
			ctx := t.Context()
			var cancel context.CancelFunc
			if test.canceled {
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
			}
			var releases atomic.Int32
			owner := lifecycleSourceOwner{capture: Capture{Definition: definition, Source: source,
				Scope: processing.EvaluationScope{SourceEligible: true},
				Release: func() error {
					releases.Add(1)
					return test.releaseErr
				}}}
			if test.canceled {
				owner.afterCapture = cancel
			}
			repository := &lifecycleRepository{pending: candidate}
			service := &Service{transactions: &dispatchFixture{}, repository: repository,
				source: owner, id: uuid.New()}

			_, _, runErr := service.Run(ctx, candidate)
			if releases.Load() != 1 {
				t.Fatalf("release count=%d want 1", releases.Load())
			}
			switch {
			case test.canceled:
				if !errors.Is(runErr, context.Canceled) {
					t.Fatalf("run error=%v want context canceled", runErr)
				}
			case test.failed:
				if runErr == nil || repository.failCalls != 1 {
					t.Fatalf("run error=%v failure calls=%d", runErr, repository.failCalls)
				}
			case test.releaseErr != nil:
				if !errors.Is(runErr, test.releaseErr) || repository.completeCalls != 1 {
					t.Fatalf("run error=%v completion calls=%d", runErr, repository.completeCalls)
				}
			default:
				if runErr != nil || repository.completeCalls != 1 {
					t.Fatalf("run error=%v completion calls=%d", runErr, repository.completeCalls)
				}
			}
		})
	}
}

func lifecycleCapturedSource(t *testing.T, candidate processing.Candidate) (*processing.CapturedSource, error) {
	t.Helper()
	network := uuid.New()
	dependency := processing.Dependency{TransmissionID: candidate.TransmissionID, NetworkID: network,
		SourceRevision: candidate.SourceRevision, ContextRevision: candidate.ContextRevision, Groups: []string{"payload"}}
	value, err := processing.Literal(processing.BooleanValue, true)
	if err != nil {
		return nil, err
	}
	return processing.CaptureSource(t.Context(), processing.SourceProjection{
		TransmissionID: candidate.TransmissionID, NetworkID: network, SourceRevision: candidate.SourceRevision,
		ContextRevision: candidate.ContextRevision, Protocol: protocols.POCSAG, Form: "text",
		ObservedAt: candidate.CreatedAt,
		Fields: []processing.CapturedField{{Reference: processing.FieldRef{Namespace: "source", Path: []string{"flag"}},
			Schema: processing.FieldSchema{Kind: processing.BooleanValue},
			Field:  processing.FieldValue{State: processing.FieldPresent, Value: &value, Dependencies: []processing.Dependency{dependency}}}},
	})
}

func lifecycleDefinition() processing.Definition {
	field := processing.FieldRef{Namespace: "source", Path: []string{"flag"}}
	value, _ := processing.Literal(processing.BooleanValue, true)
	return processing.Definition{SchemaVersion: 1, Selector: processing.Predicate{Op: "eq", Field: &field, Value: &value},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
}
