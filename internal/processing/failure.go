package processing

import (
	"errors"
	"fmt"
)

// The reasons a failed evaluation is explained with. They are the suffixes of the
// locale-independent text keys `processing.<reason>`.
const (
	// ReasonResourceLimit is a bound that the evaluation exceeded.
	ReasonResourceLimit = "resource-limit"
	// ReasonCapabilityUnavailable is a capability or dependency the evaluation needs and cannot reach.
	ReasonCapabilityUnavailable = "capability-unavailable"
	// ReasonConversionFailed is a value that could not be converted or was invalid.
	ReasonConversionFailed = "conversion-failed"
	// ReasonEvaluationFailed is every other failure.
	ReasonEvaluationFailed = "evaluation-failed"
)

// MaxFailureStepBytes bounds a stored failing step ID; a longer ID is not stored.
const MaxFailureStepBytes = 256

// StepFailure is the failure of one Workflow step. Its message is `step "<id>": <cause>` and its
// cause keeps the platform failure code, so callers classify it like the cause.
type StepFailure struct {
	// StepID is the ID of the failing step.
	StepID string
	// Err is the cause.
	Err error
}

// Error implements error.
func (e *StepFailure) Error() string { return fmt.Sprintf("step %q: %v", e.StepID, e.Err) }

// Unwrap returns the cause.
func (e *StepFailure) Unwrap() error { return e.Err }

// FailedStep returns the ID of the step that failed, which is the innermost one when steps are
// nested in a branch. It reports false when err names no step.
func FailedStep(err error) (string, bool) {
	var step string
	var found bool
	for err != nil {
		var failure *StepFailure
		if !errors.As(err, &failure) {
			break
		}
		step, found, err = failure.StepID, true, failure.Err
	}
	return step, found
}

// FailureReason maps an error code (RESOURCE_LIMIT, AUTHORITY_UNAVAILABLE, DEPENDENCY_UNAVAILABLE,
// INCOMPATIBLE_VERSION, INVALID_INPUT, …) to the reason an editor or a status read explains a
// failure with. It never exposes the raw error text.
func FailureReason(code string) string {
	switch code {
	case "RESOURCE_LIMIT":
		return ReasonResourceLimit
	case "AUTHORITY_UNAVAILABLE", "DEPENDENCY_UNAVAILABLE", "INCOMPATIBLE_VERSION":
		return ReasonCapabilityUnavailable
	case "INVALID_INPUT":
		return ReasonConversionFailed
	default:
		return ReasonEvaluationFailed
	}
}
