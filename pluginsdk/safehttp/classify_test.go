package safehttp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gopager/gopager/pluginsdk"
)

func TestErrorResult(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err          error
		status, code string
	}{
		{&DeniedError{Reason: "x"}, pluginsdk.StatusPermanent, "destination-denied"},
		{&RequestError{Err: errors.New("x"), Written: true}, pluginsdk.StatusUncertain, "response-lost"},
		{&RequestError{Err: errors.New("x")}, pluginsdk.StatusTransient, "connection-failed"},
		{errors.New("x"), pluginsdk.StatusTransient, "connection-failed"},
	}
	for _, c := range cases {
		if got := ErrorResult(c.err); got.Status != c.status || got.Code != c.code {
			t.Errorf("%v: %+v", c.err, got)
		}
	}
}

func TestRateLimitedResult(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]int{"": 60, "30": 30, " 5 ": 5, "0": 60, "-3": 60, "soon": 60, "Wed, 21 Oct 2026 07:28:00 GMT": 60, "99999": 3600, "3600": 3600} {
		header := http.Header{}
		if value != "" {
			header.Set("Retry-After", value)
		}
		got := RateLimitedResult(header)
		if got.Status != pluginsdk.StatusTransient || got.Code != "rate-limited" || got.RetryAfterSeconds != want {
			t.Errorf("Retry-After %q: %+v, want %d", value, got, want)
		}
	}
}

func TestSecureURL(t *testing.T) {
	t.Parallel()
	for raw, allowPrivate := range map[string]bool{"https://a.example/x": false, "http://127.0.0.1:8080/": true} {
		if _, err := SecureURL(raw, allowPrivate); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
	if _, err := SecureURL("http://a.example/", false); !IsDenied(err) {
		t.Errorf("http without allowPrivate: %v", err)
	}
	for _, raw := range []string{"ftp://a.example/", "https:///x", "not a url", "://x", ""} {
		if _, err := SecureURL(raw, true); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
}
