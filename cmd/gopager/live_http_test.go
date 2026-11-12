package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// liveFixture is a composed API with a running wake hub and short stream timings.
type liveFixture struct {
	*queryFixture
	core     *core
	revision atomic.Int64
}

// newLiveFixture composes the API with the live stream enabled. searchAttempts of zero keeps the
// default admission limit.
func newLiveFixture(t *testing.T, searchAttempts int) *liveFixture {
	t.Helper()
	store := newManagementStore(t)
	shared := mustComposeCore(t, store)
	shared.searchAttempts = searchAttempts
	hub, err := events.NewWakeHub(store, events.NotifyChannel)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = hub.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	shared.live = &liveConfig{wake: hub, replayWindow: readmodel.DefaultLiveReplayWindow}
	shared.live.timing.heartbeat = 150 * time.Millisecond
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	f := &queryFixture{t: t, store: store, origin: origin, client: backend.Client(), gateway: uuid.New(),
		network: uuid.New(), agency: seedAgency(t, store)}
	f.exec(`INSERT INTO gateways (id, name) VALUES ($1, 'gw')`, f.gateway)
	f.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, 'Net', 'pocsag', $2)`, f.network, f.agency)
	return &liveFixture{queryFixture: f, core: shared}
}

// appendFact runs the append in its own transaction, as a producer would commit it.
func (f *liveFixture) appendFact(append func(ctx context.Context, tx platform.TransactionContext) error) {
	f.t.Helper()
	if err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		return append(tx.Context(), tx)
	}); err != nil {
		f.t.Fatal(err)
	}
}

func microsecondNow() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func (f *liveFixture) intake(fact events.IntakeFact) {
	f.t.Helper()
	fact.ID, fact.Revision, fact.OccurredAt = uuid.New(), f.revision.Add(1), microsecondNow()
	f.appendFact(func(ctx context.Context, tx platform.TransactionContext) error {
		_, err := f.core.facts.AppendIntakeFact(ctx, tx, fact)
		return err
	})
}

func (f *liveFixture) correction(fact events.CorrectionFact) {
	f.t.Helper()
	fact.ID, fact.Revision, fact.OccurredAt = uuid.New(), f.revision.Add(1), microsecondNow()
	f.appendFact(func(ctx context.Context, tx platform.TransactionContext) error {
		_, err := f.core.facts.AppendCorrectionFact(ctx, tx, fact)
		return err
	})
}

func (f *liveFixture) created(message uuid.UUID, at time.Time) {
	f.t.Helper()
	f.intake(events.IntakeFact{Type: events.MessageCreated, MessageID: message, TransmissionID: uuid.New(), MessageTime: at})
}

// newMessage inserts a Message assigned to the Units and announces it with message.created.
func (f *liveFixture) newMessage(at time.Time, text string, units ...uuid.UUID) uuid.UUID {
	f.t.Helper()
	message := f.message(at, text, units...)
	f.created(message, at)
	return message
}

// reassign moves the Message's appearance from one Unit to another and records the correction.
func (f *liveFixture) reassign(message, from, to uuid.UUID, at time.Time) {
	f.t.Helper()
	f.exec(`DELETE FROM message_units WHERE message_id = $1 AND unit_id = $2`, message, from)
	f.exec(`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor) VALUES ($1, $2, $3, $4, $4)`,
		message, to, f.agency, at)
	rule := uuid.New()
	f.correction(events.CorrectionFact{Type: "assignment.changed", Aggregate: events.EntityRef{Type: "message", ID: message},
		MessageID: message, TransmissionID: uuid.New(), NetworkID: f.network, Reason: "corrected", MessageTime: at,
		Targets: []events.CorrectionTarget{{Position: 0,
			Before: events.IntakeResolution{Status: "assigned", UnitID: &from, AddressRuleID: &rule},
			After:  events.IntakeResolution{Status: "assigned", UnitID: &to, AddressRuleID: &rule}}}})
}

// sseEvent is one received server-sent event.
type sseEvent struct{ Event, ID, Data string }

// sseClient reads a live stream in the background.
type sseClient struct {
	events    chan sseEvent
	heartbeat chan sseEvent
}

// openStream opens the stream and requires the 200 event-stream response.
func (f *liveFixture) openStream(bearer, query string, headers ...string) *sseClient {
	f.t.Helper()
	client, status, body := f.tryStream(bearer, query, headers...)
	if client == nil {
		f.t.Fatalf("stream%s = %d %s, want 200", query, status, body)
	}
	return client
}

// tryStream returns the client for a 200 response, otherwise the status and body.
func (f *liveFixture) tryStream(bearer, query string, headers ...string) (*sseClient, int, string) {
	f.t.Helper()
	request, err := http.NewRequestWithContext(f.t.Context(), http.MethodGet, f.origin+"/api/v1/messages/stream"+query, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", bearer)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	response, err := f.client.Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return nil, response.StatusCode, string(body)
	}
	if response.Header.Get("Content-Type") != "text/event-stream; charset=utf-8" || response.Header.Get("Cache-Control") != "no-store" {
		f.t.Fatalf("headers %v", response.Header)
	}
	client := &sseClient{events: make(chan sseEvent, 512), heartbeat: make(chan sseEvent, 512)}
	f.t.Cleanup(func() { _ = response.Body.Close() })
	go func() {
		defer close(client.events)
		reader := bufio.NewReader(response.Body)
		var current sseEvent
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSuffix(line, "\n")
			switch {
			case line == "":
				if current.Event == "heartbeat" {
					select {
					case client.heartbeat <- current:
					default:
					}
				} else {
					client.events <- current
				}
				current = sseEvent{}
			case strings.HasPrefix(line, "event: "):
				current.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				current.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				current.Data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return client, response.StatusCode, ""
}

// next returns the next non-heartbeat event.
func (c *sseClient) next(t *testing.T) sseEvent {
	t.Helper()
	select {
	case event, open := <-c.events:
		if !open {
			t.Fatal("stream closed while waiting for an event")
		}
		return event
	case <-time.After(10 * time.Second):
		t.Fatal("no event within 10 s")
		return sseEvent{}
	}
}

// beat returns the next heartbeat.
func (c *sseClient) beat(t *testing.T) sseEvent {
	t.Helper()
	select {
	case event := <-c.heartbeat:
		return event
	case <-time.After(10 * time.Second):
		t.Fatal("no heartbeat within 10 s")
		return sseEvent{}
	}
}

// quiet requires that no non-heartbeat event arrives for the duration, which spans several
// heartbeat batches.
func (c *sseClient) quiet(t *testing.T) {
	t.Helper()
	select {
	case event, open := <-c.events:
		if open {
			t.Fatalf("unexpected event %+v", event)
		}
		t.Fatal("stream closed unexpectedly")
	case <-time.After(600 * time.Millisecond):
	}
}

// closed requires that the stream ends without further events.
func (c *sseClient) closed(t *testing.T) {
	t.Helper()
	select {
	case event, open := <-c.events:
		if open {
			t.Fatalf("unexpected event %+v, want the stream to end", event)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the stream did not close")
	}
}

// expect requires the next event to be of the kind for the Message and returns it.
func (c *sseClient) expect(t *testing.T, kind string, message uuid.UUID) sseEvent {
	t.Helper()
	event := c.next(t)
	var data struct {
		MessageID string `json:"messageId"`
	}
	if event.Event != kind || json.Unmarshal([]byte(event.Data), &data) != nil || data.MessageID != message.String() {
		t.Fatalf("event %+v, want %s of %s", event, kind, message)
	}
	return event
}

func rfc(at time.Time) string { return url.QueryEscape(at.Format(time.RFC3339Nano)) }

func TestLiveStreamCutoffHasNoGap(t *testing.T) {
	t.Parallel()
	f := newLiveFixture(t, 0)
	unit := f.unit("A", nil)
	base := microsecondNow().Add(-time.Hour)
	bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	first := f.newMessage(base, "one", unit)
	query := "?from=" + rfc(base.Add(-time.Minute))
	page := f.feed(bearer, query, http.StatusOK)
	if got := itemIDs(page.Data.Items); len(got) != 1 || got[0] != first.String() {
		t.Fatalf("page %v", got)
	}
	between := f.newMessage(base.Add(time.Minute), "between", unit)
	stream := f.openStream(bearer, query+"&position="+page.Data.Position)
	// The cutoff lies below every transaction still in flight when the page was read, including
	// those of parallel tests, so the stream may replay the page's own Message first; such a
	// duplicate upsert is harmless, a missing one would be a gap.
	event := stream.next(t)
	for event.Event == "upsert" && strings.Contains(event.Data, first.String()) {
		event = stream.next(t)
	}
	if event.Event != "upsert" || !strings.Contains(event.Data, between.String()) {
		t.Fatalf("event %+v, want upsert of %s", event, between)
	}
	if event.ID == "" {
		t.Error("the last frame of a fact carries the position")
	}
	stream.quiet(t)
	later := f.newMessage(base.Add(2*time.Minute), "later", unit)
	stream.expect(t, "upsert", later)
	if beat := stream.beat(t); beat.ID == "" || beat.Data != "{}" {
		t.Errorf("heartbeat %+v", beat)
	}
}

func TestLiveStreamLoadedRange(t *testing.T) {
	t.Parallel()
	f := newLiveFixture(t, 0)
	unit := f.unit("A", nil)
	base := microsecondNow().Add(-time.Hour)
	bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	from, until := base, base.Add(30*time.Minute)
	query := "?from=" + rfc(from) + "&until=" + rfc(until)
	page := f.feed(bearer, query, http.StatusOK)
	stream := f.openStream(bearer, query+"&position="+page.Data.Position)

	early := f.newMessage(base.Add(-time.Hour), "before the range", unit)
	f.newMessage(until.Add(time.Minute), "after the range", unit)
	inside := f.newMessage(base.Add(time.Minute), "inside", unit)
	stream.expect(t, "upsert", inside)
	stream.quiet(t)

	// A late Transmission moves a Message that was inside the range to a time before it: the
	// previous time keeps the fact relevant, and the client drops the repositioned item.
	moved := f.newMessage(base.Add(10*time.Minute), "moves", unit)
	stream.expect(t, "upsert", moved)
	earlier := base.Add(-2 * time.Hour)
	f.exec(`UPDATE messages SET first_observed_at = $2 WHERE id = $1`, moved, earlier)
	previous := base.Add(10 * time.Minute)
	f.intake(events.IntakeFact{Type: events.MessageTransmissionJoined, MessageID: moved, TransmissionID: uuid.New(),
		MessageTime: earlier, PreviousMessageTime: &previous, LateJoin: true})
	event := stream.expect(t, "upsert", moved)
	if !strings.Contains(event.Data, earlier.Format("2006-01-02T15:04:05")) {
		t.Errorf("the item carries its new time: %s", event.Data)
	}

	// A reopened stream with an extended range sees the facts the first range ignored.
	wide := "?from=" + rfc(base.Add(-3*time.Hour)) + "&until=" + rfc(until)
	reopened := f.openStream(bearer, wide+"&position="+page.Data.Position)
	seen := map[string]bool{}
	for range 4 {
		var data struct {
			MessageID string `json:"messageId"`
		}
		event := reopened.next(t)
		_ = json.Unmarshal([]byte(event.Data), &data)
		seen[event.Event+" "+data.MessageID] = true
	}
	if !seen["upsert "+early.String()] || !seen["upsert "+inside.String()] || !seen["upsert "+moved.String()] {
		t.Errorf("the extended range replays %v", seen)
	}
}

//nolint:tparallel,paralleltest // the subtests run in order against one shared stream
func TestLiveStreamRemoveBasis(t *testing.T) {
	f := newLiveFixture(t, 0)
	a, b, c := f.unit("A", nil), f.unit("B", nil), f.unit("C", nil)
	base := microsecondNow().Add(-time.Hour)
	reader, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "unit", References: []string{a.String()}}, []string{"payload", "unit"})
	query := "?from=" + rfc(base.Add(-time.Hour))
	page := f.feed(reader, query, http.StatusOK)
	stream := f.openStream(reader, query+"&position="+page.Data.Position)

	t.Run("correction from a readable Unit to a hidden one removes", func(t *testing.T) {
		message := f.newMessage(base, "a to b", a)
		stream.expect(t, "upsert", message)
		f.reassign(message, a, b, base)
		stream.expect(t, "remove", message)
	})
	t.Run("correction between hidden Units yields no frame", func(t *testing.T) {
		hidden := f.newMessage(base.Add(time.Minute), "b to c", b)
		f.reassign(hidden, b, c, base.Add(time.Minute))
		sentinel := f.newMessage(base.Add(2*time.Minute), "visible", a)
		stream.expect(t, "upsert", sentinel)
		stream.quiet(t)
	})
	t.Run("a Network move of an unassigned Message removes it from the former Agency", func(t *testing.T) {
		other := seedAgency(t, f.store)
		moved := uuid.New()
		f.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, 'Other', 'pocsag', $2)`, moved, other)
		agencyReader, _ := f.caller([]string{"message.read", "without-unit.read"},
			authz.Scope{Kind: "agency", References: []string{f.agency.String()}}, []string{"payload", "unit"})
		agencyPage := f.feed(agencyReader, query, http.StatusOK)
		agencyStream := f.openStream(agencyReader, query+"&position="+agencyPage.Data.Position)
		message := uuid.New()
		f.exec(`INSERT INTO messages (id, identity_form, identity_value, first_observed_at, last_observed_at) VALUES ($1, 'text', 'x', $2, $2)`,
			message, base)
		f.exec(`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor) VALUES ($1, NULL, $2, $3, $3)`,
			message, f.agency, base)
		f.created(message, base)
		agencyStream.expect(t, "upsert", message)
		f.exec(`UPDATE message_units SET agency_id = $2 WHERE message_id = $1`, message, other)
		f.correction(events.CorrectionFact{Type: "assignment.changed", Aggregate: events.EntityRef{Type: "message", ID: message},
			MessageID: message, TransmissionID: uuid.New(), NetworkID: moved, PreviousNetworkID: &f.network, Reason: "corrected",
			MessageTime: base})
		agencyStream.expect(t, "remove", message)
	})
	t.Run("processing completion upserts and a Gateway state change yields nothing", func(t *testing.T) {
		message := f.newMessage(base.Add(5*time.Minute), "processed", a)
		stream.expect(t, "upsert", message)
		contact := microsecondNow()
		f.intake(events.IntakeFact{Type: events.GatewayStateChanged, GatewayID: f.gateway, OldState: "active", NewState: "degraded",
			ContactTime: &contact})
		stream.quiet(t)
		now := microsecondNow()
		f.appendFact(func(ctx context.Context, tx platform.TransactionContext) error {
			_, err := f.core.facts.AppendProcessingCompletion(ctx, tx, events.ProcessingCompletionFact{
				ID: uuid.New(), WorkflowID: uuid.New(), ResultID: uuid.New(), ResultRevision: 1, TransmissionID: uuid.New(),
				MessageID: message, MessageTime: base.Add(5 * time.Minute), DefinitionVersionID: uuid.New(), SourceRevision: 1,
				ContextRevision: 1, OccurredAt: now, Outputs: fields.New().Declarations(fields.ScopeMessage, false),
				Result: processing.Evaluation{Selected: true, Fields: map[string]map[string]processing.FieldValue{}}})
			return err
		})
		stream.expect(t, "upsert", message)
	})
	t.Run("a retraction re-evaluates the Message", func(t *testing.T) {
		message := f.newMessage(base.Add(6*time.Minute), "retracted", a)
		stream.expect(t, "upsert", message)
		now := microsecondNow()
		f.appendFact(func(ctx context.Context, tx platform.TransactionContext) error {
			_, err := f.core.facts.AppendProcessingRetraction(ctx, tx, events.ProcessingRetractionFact{
				ID: uuid.New(), TransmissionID: uuid.New(), MessageID: message, MessageTime: base.Add(6 * time.Minute),
				WorkflowID: uuid.New(), ResultIDs: []uuid.UUID{uuid.New()}, OccurredAt: now})
			return err
		})
		stream.expect(t, "upsert", message)
	})
	t.Run("an unknown schema version requires a resync", func(t *testing.T) {
		envelope, _ := json.Marshal(map[string]any{"schemaVersion": 2, "domainEventId": uuid.New(), "type": "message.created",
			"data": map[string]any{"messageId": uuid.New(), "messageTime": base.Format(time.RFC3339Nano)}})
		id := uuid.New()
		var wrapped map[string]any
		_ = json.Unmarshal(envelope, &wrapped)
		wrapped["domainEventId"] = id
		envelope, _ = json.Marshal(wrapped)
		f.exec(`INSERT INTO domain_events (id, envelope) VALUES ($1, $2::jsonb)`, id, string(envelope))
		if event := stream.next(t); event.Event != "resync-required" || event.Data != "{}" {
			t.Fatalf("event %+v", event)
		}
		stream.closed(t)
	})
}

//nolint:tparallel,paralleltest // the subtests run in order against one shared stream
func TestLiveStreamBoundsAndResync(t *testing.T) {
	f := newLiveFixture(t, 0)
	unit := f.unit("A", nil)
	base := microsecondNow().Add(-time.Hour)
	query := "?from=" + rfc(base.Add(-time.Hour))
	bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	page := f.feed(bearer, query, http.StatusOK)
	pagePosition, err := readmodel.ParseSignedPosition(f.tokenKey(), page.Data.Position)
	if err != nil {
		t.Fatal(err)
	}
	positionPart := func(issued time.Time) string {
		moved := pagePosition
		moved.Issued = issued
		return url.QueryEscape(moved.Sign(f.tokenKey()))
	}
	fresh := func() string { return f.feed(bearer, query, http.StatusOK).Data.Position }
	requireResync := func(t *testing.T, stream *sseClient) {
		t.Helper()
		if event := stream.next(t); event.Event != "resync-required" || event.Data != "{}" {
			t.Fatalf("event %+v", event)
		}
		stream.closed(t)
	}

	t.Run("more than 1 MiB in one batch", func(t *testing.T) {
		large := f.newMessage(base, strings.Repeat("x", 8*1024), unit)
		stream := f.openStream(bearer, query+"&position="+url.QueryEscape(page.Data.Position))
		stream.expect(t, "upsert", large)
		// 200 updates of an 8 KiB item exceed the 1 MiB batch bound before the 256-fact read
		// limit; each fact yields at most one frame, so the byte bound is the reachable one.
		facts := make([]events.IntakeFact, 200)
		for i := range facts {
			facts[i] = events.IntakeFact{ID: uuid.New(), Type: events.MessageTransmissionJoined, MessageID: large,
				TransmissionID: uuid.New(), MessageTime: base, Revision: f.revision.Add(1), OccurredAt: microsecondNow()}
		}
		f.appendIntake(facts...)
		requireResync(t, stream)
	})
	t.Run("a position older than the replay window", func(t *testing.T) {
		requireResync(t, f.openStream(bearer, query+"&position="+positionPart(time.Now().Add(-25*time.Hour))))
	})
	t.Run("malformed, forged and future positions", func(t *testing.T) {
		requireResync(t, f.openStream(bearer, query+"&position=garbage"))
		future := readmodel.Position{TxID: 99999999999999, Issued: time.Now()}
		requireResync(t, f.openStream(bearer, query+"&position="+url.QueryEscape(future.Sign(f.tokenKey()))))
		renewed := strings.Split(page.Data.Position, ".")
		parts := strings.Split(renewed[0], "-")
		forged := parts[0] + "-" + parts[1] + "-" + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + "." + renewed[1]
		requireResync(t, f.openStream(bearer, query+"&position="+url.QueryEscape(forged)))
	})
	t.Run("a grant change", func(t *testing.T) {
		stream := f.openStream(bearer, query+"&position="+fresh())
		stream.beat(t)
		f.exec(`UPDATE identity_control SET access_revision = access_revision + 1`)
		requireResync(t, stream)
	})
	t.Run("a configuration change of a Unit", func(t *testing.T) {
		stream := f.openStream(bearer, query+"&position="+fresh())
		stream.beat(t)
		f.appendFact(func(ctx context.Context, tx platform.TransactionContext) error {
			_, err := f.core.facts.AppendMasterDataFact(ctx, tx, events.MasterDataFact{ID: uuid.New(),
				Aggregate: events.EntityRef{Type: "unit", ID: unit}, Revision: 99, OccurredAt: microsecondNow(), ChangeKind: "updated"})
			return err
		})
		requireResync(t, stream)
	})
	t.Run("a configuration change of an Attribute schema", func(t *testing.T) {
		stream := f.openStream(bearer, query+"&position="+fresh())
		stream.beat(t)
		f.appendFact(func(ctx context.Context, tx platform.TransactionContext) error {
			_, err := f.core.facts.AppendMasterDataFact(ctx, tx, events.MasterDataFact{ID: uuid.New(),
				Aggregate: events.EntityRef{Type: "attributeSchema", ID: uuid.New()}, Revision: 2, OccurredAt: microsecondNow(), ChangeKind: "updated"})
			return err
		})
		requireResync(t, stream)
	})
	t.Run("a revoked credential ends the stream with an error and no output", func(t *testing.T) {
		revoked, principal := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
		stream := f.openStream(revoked, query+"&position="+fresh())
		stream.beat(t)
		f.exec(`UPDATE identity_credentials SET revoked_at = now() WHERE account_id = $1`, principal)
		f.newMessage(base.Add(time.Minute), "after revocation", unit)
		if event := stream.next(t); event.Event != "error" || event.Data != `{"code":"LOGIN_REQUIRED"}` {
			t.Fatalf("event %+v, want the error frame and no upsert", event)
		}
		stream.closed(t)
	})
}

func TestLiveStreamResumesWithLastEventID(t *testing.T) {
	t.Parallel()
	f := newLiveFixture(t, 0)
	unit := f.unit("A", nil)
	base := microsecondNow().Add(-time.Hour)
	bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	query := "?from=" + rfc(base.Add(-time.Hour))
	page := f.feed(bearer, query, http.StatusOK)
	first := f.openStream(bearer, query+"&position="+page.Data.Position)
	one := f.newMessage(base, "one", unit)
	two := f.newMessage(base.Add(time.Minute), "two", unit)
	first.expect(t, "upsert", one)
	last := first.expect(t, "upsert", two)
	three := f.newMessage(base.Add(2*time.Minute), "three", unit)
	resumed := f.openStream(bearer, query, "Last-Event-ID", last.ID)
	resumed.expect(t, "upsert", three)
	resumed.quiet(t)
}

func TestLiveStreamsAreAuthorizedPerCaller(t *testing.T) {
	t.Parallel()
	f := newLiveFixture(t, 0)
	a, b := f.unit("A", nil), f.unit("B", nil)
	base := microsecondNow().Add(-time.Hour)
	query := "?from=" + rfc(base.Add(-time.Hour))
	read := []string{"message.read"}
	fields := []string{"payload", "unit"}
	forA, _ := f.caller(read, authz.Scope{Kind: "unit", References: []string{a.String()}}, fields)
	forB, _ := f.caller(read, authz.Scope{Kind: "unit", References: []string{b.String()}}, fields)
	pageA := f.feed(forA, query, http.StatusOK)
	pageB := f.feed(forB, query, http.StatusOK)
	tabOne := f.openStream(forA, query+"&position="+pageA.Data.Position)
	tabTwo := f.openStream(forA, query+"&position="+pageA.Data.Position+"&unit="+a.String())
	other := f.openStream(forB, query+"&position="+pageB.Data.Position)
	inA := f.newMessage(base, "in a", a)
	inB := f.newMessage(base.Add(time.Minute), "in b", b)
	tabOne.expect(t, "upsert", inA)
	tabTwo.expect(t, "upsert", inA)
	other.expect(t, "upsert", inB)
	for _, stream := range []*sseClient{tabOne, tabTwo, other} {
		stream.quiet(t)
	}
}

func TestLiveStreamRequestRules(t *testing.T) {
	t.Parallel()
	withinOneAdmissionWindow(t, newAdmissionClockStore(t), func(int) error {
		f := newLiveFixture(t, 2)
		bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
		from := "from=" + rfc(microsecondNow().Add(-time.Hour))
		var failures []error
		for name, query := range map[string]string{"no position": "?" + from, "no from": "?position=1-2-3",
			"unknown parameter": "?" + from + "&position=1-2-3&x=1", "invalid text": "?" + from + "&position=1-2-3&text=%FF"} {
			if client, status, body := f.tryStream(bearer, query); client != nil || status != http.StatusBadRequest ||
				!strings.Contains(body, "INVALID_INPUT") {
				failures = append(failures, fmt.Errorf("%s: %d %s", name, status, body))
			}
		}
		if _, status, _ := f.tryStream("", "?"+from+"&position=1-2-3"); status != http.StatusForbidden {
			failures = append(failures, fmt.Errorf("a guest without grants = %d, want 403 before streaming", status))
		}
		if _, status, _ := f.tryStream("Bearer unknown", "?"+from+"&position=1-2-3"); status != http.StatusUnauthorized {
			failures = append(failures, fmt.Errorf("an unknown key = %d, want 401", status))
		}
		// Two admissions of the limit of two are used by two opens; the feed shares the window.
		f.openStream(bearer, "?"+from+"&position=1-2-3")
		f.openStream(bearer, "?"+from+"&position=1-2-3")
		if client, status, body := f.tryStream(bearer, "?"+from+"&position=1-2-3"); client != nil || status != http.StatusTooManyRequests ||
			!strings.Contains(body, "RATE_LIMITED") {
			failures = append(failures, fmt.Errorf("third open = %d %s, want 429 RATE_LIMITED", status, body))
		}
		return errors.Join(failures...)
	})
}

// mergeInto moves the absorbed Message's Transmissions and appearances to the survivor and
// deletes it, as Grouping does, and adds the joining Transmission to the survivor. It returns
// the merged fact without appending it.
func (f *liveFixture) mergeInto(survivor, absorbed uuid.UUID, at time.Time) events.IntakeFact {
	f.t.Helper()
	joining := uuid.New()
	f.exec(`INSERT INTO transmissions (id, gateway_id, network_id, submission_id, submission_fingerprint, network_resolution, protocol,
		payload, observed_at, observation_offset_seconds, accepted_at, source_revision, message_id)
		VALUES ($1, $2, $3, $4, $5, 'declared', 'pocsag', '{"form":"tone-only"}'::json, $6, 0, $6, 1, $7)`,
		joining, f.gateway, f.network, uuid.NewString(), make([]byte, 32), at, survivor)
	f.exec(`UPDATE transmissions SET message_id = $1 WHERE message_id = $2`, survivor, absorbed)
	f.exec(`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor)
		SELECT $1, unit_id, agency_id, first_observed_at, age_anchor FROM message_units WHERE message_id = $2
		ON CONFLICT DO NOTHING`, survivor, absorbed)
	f.exec(`DELETE FROM messages WHERE id = $1`, absorbed)
	return events.IntakeFact{ID: uuid.New(), Type: events.MessageMerged, MessageID: survivor, TransmissionID: joining,
		MessageTime: at, Absorbed: []uuid.UUID{absorbed}, Revision: f.revision.Add(1), OccurredAt: microsecondNow()}
}

// appendIntake appends the facts in one transaction, as one Grouping commit would.
func (f *liveFixture) appendIntake(facts ...events.IntakeFact) {
	f.t.Helper()
	f.appendFact(func(ctx context.Context, tx platform.TransactionContext) error {
		for _, fact := range facts {
			if _, err := f.core.facts.AppendIntakeFact(ctx, tx, fact); err != nil {
				return err
			}
		}
		return nil
	})
}

//nolint:tparallel,paralleltest // every open stream sees every merge, so the subtests run in order
func TestLiveStreamMergesResynchronizeWithoutDisclosingIDs(t *testing.T) {
	f := newLiveFixture(t, 0)
	a, b, c := f.unit("A", nil), f.unit("B", nil), f.unit("C", nil)
	base := microsecondNow().Add(-time.Hour)
	reader, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "unit", References: []string{a.String()}}, []string{"payload", "unit"})
	query := "?from=" + rfc(base.Add(-time.Hour))
	// A stream may replay the creation of Messages committed shortly before its position (the
	// position trails transactions still running anywhere in the cluster); such an upsert of an
	// already loaded Message is harmless and skipped, every other event is checked.
	// Only Messages the reader can see (Unit A) are skipped, so a disclosed hidden Message still fails.
	known := map[uuid.UUID]bool{}
	message := func(at time.Time, text string, unit uuid.UUID) uuid.UUID {
		id := f.newMessage(at, text, unit)
		known[id] = unit == a
		return id
	}
	// next returns the next event other than a replayed upsert of a known Message; want is never
	// skipped.
	next := func(t *testing.T, stream *sseClient, want ...uuid.UUID) sseEvent {
		t.Helper()
		for {
			event := stream.next(t)
			var data struct {
				MessageID uuid.UUID `json:"messageId"`
			}
			if event.Event != "upsert" || json.Unmarshal([]byte(event.Data), &data) != nil ||
				!known[data.MessageID] || slices.Contains(want, data.MessageID) {
				return event
			}
		}
	}
	open := func() *sseClient {
		t.Helper()
		return f.openStream(reader, query+"&position="+f.feed(reader, query, http.StatusOK).Data.Position)
	}
	expectResync := func(t *testing.T, stream *sseClient, hidden ...uuid.UUID) {
		t.Helper()
		event := next(t, stream)
		if event.Event != "resync-required" {
			t.Fatalf("event %+v, want resync-required", event)
		}
		for _, id := range hidden {
			if strings.Contains(event.Data, id.String()) {
				t.Fatalf("the resync discloses %s: %+v", id, event)
			}
		}
		stream.closed(t)
	}

	t.Run("a visible merge resynchronizes", func(t *testing.T) {
		survivor := message(base, "survivor", a)
		absorbed := message(base.Add(time.Minute), "absorbed", a)
		stream := open()
		f.appendIntake(f.mergeInto(survivor, absorbed, base))
		expectResync(t, stream)
	})
	t.Run("a hidden absorbed Message is never named", func(t *testing.T) {
		survivor := message(base.Add(2*time.Minute), "readable", a)
		hidden := message(base.Add(3*time.Minute), "hidden", b)
		stream := open()
		f.appendIntake(f.mergeInto(survivor, hidden, base.Add(2*time.Minute)))
		expectResync(t, stream, hidden)
	})
	t.Run("a merge of hidden Messages yields nothing", func(t *testing.T) {
		survivor := message(base.Add(4*time.Minute), "hidden survivor", b)
		absorbed := message(base.Add(5*time.Minute), "hidden absorbed", c)
		stream := open()
		f.appendIntake(f.mergeInto(survivor, absorbed, base.Add(4*time.Minute)))
		visible := message(base.Add(6*time.Minute), "still live", a)
		if event := next(t, stream, visible); event.Event != "upsert" || !strings.Contains(event.Data, visible.String()) {
			t.Fatalf("event %+v, want only the upsert of %s after a hidden merge", event, visible)
		}
	})
	t.Run("a chain of merges resolves to the final survivor", func(t *testing.T) {
		// A transaction left open across the setup makes the stream position trail the
		// creations, so the stream replays the loaded Message before the merges arrive.
		release := holdTransaction(t, f.store)
		defer release()
		loaded := message(base.Add(7*time.Minute), "loaded", a)
		middle := message(base.Add(8*time.Minute), "middle", b)
		final := message(base.Add(9*time.Minute), "final", b)
		stream := open()
		release()
		// The marker wakes the stream; its batch also replays the creations held back so far.
		marker := message(base.Add(10*time.Minute), "marker", a)
		if event := next(t, stream, marker); event.Event != "upsert" || !strings.Contains(event.Data, marker.String()) {
			t.Fatalf("event %+v, want the upsert of the marker %s", event, marker)
		}
		// The middle Message absorbs the loaded one and is itself absorbed before the stream reads
		// either fact, so the first fact's survivor no longer exists.
		first := f.mergeInto(middle, loaded, base.Add(7*time.Minute))
		second := f.mergeInto(final, middle, base.Add(7*time.Minute))
		f.appendIntake(first, second)
		expectResync(t, stream, middle, final)
	})
}

// TestLiveStreamAgeLimitedCallerGetsNoRemoveIDs checks that a caller whose grant has a maximum
// age is never told the ID of a Message that may have been too old for it to load: a correction
// that moves an old Message out of its Unit resynchronizes the view instead.
func TestLiveStreamAgeLimitedCallerGetsNoRemoveIDs(t *testing.T) {
	t.Parallel()
	f := newLiveFixture(t, 0)
	a, b := f.unit("A", nil), f.unit("B", nil)
	old := microsecondNow().Add(-48 * time.Hour)
	reader, principal := f.caller([]string{"message.read"}, authz.Scope{Kind: "unit", References: []string{a.String()}}, []string{"payload", "unit"})
	f.exec(`UPDATE identity_grants SET max_age_seconds = 3600 WHERE subject_id = $1`, principal)
	message := f.message(old, "too old", a)
	query := "?from=" + rfc(old.Add(-time.Hour))
	page := f.feed(reader, query, http.StatusOK)
	if len(page.Data.Items) != 0 {
		t.Fatalf("the old Message is outside the maximum age: %v", itemIDs(page.Data.Items))
	}
	stream := f.openStream(reader, query+"&position="+page.Data.Position)
	f.reassign(message, a, b, old)
	event := stream.next(t)
	if event.Event != "resync-required" || strings.Contains(event.Data, message.String()) {
		t.Fatalf("event %+v, want an ID-free resync-required", event)
	}
	stream.closed(t)
}

// tokenKey returns the installation's token secret, with which tests sign positions they move.
func (f *liveFixture) tokenKey() []byte {
	f.t.Helper()
	var key []byte
	err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT secret FROM query_token_keys WHERE id = 1`).Scan(&key)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return key
}

// holdTransaction keeps a transaction with an assigned ID open until the returned function is
// called (at most once has an effect), so that snapshots taken meanwhile have it as their xmin.
func holdTransaction(t *testing.T, store *postgres.Store) func() {
	t.Helper()
	started, done := make(chan error, 1), make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		var once sync.Once
		_ = store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err == nil {
				_, err = executor.ExecContext(tx.Context(), `SELECT pg_current_xact_id()`)
			}
			once.Do(func() { started <- err })
			<-done
			return errors.New("release the held transaction")
		})
	}()
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
}
