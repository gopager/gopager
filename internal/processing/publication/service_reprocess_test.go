package publication_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/publication"
)

type fakeTx struct{}

func (fakeTx) Context() context.Context { return context.Background() }

type fakeClock struct{}

func (fakeClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC), nil
}

// fakeSource returns one fixed publication binding and candidate.
type fakeSource struct {
	binding   processing.PublicationBinding
	candidate processing.CandidateBinding
}

func (s fakeSource) CapturePublication(context.Context, platform.TransactionContext, uuid.UUID) (processing.PublicationBinding, []processing.CandidateBinding, error) {
	return s.binding, []processing.CandidateBinding{s.candidate}, nil
}

func (fakeSource) CheckItem(context.Context, platform.TransactionContext, processing.PublicationBinding, processing.PublicationItem) error {
	return nil
}

func (fakeSource) ApplyProcessingStatus(context.Context, platform.TransactionContext, processing.PublicationBinding) error {
	return nil
}

// fakeRepository records what the service applies and replaces.
type fakeRepository struct {
	item     processing.PublicationItem
	applied  int
	replaced int
	// retract is how many other results a replacement withdraws.
	retract int
}

func (r *fakeRepository) ReadPublicationItem(context.Context, platform.TransactionContext, processing.CandidateBinding) (processing.PublicationItem, error) {
	return r.item, nil
}

func (r *fakeRepository) ApplyPublicationItem(context.Context, platform.TransactionContext, processing.PublicationBinding, processing.PublicationItem) error {
	r.applied++
	return nil
}

func (r *fakeRepository) ReplacePublished(context.Context, platform.TransactionContext, processing.PublicationItem) ([]processing.RetractedResult, error) {
	r.replaced++
	results := make([]processing.RetractedResult, r.retract)
	for i := range results {
		results[i] = processing.RetractedResult{ID: uuid.New(), Revision: 5}
	}
	return results, nil
}

type fakeFacts struct {
	completions []events.ProcessingCompletionFact
	retractions []events.ProcessingRetractionFact
}

func (f *fakeFacts) AppendProcessingRetraction(_ context.Context, _ platform.TransactionContext, fact events.ProcessingRetractionFact) (uuid.UUID, error) {
	f.retractions = append(f.retractions, fact)
	return uuid.New(), nil
}

func (*fakeFacts) AppendProcessingConfiguration(context.Context, platform.TransactionContext, events.ProcessingConfigurationFact) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeFacts) AppendProcessingCompletion(_ context.Context, _ platform.TransactionContext, fact events.ProcessingCompletionFact) (uuid.UUID, error) {
	f.completions = append(f.completions, fact)
	return uuid.New(), nil
}

// publishItem runs Publish for one candidate state and returns what the service did.
func publishItem(t *testing.T, state string, selected, marked bool, withResult bool, retract int) (*fakeRepository, *fakeFacts) {
	t.Helper()
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	binding := processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: uuid.New(),
		DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1, WorkflowRevision: 2, ConfigurationRevision: 2}
	candidate := processing.Candidate{CandidateBinding: binding, ID: uuid.New(), Revision: 2, State: state,
		FencingToken: 1, CreatedAt: now, UpdatedAt: now, DueAt: now}
	if state == "completed" {
		candidate.Selected = &selected
	}
	item := processing.PublicationItem{Candidate: candidate}
	if withResult {
		item.Result = &processing.PrivateResult{ID: uuid.New(), CandidateID: candidate.ID, Revision: 2, ProducingFence: 1}
	}
	if marked {
		mark := uuid.New()
		item.ReprocessID = &mark
	}
	repository, facts := &fakeRepository{item: item, retract: retract}, &fakeFacts{}
	source := fakeSource{candidate: binding, binding: processing.PublicationBinding{
		Source: processing.SourceIdentity{TransmissionID: binding.TransmissionID, NetworkID: uuid.New(),
			SourceRevision: 1, ContextRevision: 1},
		MessageID: uuid.New(), MessageTime: now, SourceFence: 1}}
	service, err := publication.New(repository, source, facts, fakeClock{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Publish(context.Background(), fakeTx{}, binding.TransmissionID); err != nil {
		t.Fatal(err)
	}
	return repository, facts
}

func TestPublishReplacesEarlierResultsForMarkedCandidate(t *testing.T) {
	t.Parallel()
	repository, facts := publishItem(t, "completed", true, true, true, 1)
	if repository.applied != 1 || repository.replaced != 1 {
		t.Fatalf("applied=%d replaced=%d, want 1 and 1", repository.applied, repository.replaced)
	}
	if len(facts.completions) != 1 || !facts.completions[0].Historical {
		t.Fatalf("completions = %+v, want one historical fact", facts.completions)
	}
	if len(facts.retractions) != 1 {
		t.Fatalf("retractions = %+v, want one: every withdrawal continues its result's aggregate with a fact", facts.retractions)
	}
}

func TestPublishNotSelectedMarkedCandidateRetractsWithoutFact(t *testing.T) {
	t.Parallel()
	repository, facts := publishItem(t, "completed", false, true, false, 2)
	if repository.applied != 0 || repository.replaced != 1 || len(facts.completions) != 0 {
		t.Fatalf("applied=%d replaced=%d facts=%d, want 0, 1 and 0", repository.applied, repository.replaced, len(facts.completions))
	}
	if len(facts.retractions) != 1 || len(facts.retractions[0].ResultIDs) != 2 {
		t.Fatalf("retractions = %+v, want one fact naming both withdrawn results", facts.retractions)
	}
}

func TestPublishNotSelectedMarkedCandidateWithNothingToRetractAppendsNoFact(t *testing.T) {
	t.Parallel()
	_, facts := publishItem(t, "completed", false, true, false, 0)
	if len(facts.retractions) != 0 || len(facts.completions) != 0 {
		t.Fatalf("retractions=%d completions=%d, want no fact", len(facts.retractions), len(facts.completions))
	}
}

func TestPublishFailedMarkedCandidateReplacesNothing(t *testing.T) {
	t.Parallel()
	repository, facts := publishItem(t, "failed", false, true, false, 0)
	if repository.applied != 0 || repository.replaced != 0 || len(facts.completions) != 0 {
		t.Fatalf("applied=%d replaced=%d facts=%d, want all zero", repository.applied, repository.replaced, len(facts.completions))
	}
}

func TestPublishUnmarkedCandidateKeepsNormalFact(t *testing.T) {
	t.Parallel()
	repository, facts := publishItem(t, "completed", true, false, true, 0)
	if repository.applied != 1 || repository.replaced != 0 {
		t.Fatalf("applied=%d replaced=%d, want 1 and 0", repository.applied, repository.replaced)
	}
	if len(facts.completions) != 1 || facts.completions[0].Historical {
		t.Fatalf("completions = %+v, want one normal fact", facts.completions)
	}
}
