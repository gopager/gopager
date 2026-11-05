package platform

import (
	"errors"
	"log/slog"
)

// ErrorCode is the stable, locale-independent identifier of a failure class
// exposed to callers and API clients.
type ErrorCode string

// Platform error codes. CodeCommitUncertain means a commit may or may not have
// taken effect, so callers must look the operation up by its identifier rather
// than blindly retrying; CodeInternalError is the fallback for unknown codes and
// for errors that are not a *Failure.
const (
	CodeInvalidInput         ErrorCode = "INVALID_INPUT"
	CodeRevisionConflict     ErrorCode = "REVISION_CONFLICT"
	CodeIncompatibleVersion  ErrorCode = "INCOMPATIBLE_VERSION"
	CodeResourceLimit        ErrorCode = "RESOURCE_LIMIT"
	CodeStorageUnavailable   ErrorCode = "STORAGE_UNAVAILABLE"
	CodeAuthorityUnavailable ErrorCode = "AUTHORITY_UNAVAILABLE"
	CodeCommitUncertain      ErrorCode = "COMMIT_UNCERTAIN"
	CodeInternalError        ErrorCode = "INTERNAL_ERROR"
)

var englishFallbacks = map[ErrorCode]string{
	CodeInvalidInput:         "Check the supplied values.",
	CodeRevisionConflict:     "This item changed. Reload before saving.",
	CodeIncompatibleVersion:  "This version is not compatible.",
	CodeResourceLimit:        "The operation exceeds a configured limit.",
	CodeStorageUnavailable:   "Storage is not available for this operation.",
	CodeAuthorityUnavailable: "Current write or access authority is not available.",
	CodeCommitUncertain:      "The operation may have been committed. Check its identifier.",
	CodeInternalError:        "The operation could not be completed.",
}

// Failure is the platform error type: a public ErrorCode plus a private cause.
// Error returns only the code's generic English fallback text, so the cause is
// never leaked through messages or structured logs; it remains reachable via
// errors.Unwrap/errors.Is/errors.As. All methods are safe on a nil *Failure,
// which behaves like CodeInternalError.
type Failure struct {
	code  ErrorCode
	cause error
}

// NewFailure returns a Failure with the given code and cause. An unknown code is
// replaced by CodeInternalError. cause may be nil.
func NewFailure(code ErrorCode, cause error) *Failure {
	if _, ok := englishFallbacks[code]; !ok {
		code = CodeInternalError
	}
	return &Failure{code: code, cause: cause}
}

// Error returns the generic English fallback text for the failure's code; it
// never includes the wrapped cause.
func (f *Failure) Error() string {
	if f == nil {
		return englishFallbacks[CodeInternalError]
	}
	return englishFallbacks[f.code]
}

// Code returns the failure's error code, or CodeInternalError for a nil receiver.
func (f *Failure) Code() ErrorCode {
	if f == nil {
		return CodeInternalError
	}
	return f.code
}

// Unwrap returns the private cause, or nil for a nil receiver.
func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.cause
}

// FailureCode returns the code of the first *Failure in err's chain. It returns
// "" for a nil error and CodeInternalError for any non-nil error without a
// *Failure.
func FailureCode(err error) ErrorCode {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Code()
	}
	if err == nil {
		return ""
	}
	return CodeInternalError
}

// TextKey identifies the locale-independent platform error catalog entry.
func (f *Failure) TextKey() string { return "error." + string(f.Code()) }

// Retryable reports proven transient failures; uncertain commits require receipt lookup.
func (f *Failure) Retryable() bool {
	return f.Code() == CodeStorageUnavailable || f.Code() == CodeAuthorityUnavailable || f.Code() == CodeRevisionConflict
}

// LogValue prevents structured logging from serializing a private wrapped cause.
func (f *Failure) LogValue() slog.Value {
	return slog.GroupValue(slog.String("code", string(f.Code())), slog.String("fallback", f.Error()))
}
