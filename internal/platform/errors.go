package platform

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

// ErrorCode is the stable, locale-independent identifier of a failure class
// exposed to callers and API clients.
type ErrorCode string

// Platform error codes. CodeCommitUncertain means a commit may or may not have
// taken effect, so callers must look the operation up by its identifier rather
// than blindly retrying; CodeInternalError is the fallback for unknown codes and
// for errors that are not a *Failure. CodeLoginRequired, CodeForbidden,
// CodeNotFound and CodeIdempotencyConflict are the domain outcomes whose
// sentinel errors are Failure values, so the code survives a transaction
// boundary instead of being reported as a storage outage.
const (
	CodeLoginRequired        ErrorCode = "LOGIN_REQUIRED"
	CodeForbidden            ErrorCode = "FORBIDDEN"
	CodeNotFound             ErrorCode = "NOT_FOUND"
	CodeIdempotencyConflict  ErrorCode = "IDEMPOTENCY_CONFLICT"
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
	CodeLoginRequired:        "Sign in to continue.",
	CodeForbidden:            "This action is not permitted.",
	CodeNotFound:             "The requested item is not available.",
	CodeIdempotencyConflict:  "This operation identifier was already used differently.",
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
	// permanent marks a failure whose code is normally transient but which a
	// repeat of the same request cannot resolve.
	permanent bool
}

// NewFailure returns a Failure with the given code and cause. An unknown code is
// replaced by CodeInternalError. cause may be nil.
func NewFailure(code ErrorCode, cause error) *Failure {
	if _, ok := englishFallbacks[code]; !ok {
		code = CodeInternalError
	}
	return &Failure{code: code, cause: cause}
}

// NewPermanentFailure is NewFailure for a failure that Retryable must report as
// false although its code is normally retryable, for example a unique-constraint
// conflict: resending the same request hits the same constraint again.
func NewPermanentFailure(code ErrorCode, cause error) *Failure {
	failure := NewFailure(code, cause)
	failure.permanent = true
	return failure
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
// A failure created with NewPermanentFailure is never retryable. For
// CodeRevisionConflict, retryable means "reload the current state, then repeat
// with a fresh expected revision", not a blind resend.
func (f *Failure) Retryable() bool {
	if f == nil || f.permanent {
		return false
	}
	return f.Code() == CodeStorageUnavailable || f.Code() == CodeAuthorityUnavailable || f.Code() == CodeRevisionConflict
}

// LogValue prevents structured logging from serializing a private wrapped cause.
func (f *Failure) LogValue() slog.Value {
	return slog.GroupValue(slog.String("code", string(f.Code())), slog.String("fallback", f.Error()))
}

// HTTPStatus returns the HTTP status the API uses for a platform error code; unknown
// codes, including CodeInternalError, map to 500. Keeping the table here prevents the
// transport layers from drifting apart.
func HTTPStatus(code ErrorCode) int {
	switch code {
	case CodeInvalidInput:
		return http.StatusBadRequest
	case CodeLoginRequired:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeRevisionConflict, CodeIncompatibleVersion, CodeIdempotencyConflict:
		return http.StatusConflict
	case CodeResourceLimit:
		return http.StatusRequestEntityTooLarge
	case CodeAuthorityUnavailable, CodeStorageUnavailable, CodeCommitUncertain:
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// LogErrorResponse records that an error response with the given public code,
// HTTP status and requestId was sent, so an operator can find the server-side
// record for a client's requestId. Server errors (5xx) log at error level, client
// errors at info level. Only these three non-secret values are logged: never the
// request, its body or the private cause.
func LogErrorResponse(code string, status int, requestID string) {
	level := slog.LevelInfo
	if status >= http.StatusInternalServerError {
		level = slog.LevelError
	}
	slog.Log(context.Background(), level, "API error response", "code", code, "status", status, "requestId", requestID)
}
