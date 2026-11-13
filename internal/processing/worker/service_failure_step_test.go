package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
)

// failingMatcher validates patterns like the Core adapter and fails every match.
type failingMatcher struct{ err error }

func (f failingMatcher) Regex(ctx context.Context, pattern string, text *string) (processing.RegexResult, error) {
	if text == nil {
		return processing.CoreRegex{}.Regex(ctx, pattern, nil)
	}
	return processing.RegexResult{}, f.err
}

// TestRunStoresTheFailingStep proves that a Candidate failed by a Workflow step is stored with
// the ID of that step.
func TestRunStoresTheFailingStep(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	candidate := processing.Candidate{ID: uuid.New(), Revision: 1, State: "pending",
		CandidateBinding: processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: uuid.New(),
			DefinitionVersionID: uuid.New(), SourceRevision: 1, ContextRevision: 1,
			WorkflowRevision: 1, ConfigurationRevision: 1}, CreatedAt: now, UpdatedAt: now, DueAt: now}
	network := uuid.New()
	dependency := processing.Dependency{TransmissionID: candidate.TransmissionID, NetworkID: network,
		SourceRevision: 1, ContextRevision: 1, Groups: []string{"payload"}}
	flag, _ := processing.Literal(processing.BooleanValue, true)
	text, _ := processing.Literal(processing.StringValue, "LEVEL=7")
	present := func(value processing.Value) processing.FieldValue {
		return processing.FieldValue{State: processing.FieldPresent, Value: &value, Dependencies: []processing.Dependency{dependency}}
	}
	source, err := processing.CaptureSource(t.Context(), processing.SourceProjection{
		TransmissionID: candidate.TransmissionID, NetworkID: network, SourceRevision: 1, ContextRevision: 1,
		Protocol: protocols.POCSAG, Form: "text", ObservedAt: now,
		Fields: []processing.CapturedField{
			{Reference: processing.FieldRef{Namespace: "source", Path: []string{"flag"}},
				Schema: processing.FieldSchema{Kind: processing.BooleanValue}, Field: present(flag)},
			{Reference: processing.FieldRef{Namespace: "source", Path: []string{"text"}},
				Schema: processing.FieldSchema{Kind: processing.StringValue}, Field: present(text)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := lifecycleDefinition()
	definition.Steps = []processing.Step{{ID: "extract-level", Kind: "extract",
		OutputSchema: map[string]processing.FieldSchema{"level": {Kind: processing.IntegerValue}},
		Extract: &processing.ExtractStep{Input: processing.FieldRef{Namespace: "source", Path: []string{"text"}},
			Pattern:  `LEVEL=(?P<level>[0-9]+)`,
			Captures: map[string]processing.CaptureMapping{"level": {Name: "level", Conversion: processing.IntegerValue}}}}}
	owner := lifecycleSourceOwner{capture: Capture{Definition: definition, Source: source,
		Scope: processing.EvaluationScope{SourceEligible: true}}}
	repository := &lifecycleRepository{pending: candidate}
	service := &Service{transactions: &dispatchFixture{}, repository: repository, source: owner, id: uuid.New(),
		regex: failingMatcher{err: platform.NewFailure(platform.CodeResourceLimit, errors.New("budget"))}}

	_, _, runErr := service.Run(t.Context(), candidate)
	if runErr == nil || repository.failCalls != 1 {
		t.Fatalf("run error=%v failure calls=%d", runErr, repository.failCalls)
	}
	if repository.failedStep != "extract-level" {
		t.Fatalf("stored failing step = %q, want extract-level", repository.failedStep)
	}
}
