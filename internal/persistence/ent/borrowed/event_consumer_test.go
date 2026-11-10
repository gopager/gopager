package borrowed_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

const effectConsumerName = "effect-test"

// effectConsumer writes one row per handled fact into a table in the batch transaction.
type effectConsumer struct {
	store *postgres.Store
	mu    sync.Mutex
	fail  error
}

func (c *effectConsumer) Name() string { return effectConsumerName }

func (c *effectConsumer) Supports(eventType string, schemaVersion int) events.Support {
	if eventType != string(events.PrincipalAccessChanged) {
		return events.Unsupported
	}
	if schemaVersion != 1 {
		return events.UnknownVersion
	}
	return events.Supported
}

func (c *effectConsumer) setFail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fail = err
}

func (c *effectConsumer) Handle(ctx context.Context, tx platform.TransactionContext, record events.Record) error {
	c.mu.Lock()
	fail := c.fail
	c.mu.Unlock()
	executor, err := c.store.Resolve(tx)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, "INSERT INTO consumer_effects(event_id) VALUES ($1)", record.ID); err != nil {
		return err
	}
	return fail
}

// prepareEffectTable creates the effect table through an observer connection.
func prepareEffectTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE consumer_effects(event_id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
}

func effectCount(t *testing.T, db *sql.DB) (count int) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM consumer_effects").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func storedOffset(t *testing.T, store *postgres.Store, s *eventstore.Store) (position events.Position) {
	t.Helper()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) (err error) {
		position, err = s.Offset(tx.Context(), tx, effectConsumerName)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return position
}

// startEventRunner runs a runner with the given wake source until the test ends or stop is called.
func startEventRunner(t *testing.T, store *postgres.Store, s *eventstore.Store, consumer events.Consumer,
	wakes events.WakeSubscriber, poll time.Duration) (stop func()) {
	t.Helper()
	runner, err := events.NewRunner(consumer, s, store, wakes, slog.Default(), poll)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// idleWake is a subscriber that signals once and never again, so only the poll can wake the runner.
type idleWake struct{}

func (idleWake) Subscribe() (<-chan struct{}, func()) {
	signals := make(chan struct{}, 1)
	signals <- struct{}{}
	return signals, func() {}
}

func TestEventRunnerEffectAndOffsetCommitAtomicallyAndResume(t *testing.T) { //nolint:paralleltest // reads the event log, whose visibility horizon (pg_snapshot_xmin) is server-wide
	store := newStore(t)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	db := observerDatabase(t, store)
	prepareEffectTable(t, db)
	consumer := &effectConsumer{store: store}
	consumer.setFail(errors.New("effect refused"))
	commitFixtureFacts(t, store, s, uuid.New(), uuid.New())

	stop := startEventRunner(t, store, s, consumer, idleWake{}, 20*time.Millisecond)
	// Retries keep failing: neither the effect nor the offset is committed.
	time.Sleep(300 * time.Millisecond)
	if got := effectCount(t, db); got != 0 {
		t.Fatalf("effects of a failed batch committed: %d", got)
	}
	if got := storedOffset(t, store, s); got != (events.Position{}) {
		t.Fatalf("offset advanced by a failed batch: %v", got)
	}

	consumer.setFail(nil)
	waitUntil(t, "both facts handled", func() bool { return effectCount(t, db) == 2 })
	waitUntil(t, "offset advanced", func() bool { return storedOffset(t, store, s) != (events.Position{}) })
	stop()

	// A new runner resumes from the stored offset and handles only the later fact.
	commitFixtureFacts(t, store, s, uuid.New())
	startEventRunner(t, store, s, consumer, idleWake{}, 20*time.Millisecond)
	waitUntil(t, "third fact handled once", func() bool { return effectCount(t, db) == 3 })
	time.Sleep(200 * time.Millisecond)
	if got := effectCount(t, db); got != 3 {
		t.Fatalf("effects = %d after restart, want 3 (no reprocessing)", got)
	}
}

func TestEventRunnerWakesOnRealNotification(t *testing.T) { //nolint:paralleltest // reads the event log, whose visibility horizon (pg_snapshot_xmin) is server-wide
	store := newStore(t)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	db := observerDatabase(t, store)
	prepareEffectTable(t, db)
	hub, err := events.NewWakeHub(store, events.NotifyChannel)
	if err != nil {
		t.Fatal(err)
	}
	hubCtx, cancelHub := context.WithCancel(t.Context())
	hubDone := make(chan error, 1)
	go func() { hubDone <- hub.Run(hubCtx) }()
	t.Cleanup(func() {
		cancelHub()
		if err := <-hubDone; err != nil {
			t.Error(err)
		}
	})
	// The poll interval is far longer than the test, so only the notification can wake the runner.
	startEventRunner(t, store, s, &effectConsumer{store: store}, hub, time.Hour)
	// Let the runner's initial pass and the listener's first signal pass before the fact is appended.
	time.Sleep(500 * time.Millisecond)
	commitFixtureFacts(t, store, s, uuid.New())
	waitUntil(t, "fact handled after notification", func() bool { return effectCount(t, db) == 1 })
}
