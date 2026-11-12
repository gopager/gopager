package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

// recordedRequest is one request a local receiver got.
type recordedRequest struct {
	header http.Header
	body   []byte
	// form is the decoded form body, empty for other content types.
	form url.Values
}

// recordingReceiver is a local provider stand-in. It records every request before it answers with
// the configured status; a 200 answer carries the body the Pushover API gives on success.
type recordingReceiver struct {
	*httptest.Server
	status atomic.Int32
	mu     sync.Mutex
	seen   []recordedRequest
}

// newRecordingReceiver starts a receiver answering 200.
func newRecordingReceiver(t *testing.T) *recordingReceiver {
	t.Helper()
	receiver := &recordingReceiver{}
	receiver.status.Store(http.StatusOK)
	receiver.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request := recordedRequest{header: r.Header.Clone(), body: body}
		if form, err := url.ParseQuery(string(body)); err == nil && r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
			request.form = form
		}
		receiver.mu.Lock()
		receiver.seen = append(receiver.seen, request)
		receiver.mu.Unlock()
		status := int(receiver.status.Load())
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"status":1,"request":"req-1"}`))
		}
	}))
	t.Cleanup(receiver.Close)
	return receiver
}

// requests returns a copy of the requests recorded so far.
func (r *recordingReceiver) requests() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.seen...)
}

// webhookSignature recomputes the signature the webhook plugin documents: HMAC-SHA256 over the
// timestamp, a dot and the body, hex encoded behind "sha256=".
func webhookSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
