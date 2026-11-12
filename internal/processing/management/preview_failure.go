package management

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// PreviewFailure explains why a preview's evaluation failed, without any raw error text.
type PreviewFailure struct {
	// Code is the error code of the failure.
	Code string `json:"code"`
	// Reason is the reason that Code stands for: resource-limit, capability-unavailable,
	// conversion-failed or evaluation-failed.
	Reason string `json:"reason"`
	// StepID is the ID of the failing Workflow step, when a step failed.
	StepID string `json:"stepId,omitempty"`
}

// PreviewResult is the outcome of a preview that ran: the evaluation, or the explanation of why
// the evaluation failed. Request and authorization errors are not results; they are errors.
type PreviewResult struct {
	// Evaluation is the evaluation when Failure is nil.
	Evaluation processing.Evaluation
	// Failure explains a failed evaluation.
	Failure *PreviewFailure
}

// MarshalJSON encodes the evaluation as it is, or `{"failure": {...}}` for a failed evaluation.
func (r PreviewResult) MarshalJSON() ([]byte, error) {
	if r.Failure != nil {
		return json.Marshal(struct {
			Failure *PreviewFailure `json:"failure"`
		}{r.Failure})
	}
	return json.Marshal(r.Evaluation)
}

// previewFailure classifies an error of an evaluation. A failure of a Workflow step and a resource
// limit are explained; anything else (authorization, cancellation, storage) is not a property of
// the evaluated Workflow and stays an error.
func previewFailure(err error) (*PreviewFailure, bool) {
	step, failedStep := processing.FailedStep(err)
	code := platform.FailureCode(err)
	var failure *platform.Failure
	if errors.Is(err, context.Canceled) || !errors.As(err, &failure) || (!failedStep && code != platform.CodeResourceLimit) {
		return nil, false
	}
	return &PreviewFailure{Code: string(code), Reason: processing.FailureReason(string(code)), StepID: step}, true
}
