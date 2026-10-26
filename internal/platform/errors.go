package platform

import (
	"errors"
	"log/slog"
)

type ErrorCode string

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

type Failure struct {
	code  ErrorCode
	cause error
}

func NewFailure(code ErrorCode, cause error) *Failure {
	if _, ok := englishFallbacks[code]; !ok {
		code = CodeInternalError
	}
	return &Failure{code: code, cause: cause}
}

func (f *Failure) Error() string {
	if f == nil {
		return englishFallbacks[CodeInternalError]
	}
	return englishFallbacks[f.code]
}

func (f *Failure) Code() ErrorCode {
	if f == nil {
		return CodeInternalError
	}
	return f.code
}

func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.cause
}

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
