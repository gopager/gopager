package processing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

// failingRegex validates patterns like the Core adapter but fails every match.
type failingRegex struct{ err error }

func (f failingRegex) Regex(ctx context.Context, pattern string, text *string) (RegexResult, error) {
	if text == nil {
		return CoreRegex{}.Regex(ctx, pattern, nil)
	}
	return RegexResult{}, f.err
}

func extractFailureDefinition() Definition {
	definition := selectedDefinition()
	text := FieldRef{"source", []string{"text"}}
	definition.Steps = []Step{{ID: "extract", Kind: "extract", OutputSchema: map[string]FieldSchema{"level": {Kind: IntegerValue}},
		Extract: &ExtractStep{Input: text, Pattern: `LEVEL=(?P<level>[0-9]+)`,
			Captures: map[string]CaptureMapping{"level": {Name: "level", Conversion: IntegerValue}}}}}
	return definition
}

func TestFailingStepIsReportedWithItsIDAndKeepsTheMessageFormat(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t)
	program, err := CompileWorkflow(t.Context(), extractFailureDefinition(), source, testOutputs,
		failingRegex{err: platform.NewFailure(platform.CodeResourceLimit, errors.New("budget"))})
	if err != nil {
		t.Fatal(err)
	}
	_, err = program.Evaluate(t.Context(), source)
	if err == nil {
		t.Fatal("the failing step did not fail the evaluation")
	}
	var failure *StepFailure
	if !errors.As(err, &failure) || failure.StepID != "extract" {
		t.Fatalf("err = %v, want a StepFailure of step extract", err)
	}
	if step, ok := FailedStep(err); !ok || step != "extract" {
		t.Fatalf("FailedStep = %q, %v", step, ok)
	}
	if platform.FailureCode(err) != platform.CodeResourceLimit || !strings.HasPrefix(err.Error(), `step "extract": `) {
		t.Fatalf("code %s, message %q", platform.FailureCode(err), err)
	}
}

func TestFailedStepNamesTheInnermostStep(t *testing.T) {
	t.Parallel()
	inner := &StepFailure{StepID: "inner", Err: platform.NewFailure(platform.CodeInvalidInput, errors.New("x"))}
	outer := fmt.Errorf("ctx: %w", &StepFailure{StepID: "outer", Err: inner})
	if step, ok := FailedStep(outer); !ok || step != "inner" {
		t.Fatalf("FailedStep = %q, %v", step, ok)
	}
	if _, ok := FailedStep(errors.New("plain")); ok {
		t.Fatal("an error without a step failure names no step")
	}
}

func TestFailureReasonMapsCodes(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]string{
		"RESOURCE_LIMIT":         "resource-limit",
		"AUTHORITY_UNAVAILABLE":  "capability-unavailable",
		"DEPENDENCY_UNAVAILABLE": "capability-unavailable",
		"INCOMPATIBLE_VERSION":   "capability-unavailable",
		"INVALID_INPUT":          "conversion-failed",
		"INTERNAL_ERROR":         "evaluation-failed",
		"STORAGE_UNAVAILABLE":    "evaluation-failed",
		"anything":               "evaluation-failed",
		"":                       "evaluation-failed",
	} {
		if got := FailureReason(code); got != want {
			t.Errorf("FailureReason(%s) = %q, want %q", code, got, want)
		}
	}
}
