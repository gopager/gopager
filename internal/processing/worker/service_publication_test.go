package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

type publicationRepositoryFixture struct {
	*lifecycleRepository
	completionTransaction platform.TransactionContext
}

func (r *publicationRepositoryFixture) CompleteCandidate(ctx context.Context, tx platform.TransactionContext,
	lease processing.CandidateLease, targetCount uint32, result processing.Evaluation,
) (processing.Candidate, *processing.PrivateResult, error) {
	r.completionTransaction = tx
	return r.lifecycleRepository.CompleteCandidate(ctx, tx, lease, targetCount, result)
}

type publicationFixture struct {
	transaction  platform.TransactionContext
	transmission uuid.UUID
	err          error
	calls        int
}

func (p *publicationFixture) Publish(_ context.Context, tx platform.TransactionContext, transmission uuid.UUID) error {
	p.calls++
	p.transaction = tx
	p.transmission = transmission
	return p.err
}

func TestFinishPublishesCompletionInTheSameTransaction(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "publication succeeds"},
		{name: "publication failure rolls completion back", err: errors.New("publication failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC()
			pending := processing.Candidate{ID: uuid.New(), Revision: 1, State: "pending",
				CandidateBinding: processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: uuid.New(),
					DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1,
					WorkflowRevision: 1, ConfigurationRevision: 1}, CreatedAt: now, UpdatedAt: now, DueAt: now}
			current := pending
			current.Revision++
			current.State, current.FencingToken = "running", 1
			workerID := uuid.New()
			leaseUntil := now.Add(processing.CandidateLeaseDuration)
			current.WorkerID, current.LeaseUntil = &workerID, &leaseUntil
			source, err := lifecycleCapturedSource(t, pending)
			if err != nil {
				t.Fatal(err)
			}
			repository := &publicationRepositoryFixture{lifecycleRepository: &lifecycleRepository{pending: pending}}
			publisher := &publicationFixture{err: test.err}
			service := &Service{transactions: &dispatchFixture{}, repository: repository,
				source: lifecycleSourceOwner{}, publisher: publisher, id: workerID}

			_, _, finishErr := service.finish(t.Context(), current, Capture{Source: source},
				outcome{evaluation: processing.Evaluation{Selected: true}}, true)
			if test.err != nil {
				if !errors.Is(finishErr, test.err) {
					t.Fatalf("finish error=%v want %v", finishErr, test.err)
				}
			} else if finishErr != nil {
				t.Fatalf("finish returned error: %v", finishErr)
			}
			if publisher.calls != 1 || publisher.transmission != pending.TransmissionID {
				t.Fatalf("publication calls=%d transmission=%s", publisher.calls, publisher.transmission)
			}
			if repository.completionTransaction == nil || repository.completionTransaction.Context() != publisher.transaction.Context() {
				t.Fatal("candidate completion and publication did not share a transaction")
			}
		})
	}
}
