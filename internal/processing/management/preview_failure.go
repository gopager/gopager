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
// A failure while the caller's own context has ended (cancelled or past its deadline) is the
// caller's, not the Workflow's, and stays an error too.
func previewFailure(caller context.Context, err error) (*PreviewFailure, bool) {
	if caller.Err() != nil {
		return nil, false
	}
	step, failedStep := processing.FailedStep(err)
	code := platform.FailureCode(err)
	var failure *platform.Failure
	if errors.Is(err, context.Canceled) || !errors.As(err, &failure) || !explained(code, err) ||
		(!failedStep && code != platform.CodeResourceLimit) {
		return nil, false
	}
	return &PreviewFailure{Code: string(code), Reason: processing.FailureReason(string(code)), StepID: step}, true
}

// explained reports whether a failure describes the evaluated Workflow: invalid or unconvertible
// data, a limit, an incompatible version, or an unavailable plugin capability. AUTHORITY_UNAVAILABLE
// counts only with processing.ErrCapabilityUnavailable; without it the authority itself is out and
// the request must fail retryably. An internal error is a fault of the server, not of the Workflow,
// and stays an error so that it is logged and reported.
func explained(code platform.ErrorCode, err error) bool {
	switch code {
	case platform.CodeInvalidInput, platform.CodeResourceLimit, platform.CodeIncompatibleVersion:
		return true
	case platform.CodeAuthorityUnavailable:
		return errors.Is(err, processing.ErrCapabilityUnavailable)
	}
	return false
}

// explainAfterRecheck returns the explanation of an evaluation error, or the error itself when it
// is not explained. An explanation is returned only after recheck, the final authority and source
// fence a successful result also passes, so an editor who lost the right or the source during the
// evaluation gets that error instead of an explanation.
func explainAfterRecheck(caller context.Context, err error, recheck func() error) (*PreviewFailure, error) {
	failure, ok := previewFailure(caller, err)
	if !ok {
		return nil, err
	}
	if err := recheck(); err != nil {
		return nil, err
	}
	return failure, nil
}
