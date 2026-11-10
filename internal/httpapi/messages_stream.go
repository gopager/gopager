package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

// Timing defaults of a live stream.
const (
	// DefaultStreamHeartbeat is how often an idle stream re-checks the caller's authority and
	// sends a heartbeat.
	DefaultStreamHeartbeat = 15 * time.Second
	// DefaultStreamBatchTimeout bounds one batch's transaction.
	DefaultStreamBatchTimeout = 30 * time.Second
	// DefaultStreamWriteTimeout bounds one write to the client; a client that does not read is
	// disconnected.
	DefaultStreamWriteTimeout = 30 * time.Second
)

// MessageStreams opens live sessions. *readmodel.Live implements it through LiveSessions.
type MessageStreams interface {
	// OpenStream validates the loaded range and the start position of a stream.
	OpenStream(filters readmodel.Filters, position string) (MessageStream, error)
}

// MessageStream produces the batches of one stream. It is used by a single goroutine.
type MessageStream interface {
	// Batch evaluates the facts since the last batch for the caller.
	Batch(ctx context.Context, proof identity.Proof) (readmodel.LiveBatch, error)
}

// LiveSessions adapts *readmodel.Live to MessageStreams.
type LiveSessions struct{ Live *readmodel.Live }

// OpenStream opens a session on the read side.
func (l LiveSessions) OpenStream(filters readmodel.Filters, position string) (MessageStream, error) {
	return l.Live.Open(filters, position)
}

// StreamOption adjusts a stream route, mainly its timing in tests.
type StreamOption func(*streamConfig)

type streamConfig struct {
	heartbeat, batch, write time.Duration
	admission               SearchAdmission
}

// WithStreamTiming overrides the heartbeat interval, the batch timeout and the per-write
// timeout; a zero value keeps the default.
func WithStreamTiming(heartbeat, batch, write time.Duration) StreamOption {
	return func(c *streamConfig) {
		for target, value := range map[*time.Duration]time.Duration{&c.heartbeat: heartbeat, &c.batch: batch, &c.write: write} {
			if value > 0 {
				*target = value
			}
		}
	}
}

// WithStreamAdmission admits every stream open against the search limit like a feed page, after
// authentication and validation and before the response starts; a refusal is answered as a
// normal error response.
func WithStreamAdmission(admission SearchAdmission) StreamOption {
	return func(c *streamConfig) { c.admission = admission }
}

// RegisterMessageStreamRoutes adds the Message live stream. The guest may use it under its
// grants like any other caller. wake delivers the signals that new facts may be available.
func RegisterMessageStreamRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator,
	streams MessageStreams, wake events.WakeSubscriber, options ...StreamOption) {
	config := streamConfig{heartbeat: DefaultStreamHeartbeat, batch: DefaultStreamBatchTimeout, write: DefaultStreamWriteTimeout}
	for _, option := range options {
		option(&config)
	}
	api := messageStreamAPI{routeGuard: routeGuard{transport: transport, authenticator: authenticator},
		streams: streams, wake: wake, streamConfig: config}
	mux.HandleFunc("GET /api/v1/messages/stream", api.serve)
}

type messageStreamAPI struct {
	routeGuard
	streams MessageStreams
	wake    events.WakeSubscriber
	streamConfig
}

// startPosition returns the position a stream resumes from: Last-Event-ID wins over the
// position parameter, and one of them is required.
func startPosition(r *http.Request, values url.Values) (string, error) {
	if len(values["position"]) > 1 {
		return "", authz.ErrInvalidInput
	}
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		return last, nil
	}
	if position := values.Get("position"); position != "" {
		return position, nil
	}
	return "", authz.ErrInvalidInput
}

// serve authenticates the caller, runs the first batch before any response byte so that a
// refused caller gets a plain error response, and then streams until the client leaves, the
// caller's authority ends or the view needs a resync.
func (a messageStreamAPI) serve(w http.ResponseWriter, r *http.Request) {
	proof, err := a.transport.SelectProof(r)
	if err == nil {
		err = a.authenticator.Authenticate(r.Context(), proof)
	}
	if err != nil {
		writeManagementError(w, err)
		return
	}
	stream, err := a.open(r)
	if err == nil && a.admission != nil {
		err = a.admission.Admit(r.Context(), proof, identity.ClientSource(r))
	}
	if err != nil {
		writeManagementError(w, err)
		return
	}
	signals, release := a.wake.Subscribe()
	defer release()
	// The subscription holds a signal for the first check; the first batch below is that check.
	select {
	case <-signals:
	default:
	}
	batch, err := a.runBatch(r.Context(), stream, proof)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	out := newFrameWriter(w, a.write)
	if !out.begin() {
		return
	}
	a.stream(r.Context(), out, stream, proof, batch, signals)
}

// open parses the filters and start position of the request into a session.
func (a messageStreamAPI) open(r *http.Request) (MessageStream, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, authz.ErrInvalidInput
	}
	filters, err := readmodel.ParseFilters(values, "position")
	if err != nil {
		return nil, err
	}
	if filters.From == nil {
		return nil, authz.ErrInvalidInput
	}
	position, err := startPosition(r, values)
	if err != nil {
		return nil, err
	}
	return a.streams.OpenStream(filters, position)
}

// runBatch runs one batch within the batch timeout.
func (a messageStreamAPI) runBatch(ctx context.Context, stream MessageStream, proof identity.Proof) (readmodel.LiveBatch, error) {
	ctx, cancel := context.WithTimeout(ctx, a.batch)
	defer cancel()
	return stream.Batch(ctx, proof)
}

// stream writes the first batch and then one batch per wake-up or heartbeat tick. A batch that
// filled its read limit is followed immediately by the next one.
func (a messageStreamAPI) stream(ctx context.Context, out *frameWriter, stream MessageStream, proof identity.Proof,
	batch readmodel.LiveBatch, signals <-chan struct{}) {
	ticker := time.NewTicker(a.heartbeat)
	defer ticker.Stop()
	for {
		heartbeat := false
		if batch.Resync {
			out.write(batch.Frames...)
			return
		}
		if !out.write(batch.Frames...) {
			return
		}
		if !batch.More {
			select {
			case <-ctx.Done():
				return
			case <-signals:
			case <-ticker.C:
				heartbeat = true
			}
		}
		var err error
		if batch, err = a.runBatch(ctx, stream, proof); err != nil {
			if ctx.Err() == nil {
				out.write(readmodel.ErrorFrame(streamErrorCode(err)))
			}
			return
		}
		if heartbeat && len(batch.Frames) == 0 && !batch.Resync {
			batch.Frames = []readmodel.LiveFrame{readmodel.HeartbeatFrame(batch.Position)}
		}
	}
}

// streamErrorCode maps a failure of a running stream to the public code of its error frame.
func streamErrorCode(err error) string {
	var failure *platform.Failure
	switch {
	case errors.Is(err, identity.ErrLoginRequired):
		return "LOGIN_REQUIRED"
	case errors.Is(err, authz.ErrForbidden):
		return "FORBIDDEN"
	case errors.As(err, &failure):
		return string(failure.Code())
	}
	return "INTERNAL_ERROR"
}

// frameWriter writes server-sent events with a deadline per write. The server-wide write
// timeout covers a whole response and would end a long-lived stream, so every write sets its
// own deadline and thereby replaces the server's.
type frameWriter struct {
	w       http.ResponseWriter
	control *http.ResponseController
	timeout time.Duration
}

func newFrameWriter(w http.ResponseWriter, timeout time.Duration) *frameWriter {
	return &frameWriter{w: w, control: http.NewResponseController(w), timeout: timeout}
}

// begin sends the response head.
func (f *frameWriter) begin() bool {
	header := f.w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Accel-Buffering", "no")
	f.w.WriteHeader(http.StatusOK)
	return f.flush()
}

// write sends the frames and flushes once; it reports false when the client is gone or too slow.
func (f *frameWriter) write(frames ...readmodel.LiveFrame) bool {
	if len(frames) == 0 {
		return true
	}
	if err := f.control.SetWriteDeadline(time.Now().Add(f.timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return false
	}
	for _, frame := range frames {
		if _, err := fmt.Fprint(f.w, "event: ", frame.Event, "\n"); err != nil {
			return false
		}
		if frame.ID != "" {
			if _, err := fmt.Fprint(f.w, "id: ", frame.ID, "\n"); err != nil {
				return false
			}
		}
		if _, err := fmt.Fprintf(f.w, "data: %s\n\n", frame.Data); err != nil {
			return false
		}
	}
	return f.flush()
}

func (f *frameWriter) flush() bool {
	if err := f.control.SetWriteDeadline(time.Now().Add(f.timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return false
	}
	return f.control.Flush() == nil
}
