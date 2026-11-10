package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/readmodel"
)

type allowAll struct{}

func (allowAll) Authenticate(context.Context, identity.Proof) error { return nil }

// fakeWake hands out subscriptions whose signals the test fires itself.
type fakeWake struct {
	mu      sync.Mutex
	signals []chan struct{}
}

func (w *fakeWake) Subscribe() (<-chan struct{}, func()) {
	signal := make(chan struct{}, 1)
	signal <- struct{}{}
	w.mu.Lock()
	w.signals = append(w.signals, signal)
	w.mu.Unlock()
	return signal, func() {}
}

func (w *fakeWake) fire() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, signal := range w.signals {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
}

var _ events.WakeSubscriber = (*fakeWake)(nil)

// fakeStreams records how a stream was opened and replays scripted batches; after the script
// it answers empty batches.
type fakeStreams struct {
	mu       sync.Mutex
	filters  readmodel.Filters
	position string
	opened   int
	script   []func() (readmodel.LiveBatch, error)
	batches  int
}

func (f *fakeStreams) OpenStream(filters readmodel.Filters, position string) (MessageStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.filters, f.position = filters, position
	f.opened++
	return f, nil
}

func (f *fakeStreams) Batch(context.Context, identity.Proof) (readmodel.LiveBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches++
	if len(f.script) == 0 {
		return readmodel.LiveBatch{Position: "1-2-3"}, nil
	}
	next := f.script[0]
	f.script = f.script[1:]
	return next()
}

func batchOf(frames ...readmodel.LiveFrame) func() (readmodel.LiveBatch, error) {
	return func() (readmodel.LiveBatch, error) {
		return readmodel.LiveBatch{Frames: frames, Position: "1-2-3"}, nil
	}
}

func streamServer(t *testing.T, streams MessageStreams, wake events.WakeSubscriber, options ...StreamOption) *httptest.Server {
	t.Helper()
	transport, err := identity.NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterMessageStreamRoutes(mux, transport, allowAll{}, streams, wake, options...)
	server := httptest.NewUnstartedServer(mux)
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func get(t *testing.T, server *httptest.Server, path string, headers ...string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

// readEvent reads one SSE event as its lines.
func readEvent(t *testing.T, reader *bufio.Reader) []string {
	t.Helper()
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if len(lines) == 0 && err == io.EOF {
				return nil
			}
			t.Fatalf("reading event: %v (got %v)", err, lines)
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			return lines
		}
		lines = append(lines, line)
	}
}

const streamQuery = "/api/v1/messages/stream?from=2026-01-01T00:00:00Z"

func TestStreamRequiresFromAndAStartPosition(t *testing.T) {
	t.Parallel()
	streams := &fakeStreams{}
	server := streamServer(t, streams, &fakeWake{})
	for _, path := range []string{
		streamQuery,
		"/api/v1/messages/stream?position=1-2-3",
		streamQuery + "&position=1-2-3&position=1-2-4",
		streamQuery + "&position=1-2-3&bogus=1",
	} {
		response := get(t, server, path)
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), `"INVALID_INPUT"`) {
			t.Errorf("%s: status %d body %s, want 400 INVALID_INPUT", path, response.StatusCode, body)
		}
	}
	if streams.opened != 0 {
		t.Errorf("no stream may open for invalid requests, opened %d", streams.opened)
	}
}

func TestStreamStartPositionPrefersLastEventID(t *testing.T) {
	t.Parallel()
	streams := &fakeStreams{script: []func() (readmodel.LiveBatch, error){batchOf()}}
	server := streamServer(t, streams, &fakeWake{})
	response := get(t, server, streamQuery+"&position=9-9-9", "Last-Event-ID", "4-5-6")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	if streams.position != "4-5-6" || streams.filters.From == nil {
		t.Errorf("position %q filters %+v, want Last-Event-ID and the parsed range", streams.position, streams.filters)
	}
	header := response.Header
	if header.Get("Content-Type") != "text/event-stream; charset=utf-8" || header.Get("Cache-Control") != "no-store" ||
		header.Get("X-Accel-Buffering") != "no" {
		t.Errorf("headers %v", header)
	}
}

func TestStreamWritesFramesAndClosesAfterResync(t *testing.T) {
	t.Parallel()
	streams := &fakeStreams{script: []func() (readmodel.LiveBatch, error){
		batchOf(readmodel.LiveFrame{Event: "upsert", Data: json.RawMessage(`{"messageId":"a"}`)},
			readmodel.LiveFrame{Event: "remove", ID: "7-8-9", Data: json.RawMessage(`{"messageId":"b"}`)}),
		func() (readmodel.LiveBatch, error) {
			return readmodel.LiveBatch{Resync: true, Frames: []readmodel.LiveFrame{readmodel.ResyncFrame()}}, nil
		},
	}}
	wake := &fakeWake{}
	server := streamServer(t, streams, wake)
	response := get(t, server, streamQuery+"&position=1-2-3")
	reader := bufio.NewReader(response.Body)
	if got := readEvent(t, reader); strings.Join(got, "|") != `event: upsert|data: {"messageId":"a"}` {
		t.Errorf("first event %v", got)
	}
	if got := readEvent(t, reader); strings.Join(got, "|") != `event: remove|id: 7-8-9|data: {"messageId":"b"}` {
		t.Errorf("second event %v", got)
	}
	wake.fire()
	if got := readEvent(t, reader); strings.Join(got, "|") != `event: resync-required|data: {}` {
		t.Errorf("resync event %v", got)
	}
	if got := readEvent(t, reader); got != nil {
		t.Errorf("the stream must close after resync-required, got %v", got)
	}
}

func TestStreamFirstBatchFailureIsAnErrorResponse(t *testing.T) {
	t.Parallel()
	streams := &fakeStreams{script: []func() (readmodel.LiveBatch, error){
		func() (readmodel.LiveBatch, error) { return readmodel.LiveBatch{}, identity.ErrLoginRequired },
	}}
	server := streamServer(t, streams, &fakeWake{})
	response := get(t, server, streamQuery+"&position=1-2-3")
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), `"LOGIN_REQUIRED"`) {
		t.Fatalf("status %d body %s, want 401 LOGIN_REQUIRED before streaming", response.StatusCode, body)
	}
}

func TestStreamAuthorityLossEndsWithErrorFrameAtNextHeartbeat(t *testing.T) {
	t.Parallel()
	streams := &fakeStreams{script: []func() (readmodel.LiveBatch, error){
		batchOf(),
		func() (readmodel.LiveBatch, error) { return readmodel.LiveBatch{}, identity.ErrLoginRequired },
	}}
	server := streamServer(t, streams, &fakeWake{}, WithStreamTiming(50*time.Millisecond, 0, 0))
	response := get(t, server, streamQuery+"&position=1-2-3")
	reader := bufio.NewReader(response.Body)
	if got := readEvent(t, reader); strings.Join(got, "|") != `event: error|data: {"code":"LOGIN_REQUIRED"}` {
		t.Errorf("event %v", got)
	}
	if got := readEvent(t, reader); got != nil {
		t.Errorf("the stream must close after an error frame, got %v", got)
	}
}

func TestStreamHeartbeatCarriesPositionAndOutlivesServerTimeouts(t *testing.T) {
	t.Parallel()
	streams := &fakeStreams{}
	transport, err := identity.NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterMessageStreamRoutes(mux, transport, allowAll{}, streams, &fakeWake{}, WithStreamTiming(100*time.Millisecond, 0, 0))
	server := httptest.NewUnstartedServer(mux)
	// Both server timeouts are far shorter than the time the stream is observed for.
	server.Config.ReadTimeout = 300 * time.Millisecond
	server.Config.WriteTimeout = 300 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)
	response := get(t, server, streamQuery+"&position=1-2-3")
	reader := bufio.NewReader(response.Body)
	started := time.Now()
	heartbeats := 0
	for time.Since(started) < 1200*time.Millisecond {
		got := readEvent(t, reader)
		if strings.Join(got, "|") != "event: heartbeat|id: 1-2-3|data: {}" {
			t.Fatalf("after %v: event %v", time.Since(started), got)
		}
		heartbeats++
	}
	if heartbeats < 8 {
		t.Errorf("only %d heartbeats in 1.2 s at a 100 ms interval", heartbeats)
	}
}

func TestStreamClosesWhenTheClientDoesNotRead(t *testing.T) {
	t.Parallel()
	large := readmodel.LiveFrame{Event: "upsert", Data: json.RawMessage(`"` + strings.Repeat("x", 32<<20) + `"`)}
	streams := &fakeStreams{script: []func() (readmodel.LiveBatch, error){batchOf(large)}}
	var calls sync.WaitGroup
	calls.Add(1)
	transport, err := identity.NewHTTPTransport("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterMessageStreamRoutes(mux, transport, allowAll{}, streams, &fakeWake{}, WithStreamTiming(0, 0, 200*time.Millisecond))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer calls.Done()
		mux.ServeHTTP(w, r)
	}))
	server.Start()
	t.Cleanup(server.Close)
	response := get(t, server, streamQuery+"&position=1-2-3")
	_ = response
	finished := make(chan struct{})
	go func() { calls.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("the handler stayed blocked on a client that never reads")
	}
}

type refusingAdmission struct{ sources []string }

func (a *refusingAdmission) Admit(_ context.Context, _ identity.Proof, source string) error {
	a.sources = append(a.sources, source)
	return identity.ErrLoginRequired
}

func TestStreamIsAdmittedBeforeAnyBatchRuns(t *testing.T) {
	t.Parallel()
	streams := &fakeStreams{}
	admission := &refusingAdmission{}
	server := streamServer(t, streams, &fakeWake{}, WithStreamAdmission(admission))
	response := get(t, server, streamQuery+"&position=1-2-3")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want the admission error", response.StatusCode)
	}
	if len(admission.sources) != 1 || admission.sources[0] != "127.0.0.1" || streams.batches != 0 {
		t.Errorf("sources %v batches %d, want one admission by client host and no batch", admission.sources, streams.batches)
	}
}
