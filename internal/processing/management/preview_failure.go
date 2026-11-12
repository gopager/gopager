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
// limit are explained when their code describes the evaluated Workflow; authorization, session,
// storage and concurrency failures (also when a step raised them) and cancellation are not a
// property of the Workflow and stay errors, so they keep their HTTP status and retry semantics.
func previewFailure(err error) (*PreviewFailure, bool) {
	step, failedStep := processing.FailedStep(err)
	code := platform.FailureCode(err)
	var failure *platform.Failure
	if errors.Is(err, context.Canceled) || !errors.As(err, &failure) || !explainedCode(code) ||
		(!failedStep && code != platform.CodeResourceLimit) {
		return nil, false
	}
	return &PreviewFailure{Code: string(code), Reason: processing.FailureReason(string(code)), StepID: step}, true
}

// explainedCode reports whether a failure code describes the evaluated Workflow: invalid or
// unconvertible data, a limit, an unavailable or incompatible capability, or a failing step.
func explainedCode(code platform.ErrorCode) bool {
	switch code {
	case platform.CodeInvalidInput, platform.CodeResourceLimit, platform.CodeIncompatibleVersion,
		platform.CodeAuthorityUnavailable, platform.CodeInternalError:
		return true
	}
	return false
}
