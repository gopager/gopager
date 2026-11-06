package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
)

type syntheticCandidateSource struct{ source *processing.CapturedSource }

// CaptureCandidate deliberately presents a synthetic source to the durable-worker boundary.
func (s syntheticCandidateSource) CaptureCandidate(context.Context, platform.TransactionContext,
	processing.Candidate) (Capture, error) {
	return Capture{Source: s.source}, nil
}

// CheckCandidate is unreachable because synthetic capture must fail before claiming work.
func (syntheticCandidateSource) CheckCandidate(context.Context, platform.TransactionContext,
	processing.Candidate, Capture) error {
	panic("synthetic source reached candidate completion")
}

func TestWorkerRejectsSyntheticSourceBeforeClaimOrEvaluation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	pending := processing.Candidate{ID: uuid.New(), Revision: 1, State: "pending",
		CandidateBinding: processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: uuid.New(),
			DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1,
			WorkflowRevision: 1, ConfigurationRevision: 1}, CreatedAt: now, UpdatedAt: now, DueAt: now}
	source, err := processing.CaptureSource(t.Context(), processing.SourceProjection{
		TransmissionID: pending.TransmissionID, NetworkID: uuid.New(), SourceRevision: 1, ContextRevision: 1,
		Synthetic: true, Protocol: protocols.POCSAG, Form: "tone-only", ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{transactions: &dispatchFixture{}, source: syntheticCandidateSource{source: source}}
	_, result, err := service.Run(t.Context(), pending)
	if platform.FailureCode(err) != platform.CodeAuthorityUnavailable || result != nil {
		t.Fatalf("synthetic capture reached durable evaluation: result=%v err=%v", result, err)
	}
}
