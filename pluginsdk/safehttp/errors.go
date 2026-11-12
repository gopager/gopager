package safehttp

import "errors"

// DeniedError reports a request the Policy refuses. No connection to the destination was
// made. Reason is a short, secret-free explanation such as "loopback address".
type DeniedError struct{ Reason string }

// Error implements error.
func (e *DeniedError) Error() string { return "safehttp: destination denied: " + e.Reason }

// RequestError reports a transport failure of a permitted request. Err is the cause.
type RequestError struct {
	Err error
	// Written is true when the request, including its body, was handed to the server
	// completely, so the server may have acted on it.
	Written bool
}

// Error implements error.
func (e *RequestError) Error() string { return "safehttp: request failed: " + e.Err.Error() }

// Unwrap returns the cause.
func (e *RequestError) Unwrap() error { return e.Err }

// IsDenied reports whether err is, or wraps, a DeniedError.
func IsDenied(err error) bool {
	var denied *DeniedError
	return errors.As(err, &denied)
}

// Written reports whether err is a RequestError whose request was fully written to the
// server. It is false for denials, DNS and connection failures, and for a nil error.
func Written(err error) bool {
	var requestErr *RequestError
	return errors.As(err, &requestErr) && requestErr.Written
}
