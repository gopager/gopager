package worker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// renewalRepository scripts one result per renewal attempt on top of the lifecycle fixture.
type renewalRepository struct {
	lifecycleRepository
	script   []error
	attempts int
}

// RenewCandidate replays the scripted failures, then extends the lease like the real store.
func (r *renewalRepository) RenewCandidate(_ context.Context, _ platform.TransactionContext,
	lease processing.CandidateLease) (processing.Candidate, error) {
	index := r.attempts
	r.attempts++
	if index < len(r.script) && r.script[index] != nil {
		return processing.Candidate{}, r.script[index]
	}
	now := time.Now().UTC()
	until := now.Add(processing.CandidateLeaseDuration)
	candidate := r.pending
	candidate.Revision, candidate.State, candidate.FencingToken = lease.Revision+1, "running", lease.Fence
	candidate.WorkerID, candidate.LeaseUntil, candidate.UpdatedAt = &lease.WorkerID, &until, now
	return candidate, nil
}

// blockedArtifacts keeps the evaluation running until released so renewals can tick.
type blockedArtifacts struct{ release <-chan struct{} }

// PrepareInstalled blocks until release, then fails the evaluation with a non-retryable code.
func (a blockedArtifacts) PrepareInstalled(ctx context.Context, _ uuid.UUID, _ int64) (*processing.InstalledArtifact, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.release:
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("fixture module unavailable"))
	}
}

func TestRunToleratesTransientRenewalFailureInsideLease(t *testing.T) {
	t.Parallel()
	failure := func(code platform.ErrorCode) error { return platform.NewFailure(code, errors.New("renewal failed")) }
	for _, test := range []struct {
		name         string
		script       []error
		aborted      bool
		wantAttempts int
	}{
		{name: "one storage outage", script: []error{failure(platform.CodeStorageUnavailable)}, wantAttempts: 2},
		{name: "one authority outage", script: []error{failure(platform.CodeAuthorityUnavailable)}, wantAttempts: 2},
		{name: "one uncertain commit", script: []error{failure(platform.CodeCommitUncertain)}, wantAttempts: 2},
		{name: "revision conflict", script: []error{failure(platform.CodeRevisionConflict)}, aborted: true, wantAttempts: 1},
		{name: "other failure", script: []error{errors.New("boom")}, aborted: true, wantAttempts: 1},
		{name: "lease would expire", aborted: true, wantAttempts: 2, script: []error{
			failure(platform.CodeStorageUnavailable), failure(platform.CodeStorageUnavailable)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				now := time.Now().UTC()
				candidate := processing.Candidate{ID: uuid.New(), Revision: 1, State: "pending",
					CandidateBinding: processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: uuid.New(),
						DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1,
						WorkflowRevision: 1, ConfigurationRevision: 1}, CreatedAt: now, UpdatedAt: now, DueAt: now}
				source, err := lifecycleCapturedSource(t, candidate)
				if err != nil {
					t.Fatal(err)
				}
				release := make(chan struct{})
				definition := lifecycleDefinition()
				definition.Steps = []processing.Step{{ID: "installed", Kind: "installed",
					OutputSchema: map[string]processing.FieldSchema{},
					Installed:    &processing.InstalledStep{InstanceID: uuid.New(), ConfigurationRevision: 1}}}
				owner := lifecycleSourceOwner{capture: Capture{Definition: definition, Source: source,
					Artifacts: blockedArtifacts{release: release},
					Scope:     processing.EvaluationScope{SourceEligible: true}}}
				repository := &renewalRepository{lifecycleRepository: lifecycleRepository{pending: candidate}, script: test.script}
				service := &Service{transactions: &dispatchFixture{}, repository: repository, source: owner, id: uuid.New()}
				done := make(chan error, 1)
				go func() {
					_, _, err := service.Run(t.Context(), candidate)
					done <- err
				}()
				// Ticks fire at 20 s and 40 s; the evaluation outlives both.
				time.Sleep(2*processing.CandidateRenewInterval + time.Second)
				synctest.Wait()
				if !test.aborted {
					close(release)
				}
				synctest.Wait()
				err = <-done
				switch {
				case test.aborted && err == nil:
					t.Fatal("renewal failure did not abort the evaluation")
				case test.aborted && repository.failCalls != 0:
					t.Fatalf("aborted run recorded a failure: %d", repository.failCalls)
				case !test.aborted && (platform.FailureCode(err) != platform.CodeInvalidInput || repository.failCalls != 1):
					t.Fatalf("transient renewal failure aborted the evaluation: err=%v failCalls=%d", err, repository.failCalls)
				}
				if repository.attempts != test.wantAttempts {
					t.Fatalf("renewal attempts=%d want %d", repository.attempts, test.wantAttempts)
				}
			})
		})
	}
}
