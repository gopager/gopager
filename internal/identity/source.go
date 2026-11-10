package identity

import (
	"net/http"
	"time"
)

// ClientSource returns the client source of a request: the host part of the connection's
// remote address. Login throttling and search admission bucket anonymous clients by it.
// Forwarding headers are deliberately ignored, as any client can set them.
func ClientSource(r *http.Request) string { return loginSource(r) }

// Seconds returns the retry delay in whole seconds, rounded up and at least 1, as sent in the
// Retry-After header and in retryAfterSeconds.
func (e *RateLimited) Seconds() int64 {
	seconds := int64(e.RetryAfter / time.Second)
	if e.RetryAfter%time.Second != 0 {
		seconds++
	}
	return max(seconds, 1)
}
