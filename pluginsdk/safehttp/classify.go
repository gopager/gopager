package safehttp

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gopager/gopager/pluginsdk"
)

const (
	// DefaultRetryAfterSeconds is the delay reported for a rate limit without a usable Retry-After.
	DefaultRetryAfterSeconds = 60
	// MaxRetryAfterSeconds caps a reported Retry-After.
	MaxRetryAfterSeconds = 3600
)

// ErrorResult classifies a Do error as a Delivery result: a denied destination is permanent
// "destination-denied", a failure before the request was fully written is transient
// "connection-failed", and any failure after it is uncertain "response-lost".
func ErrorResult(err error) pluginsdk.DeliveryResult {
	switch {
	case IsDenied(err):
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusPermanent, Code: "destination-denied"}
	case Written(err):
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusUncertain, Code: "response-lost"}
	default:
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusTransient, Code: "connection-failed"}
	}
}

// RateLimitedResult returns the transient "rate-limited" result for an HTTP 429 answer with
// the given headers. The delay is the Retry-After value in seconds, DefaultRetryAfterSeconds
// when it is absent or not a positive integer, and at most MaxRetryAfterSeconds.
func RateLimitedResult(header http.Header) pluginsdk.DeliveryResult {
	seconds, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After")))
	if err != nil || seconds < 1 {
		seconds = DefaultRetryAfterSeconds
	}
	return pluginsdk.DeliveryResult{
		Status:            pluginsdk.StatusTransient,
		Code:              "rate-limited",
		RetryAfterSeconds: min(seconds, MaxRetryAfterSeconds),
	}
}

// SecureURL parses raw and requires an absolute https URL with a host. An http URL is
// accepted only when allowPrivate is set, so credentials never cross the public network
// unencrypted. It returns a *DeniedError for a refused scheme and a plain error for an
// unparsable URL.
func SecureURL(raw string, allowPrivate bool) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("safehttp: invalid URL")
	}
	switch {
	case parsed.Scheme == "https", parsed.Scheme == "http" && allowPrivate:
		return parsed, nil
	default:
		return nil, &DeniedError{Reason: "URL scheme not allowed"}
	}
}
