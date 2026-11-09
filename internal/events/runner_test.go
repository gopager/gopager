package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
)

// fakeTx is a transaction handle.
type fakeTx struct{ ctx context.Context }

func (t fakeTx) Context() context.Context { return t.ctx }

// fakeWorld is an in-memory log, offset store and transactor. A transaction buffers its
// offset change and commits it only when the callback succeeds.
type fakeWorld struct {
	mu      sync.Mutex
	records []events.Record
	offset  events.Position
	commits int
	reads   int
}

func (w *fakeWorld) add(seq int64, eventType string, schemaVersion int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	envelope, _ := json.Marshal(map[string]any{"type": eventType, "schemaVersion": schemaVersion})
	w.records = append(w.records, events.Record{Seq: seq, TxID: uint64(seq), Type: eventType, Envelope: envelope})
}

func (w *fakeWorld) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	w.mu.Lock()
	staged := w.offset
	w.mu.Unlock()
	tx := &stagedTx{fakeTx: fakeTx{ctx: ctx}, offset: staged}
	if err := callback(tx); err != nil {
		return err
	}
	w.mu.Lock()
	w.offset = tx.offset
	w.commits++
	w.mu.Unlock()
	return nil
}

type stagedTx struct {
	fakeTx
	offset events.Position
}

func (w *fakeWorld) ReadAfter(_ context.Context, _ platform.TransactionContext, after events.Position, limit int) ([]events.Record, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reads++
	var out []events.Record
	for _, record := range w.records {
		if record.Position().Compare(after) > 0 && len(out) < limit {
			out = append(out, record)
		}
	}
	return out, nil
}

func (w *fakeWorld) Offset(_ context.Context, tx platform.TransactionContext, _ string) (events.Position, error) {
	return tx.(*stagedTx).offset, nil
}

func (w *fakeWorld) Advance(_ context.Context, tx platform.TransactionContext, _ string, position events.Position) error {
	staged := tx.(*stagedTx)
	if position.Compare(staged.offset) > 0 {
		staged.offset = position
	}
	return nil
}

func (w *fakeWorld) storedOffset() events.Position {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.offset
}

// fakeConsumer supports "known" at version 1, knows "versioned" only at version 1 and ignores the rest.
type fakeConsumer struct {
	mu      sync.Mutex
	handled []int64
	failOn  int64
	failed  int
}

func (c *fakeConsumer) Name() string { return "fake" }

func (c *fakeConsumer) Supports(eventType string, schemaVersion int) events.Support {
	switch eventType {
	case "known":
		return events.Supported
	case "versioned":
		if schemaVersion == 1 {
			return events.Supported
		}
		return events.UnknownVersion
	}
	return events.Unsupported
}

func (c *fakeConsumer) Handle(_ context.Context, _ platform.TransactionContext, record events.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if record.Seq == c.failOn {
		c.failed++
		return errors.New("handler failed")
	}
	c.handled = append(c.handled, record.Seq)
	return nil
}

func (c *fakeConsumer) handledSeqs() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.handled...)
}

func (c *fakeConsumer) setFailOn(seq int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failOn = seq
}

// manualWakes is a WakeSubscriber the test signals directly.
type manualWakes struct{ signals chan struct{} }

func newManualWakes() *manualWakes { return &manualWakes{signals: make(chan struct{}, 1)} }

func (m *manualWakes) Subscribe() (<-chan struct{}, func()) { return m.signals, func() {} }

func (m *manualWakes) wake() {
	select {
	case m.signals <- struct{}{}:
	default:
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func startRunner(t *testing.T, world *fakeWorld, consumer events.Consumer, wakes events.WakeSubscriber,
	poll time.Duration) (*events.Runner, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	runner, err := events.NewRunner(consumer, world, world, wakes, slog.New(slog.NewTextHandler(logs, nil)), poll)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return runner, logs
}

func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestRunnerRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
	if _, err := events.NewRunner(&fakeConsumer{}, world, world, newManualWakes(), logger, 0); err == nil {
		t.Fatal("zero poll interval accepted")
	}
	if _, err := events.NewRunner(nil, world, world, newManualWakes(), logger, time.Second); err == nil {
		t.Fatal("nil consumer accepted")
	}
}

func TestRunnerAdvancesOverUnsupportedAndHandlesSupportedInOrder(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	world.add(1, "other", 1)
	world.add(2, "known", 1)
	world.add(3, "other", 1)
	world.add(4, "known", 1)
	world.add(5, "other", 1)
	consumer := &fakeConsumer{}
	runner, _ := startRunner(t, world, consumer, newManualWakes(), time.Hour)
	eventually(t, "offset at 5", func() bool { return world.storedOffset().Seq == 5 })
	if got := fmt.Sprint(consumer.handledSeqs()); got != "[2 4]" {
		t.Fatalf("handled %s, want [2 4]", got)
	}
	if runner.Blocked() {
		t.Fatal("runner blocked without cause")
	}
}

func TestRunnerProcessesMoreThanOneBatch(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	for seq := int64(1); seq <= 250; seq++ {
		world.add(seq, "known", 1)
	}
	consumer := &fakeConsumer{}
	startRunner(t, world, consumer, newManualWakes(), time.Hour)
	eventually(t, "offset at 250", func() bool { return world.storedOffset().Seq == 250 })
	if got := len(consumer.handledSeqs()); got != 250 {
		t.Fatalf("handled %d, want 250", got)
	}
}

func TestRunnerBlocksOnUnknownVersionWithoutSkippingIt(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	world.add(1, "known", 1)
	world.add(2, "other", 1)
	world.add(3, "versioned", 2)
	world.add(4, "known", 1)
	consumer := &fakeConsumer{}
	wakes := newManualWakes()
	runner, logs := startRunner(t, world, consumer, wakes, 20*time.Millisecond)
	eventually(t, "blocked", runner.Blocked)
	if got := world.storedOffset().Seq; got != 2 {
		t.Fatalf("offset %d, want 2 (last record before the blocker)", got)
	}
	// The blocker is retried and logged at every tick and nothing behind it is handled.
	eventually(t, "repeated log", func() bool {
		return strings.Count(logs.String(), "event consumer blocked") >= 2
	})
	for _, want := range []string{"consumer=fake", "seq=3", "type=versioned", "schemaVersion=2"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log lacks %q: %s", want, logs.String())
		}
	}
	if got := fmt.Sprint(consumer.handledSeqs()); got != "[1]" {
		t.Fatalf("handled %s, want [1]", got)
	}
	if world.storedOffset().Seq != 2 {
		t.Fatal("offset moved past the blocker")
	}
}

func TestRunnerBlockedAtFirstRecordAdvancesNothing(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	world.add(1, "versioned", 7)
	runner, _ := startRunner(t, world, &fakeConsumer{}, newManualWakes(), 20*time.Millisecond)
	eventually(t, "blocked", runner.Blocked)
	if world.storedOffset() != (events.Position{}) {
		t.Fatalf("offset %v, want zero", world.storedOffset())
	}
}

func TestRunnerRollsBackBatchOnHandlerErrorAndRetries(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	world.add(1, "known", 1)
	world.add(2, "known", 1)
	consumer := &fakeConsumer{failOn: 2}
	wakes := newManualWakes()
	_, logs := startRunner(t, world, consumer, wakes, 20*time.Millisecond)
	eventually(t, "failure retried", func() bool {
		consumer.mu.Lock()
		defer consumer.mu.Unlock()
		return consumer.failed >= 2
	})
	if world.storedOffset() != (events.Position{}) {
		t.Fatalf("offset %v advanced despite a failed batch", world.storedOffset())
	}
	if !strings.Contains(logs.String(), "event consumer batch failed") {
		t.Fatalf("failure not logged: %s", logs.String())
	}
	consumer.setFailOn(0)
	eventually(t, "offset at 2", func() bool { return world.storedOffset().Seq == 2 })
}

func TestRunnerWakesOnHubSignal(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	consumer := &fakeConsumer{}
	wakes := newManualWakes()
	startRunner(t, world, consumer, wakes, time.Hour)
	eventually(t, "initial pass", func() bool {
		world.mu.Lock()
		defer world.mu.Unlock()
		return world.commits >= 1
	})
	world.add(1, "known", 1)
	wakes.wake()
	eventually(t, "handled after wake", func() bool { return world.storedOffset().Seq == 1 })
}

func TestRunnerPollsWithoutWakeUp(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	consumer := &fakeConsumer{}
	startRunner(t, world, consumer, newManualWakes(), 20*time.Millisecond)
	world.add(1, "known", 1)
	eventually(t, "handled by poll", func() bool { return world.storedOffset().Seq == 1 })
}

func TestRunnerResumesFromStoredOffset(t *testing.T) {
	t.Parallel()
	world := &fakeWorld{}
	world.add(1, "known", 1)
	first := &fakeConsumer{}
	runnerCtx, cancel := context.WithCancel(t.Context())
	logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
	runner, err := events.NewRunner(first, world, world, newManualWakes(), logger, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runner.Run(runnerCtx) }()
	eventually(t, "first handled", func() bool { return world.storedOffset().Seq == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	world.add(2, "known", 1)
	second := &fakeConsumer{}
	startRunner(t, world, second, newManualWakes(), time.Hour)
	eventually(t, "second handled", func() bool { return world.storedOffset().Seq == 2 })
	if got := fmt.Sprint(second.handledSeqs()); got != "[2]" {
		t.Fatalf("restarted runner handled %s, want [2]", got)
	}
}

func TestSchemaVersion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		envelope string
		want     int
	}{
		{"present", `{"schemaVersion":3}`, 3},
		{"missing", `{}`, 0},
		{"malformed", `nope`, 0},
		{"wrong type", `{"schemaVersion":"1"}`, 0},
	} {
		if got := events.SchemaVersion(events.Record{Envelope: json.RawMessage(test.envelope)}); got != test.want {
			t.Errorf("%s: got %d, want %d", test.name, got, test.want)
		}
	}
}
