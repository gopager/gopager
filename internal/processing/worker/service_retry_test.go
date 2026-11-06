package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// replayTransactions runs the first callback to completion and then runs it again, as
// postgres.WithinTransaction does after a 40001/40P01 failure.
type replayTransactions struct{ replayed bool }

func (r *replayTransactions) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	tx := dispatchTransaction{ctx: ctx}
	if !r.replayed {
		r.replayed = true
		if err := callback(tx); err != nil {
			return err
		}
	}
	return callback(tx)
}

// countingSourceOwner hands out a fresh Capture per call and counts begins and releases.
type countingSourceOwner struct {
	lifecycleSourceOwner
	template Capture
	begun    atomic.Int32
	released atomic.Int32
}

func (s *countingSourceOwner) CaptureCandidate(context.Context, platform.TransactionContext,
	processing.Candidate) (Capture, error) {
	s.begun.Add(1)
	capture := s.template
	capture.Release = func() error { s.released.Add(1); return nil }
	return capture, nil
}

// TestRunReleasesEveryCaptureOfRetriedTransaction expects each capture begun by a rerun
// transaction callback to be released exactly once; a lost release keeps the execution registered
// until the instance shutdown deadline.
func TestRunReleasesEveryCaptureOfRetriedTransaction(t *testing.T) {
	now := time.Now().UTC()
	candidate := processing.Candidate{ID: uuid.New(), Revision: 1, State: "pending",
		CandidateBinding: processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: uuid.New(),
			DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1,
			WorkflowRevision: 1, ConfigurationRevision: 1}, CreatedAt: now, UpdatedAt: now, DueAt: now}
	source, err := lifecycleCapturedSource(t, candidate)
	if err != nil {
		t.Fatal(err)
	}
	owner := &countingSourceOwner{template: Capture{Definition: lifecycleDefinition(), Source: source,
		Scope: processing.EvaluationScope{SourceEligible: true}}}
	service := &Service{transactions: &replayTransactions{}, repository: &lifecycleRepository{pending: candidate},
		source: owner, id: uuid.New()}
	if _, _, err := service.Run(t.Context(), candidate); err != nil {
		t.Fatal(err)
	}
	if owner.begun.Load() != 2 || owner.released.Load() != owner.begun.Load() {
		t.Fatalf("captures begun=%d released=%d, want 2 begun and every one released", owner.begun.Load(), owner.released.Load())
	}
}
