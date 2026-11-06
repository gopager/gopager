package borrowed_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

// Compile-time proof that the event store satisfies the log reader port.
var _ events.Log = (*eventstore.Store)(nil)

// observerDatabase opens an independent observer pool on the fixture database, so assertions
// see only committed data.
func observerDatabase(t *testing.T, store *postgres.Store) *sql.DB {
	t.Helper()
	config, err := pgx.ParseConfig(storeDSN(t, store))
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

// appendFixtureFact appends one identity fact with the given ID in the caller's transaction.
func appendFixtureFact(store *postgres.Store, s *eventstore.Store, tx platform.TransactionContext, id uuid.UUID) error {
	now, err := store.Now(tx.Context(), tx)
	if err != nil {
		return err
	}
	_, err = s.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
		ID: id, Type: events.PrincipalAccessChanged, Aggregate: events.EntityRef{Type: "account", ID: uuid.New()},
		Revision: 1, OccurredAt: now, ChangeKind: events.GrantsChanged,
	})
	return err
}

// commitFixtureFacts appends one fact per ID in a single committed transaction.
func commitFixtureFacts(t *testing.T, store *postgres.Store, s *eventstore.Store, ids ...uuid.UUID) {
	t.Helper()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		for _, id := range ids {
			if err := appendFixtureFact(store, s, tx, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// readLog reads one page of the log in a fresh transaction.
func readLog(t *testing.T, store *postgres.Store, s *eventstore.Store, after events.Position, limit int) []events.Record {
	t.Helper()
	var records []events.Record
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) (err error) {
		records, err = s.ReadAfter(tx.Context(), tx, after, limit)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return records
}

// recordIDs projects records to their IDs.
func recordIDs(records []events.Record) []uuid.UUID {
	ids := make([]uuid.UUID, len(records))
	for i, record := range records {
		ids[i] = record.ID
	}
	return ids
}

// assertEventLogCounts checks that the account and the fact commit or roll back together and
// that no second transport table exists.
func assertEventLogCounts(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var accounts, facts int
	if err := db.QueryRowContext(t.Context(), `SELECT
		(SELECT count(*) FROM identity_accounts),
		(SELECT count(*) FROM domain_events)`).Scan(&accounts, &facts); err != nil {
		t.Fatal(err)
	}
	if accounts != want || facts != want {
		t.Fatalf("partial transaction: accounts=%d facts=%d; want %d", accounts, facts, want)
	}
}

func TestEventLogAllProducersAreAtomic(t *testing.T) {
	type appendFact func(*eventstore.Store, platform.TransactionContext, uuid.UUID, time.Time) (uuid.UUID, error)
	producers := map[string]appendFact{
		"identity": func(s *eventstore.Store, tx platform.TransactionContext, id uuid.UUID, now time.Time) (uuid.UUID, error) {
			return s.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
				ID: id, Type: events.PrincipalAccessChanged, Aggregate: events.EntityRef{Type: "account", ID: uuid.New()},
				Revision: 1, OccurredAt: now, ChangeKind: events.GrantsChanged,
			})
		},
		"master-data": func(s *eventstore.Store, tx platform.TransactionContext, id uuid.UUID, now time.Time) (uuid.UUID, error) {
			return s.AppendConfigurationFact(tx.Context(), tx, events.ConfigurationFact{
				ID: id, Aggregate: events.EntityRef{Type: "unit", ID: uuid.New()},
				Revision: 1, OccurredAt: now, ChangeKind: "created",
			})
		},
		"processing-configuration": func(s *eventstore.Store, tx platform.TransactionContext, id uuid.UUID, now time.Time) (uuid.UUID, error) {
			return s.AppendProcessingConfiguration(tx.Context(), tx, events.ProcessingConfigurationFact{
				ID: id, WorkflowID: uuid.New(), Revision: 1, OccurredAt: now, ChangeKind: "created",
			})
		},
		"processing-instance": func(s *eventstore.Store, tx platform.TransactionContext, id uuid.UUID, now time.Time) (uuid.UUID, error) {
			return s.AppendProcessingInstance(tx.Context(), tx, events.ProcessingInstanceFact{
				ID: id, InstanceID: uuid.New(), Revision: 1, OccurredAt: now, ChangeKind: "created",
			})
		},
		"processing-completion": func(s *eventstore.Store, tx platform.TransactionContext, id uuid.UUID, now time.Time) (uuid.UUID, error) {
			// Publication fixture references are not production originals.
			return s.AppendProcessingCompletion(tx.Context(), tx, events.ProcessingCompletionFact{
				ID: id, WorkflowID: uuid.New(), ResultID: uuid.New(), ResultRevision: 1,
				TransmissionID: uuid.New(), MessageID: uuid.New(), DefinitionVersionID: uuid.New(),
				SourceRevision: 1, ContextRevision: 1, OccurredAt: now,
				OutputSchema: map[string]processing.FieldSchema{},
				Result:       processing.Evaluation{Selected: true, Fields: map[string]processing.FieldValue{}},
			})
		},
	}
	for name, append := range producers {
		t.Run(name, func(t *testing.T) {
			store := newStore(t)
			db := observerDatabase(t, store)
			s, err := eventstore.New(store)
			if err != nil {
				t.Fatal(err)
			}
			var saved platform.TransactionContext
			write := func(id uuid.UUID, abort error) error {
				return store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					saved = tx
					now, err := store.Now(tx.Context(), tx)
					if err != nil {
						return err
					}
					driver, err := borrowed.New(store, tx)
					if err != nil {
						return err
					}
					client := ent.NewClient(ent.Driver(driver))
					ctx := accountContext(tx.Context(), driver, entql.FieldNotNil("id"), true)
					if err := createAccount(ctx, client, uuid.New(), uuid.NewString(), now); err != nil {
						return err
					}
					got, err := append(s, tx, id, now)
					if err != nil {
						return err
					}
					if got != id {
						t.Fatalf("append returned %s, want %s", got, id)
					}
					return abort
				})
			}
			id := uuid.New()
			if err := write(id, nil); err != nil {
				t.Fatal(err)
			}
			assertEventLogCounts(t, db, 1)
			var seq int64
			var eventType, canonicalID string
			var occurredAt time.Time
			if err := db.QueryRowContext(t.Context(), `SELECT seq, type, occurred_at, envelope->>'domainEventId'
				FROM domain_events WHERE id=$1`, id).Scan(&seq, &eventType, &occurredAt, &canonicalID); err != nil {
				t.Fatal(err)
			}
			if seq <= 0 || eventType == "" || occurredAt.IsZero() || canonicalID != id.String() {
				t.Fatalf("generated columns not populated: seq=%d type=%q occurred_at=%v id=%s", seq, eventType, occurredAt, canonicalID)
			}
			abort := errors.New("abort after append")
			if err := write(uuid.New(), abort); !errors.Is(err, abort) {
				t.Fatalf("caller abort lost: %v", err)
			}
			assertEventLogCounts(t, db, 1)
			if err := write(id, nil); platform.FailureCode(err) != platform.CodeRevisionConflict {
				t.Fatalf("duplicate canonical fact did not abort: %v", err)
			}
			assertEventLogCounts(t, db, 1)
			if _, err := append(s, saved, uuid.New(), time.Now().UTC().Truncate(time.Microsecond)); platform.FailureCode(err) != platform.CodeInvalidInput {
				t.Fatalf("expired capability accepted: %v", err)
			}
			other := newStore(t)
			err = other.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				_, err := append(s, tx, uuid.New(), time.Now().UTC().Truncate(time.Microsecond))
				return err
			})
			if platform.FailureCode(err) != platform.CodeInvalidInput {
				t.Fatalf("foreign capability accepted: %v", err)
			}
		})
	}
}

func TestEventLogReadAfterNeverSkipsAnUncommittedLowerSeq(t *testing.T) {
	store := newStore(t)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	first, second := uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		// Transaction A takes the lower seq and stays open.
		if err := appendFixtureFact(store, s, tx, first); err != nil {
			return err
		}
		// Transaction B takes a higher seq and commits first.
		commitFixtureFacts(t, store, s, second)
		if got := readLog(t, store, s, events.Position{}, 10); len(got) != 0 {
			t.Fatalf("reader saw %d records beyond an uncommitted lower seq: %v", len(got), recordIDs(got))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := readLog(t, store, s, events.Position{}, 10)
	if want := []uuid.UUID{first, second}; !slices.Equal(recordIDs(got), want) {
		t.Fatalf("after commit got %v, want %v in seq order", recordIDs(got), want)
	}
	if got[0].Seq >= got[1].Seq {
		t.Fatalf("records not in seq order: %d, %d", got[0].Seq, got[1].Seq)
	}
}

func TestEventLogReadAfterRespectsLastSeqAndLimit(t *testing.T) {
	store := newStore(t)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	for _, id := range ids {
		commitFixtureFacts(t, store, s, id)
	}
	page := readLog(t, store, s, events.Position{}, 2)
	if !slices.Equal(recordIDs(page), ids[:2]) {
		t.Fatalf("limit page got %v, want %v", recordIDs(page), ids[:2])
	}
	first := page[0]
	if first.Type != string(events.PrincipalAccessChanged) || first.OccurredAt.IsZero() || len(first.Envelope) == 0 {
		t.Fatalf("record fields not populated: %+v", first)
	}
	next := readLog(t, store, s, page[1].Position(), 10)
	if !slices.Equal(recordIDs(next), ids[2:]) {
		t.Fatalf("resume after %+v got %v, want %v", page[1].Position(), recordIDs(next), ids[2:])
	}
	if got := readLog(t, store, s, next[1].Position(), 10); len(got) != 0 {
		t.Fatalf("read past the end returned %v", recordIDs(got))
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := s.ReadAfter(tx.Context(), tx, events.Position{}, 0)
		return err
	})
	if platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("non-positive limit accepted: %v", err)
	}
}

func TestConsumerOffsetRoundTripAndNeverMovesBackwards(t *testing.T) {
	store := newStore(t)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	offset := func(consumer string) (got events.Position) {
		t.Helper()
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) (err error) {
			got, err = s.Offset(tx.Context(), tx, consumer)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}
	advance := func(consumer string, position events.Position) error {
		return store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			return s.Advance(tx.Context(), tx, consumer, position)
		})
	}
	if got := offset("projection"); got != (events.Position{}) {
		t.Fatalf("unknown consumer offset = %+v, want zero", got)
	}
	// The order is (TxID, Seq): a higher TxID wins even with a lower Seq.
	p := func(txID uint64, seq int64) events.Position { return events.Position{TxID: txID, Seq: seq} }
	for _, step := range []struct{ advance, want events.Position }{
		{p(5, 5), p(5, 5)}, {p(5, 9), p(5, 9)}, {p(5, 3), p(5, 9)}, {p(4, 99), p(5, 9)},
		{p(5, 9), p(5, 9)}, {p(7, 2), p(7, 2)}, {p(6, 50), p(7, 2)},
	} {
		if err := advance("projection", step.advance); err != nil {
			t.Fatalf("advance to %+v: %v", step.advance, err)
		}
		if got := offset("projection"); got != step.want {
			t.Fatalf("after advance to %+v offset = %+v, want %+v", step.advance, got, step.want)
		}
	}
	if got := offset("other"); got != (events.Position{}) {
		t.Fatalf("consumers share an offset: %+v", got)
	}
	if err := advance("projection", p(8, -1)); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("negative seq accepted: %v", err)
	}
	abort := errors.New("abort after advance")
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if err := s.Advance(tx.Context(), tx, "projection", p(50, 50)); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatalf("caller abort lost: %v", err)
	}
	if got := offset("projection"); got != p(7, 2) {
		t.Fatalf("rolled-back advance persisted: %+v", got)
	}
}

func TestEventLogAppendNotifiesAfterCommit(t *testing.T) {
	store := newStore(t)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	db := observerDatabase(t, store)
	listener, err := pgx.Connect(t.Context(), os.Getenv("GOPAGER_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if _, err := listener.Exec(t.Context(), "LISTEN "+events.NotifyChannel); err != nil {
		t.Fatal(err)
	}
	// NOTIFY is database-wide, so notifications of other tests may interleave; the test
	// therefore tracks specific sequence numbers instead of counting messages.
	rolledBack, committed := uuid.New(), uuid.New()
	abort := errors.New("abort after notify")
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if err := appendFixtureFact(store, s, tx, rolledBack); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	commitFixtureFacts(t, store, s, committed)
	var committedSeq int64
	if err := db.QueryRowContext(t.Context(), "SELECT seq FROM domain_events WHERE id=$1", committed).Scan(&committedSeq); err != nil {
		t.Fatal(err)
	}
	want := strconv.FormatInt(committedSeq, 10)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var received []*pgconn.Notification
	for {
		notification, err := listener.WaitForNotification(ctx)
		if err != nil {
			t.Fatalf("no notification with payload %s (received %v): %v", want, received, err)
		}
		if notification.Channel != events.NotifyChannel {
			t.Fatalf("unexpected channel %q", notification.Channel)
		}
		received = append(received, notification)
		if notification.Payload == want {
			break
		}
	}
	// The rolled-back append consumed seq committedSeq-1 and must never be announced.
	rolledBackSeq := strconv.FormatInt(committedSeq-1, 10)
	for _, notification := range received {
		if notification.Payload == rolledBackSeq {
			t.Fatalf("rolled-back append was announced: %v", received)
		}
	}
}

// TestEventLogReadAfterNeverSkipsALowerSeqOfAHigherTransaction reproduces the reversed-xid
// interleaving: transaction T takes the lower xid (early write), transaction X then takes the
// higher xid and appends the lower seq and stays open, T appends the higher seq and commits.
// A consumer that read and advanced in that window must still receive X's fact once it commits.
func TestEventLogReadAfterNeverSkipsALowerSeqOfAHigherTransaction(t *testing.T) {
	store := newStore(t)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	lowerSeq, higherSeq := uuid.New(), uuid.New()
	tHasXID, xAppended, tCommitted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	tDone := make(chan error, 1)
	go func() {
		tDone <- store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			var xid string
			if err := executor.QueryRowContext(tx.Context(), "SELECT pg_current_xact_id()::text").Scan(&xid); err != nil {
				return err
			}
			close(tHasXID)
			<-xAppended
			return appendFixtureFact(store, s, tx, higherSeq)
		})
		close(tCommitted)
	}()
	<-tHasXID
	var delivered []uuid.UUID
	var cursor events.Position
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if err := appendFixtureFact(store, s, tx, lowerSeq); err != nil {
			return err
		}
		close(xAppended)
		<-tCommitted
		got := readLog(t, store, s, cursor, 10)
		delivered = append(delivered, recordIDs(got)...)
		if len(got) > 0 {
			cursor = got[len(got)-1].Position()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-tDone; err != nil {
		t.Fatal(err)
	}
	delivered = append(delivered, recordIDs(readLog(t, store, s, cursor, 10))...)
	if !slices.Contains(delivered, lowerSeq) || !slices.Contains(delivered, higherSeq) {
		t.Fatalf("consumer lost a fact: delivered %v, want both %v and %v", delivered, lowerSeq, higherSeq)
	}
}
