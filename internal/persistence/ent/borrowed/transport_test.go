package borrowed_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"entgo.io/ent/entql"
	"github.com/ThreeDotsLabs/watermill"
	wmsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

// transportDatabase opens an independent observer/subscriber pool in the fixture schema.
func transportDatabase(t *testing.T, store *postgres.Store) *sql.DB {
	t.Helper()
	var schema string
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), "SELECT current_schema()").Scan(&schema)
	}); err != nil {
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(os.Getenv("GOPAGER_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

// assertTransportCounts checks all three participants through an independent connection.
func assertTransportCounts(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var accounts, facts, references int
	if err := db.QueryRowContext(t.Context(), `SELECT
		(SELECT count(*) FROM identity_accounts),
		(SELECT count(*) FROM domain_events),
		(SELECT count(*) FROM watermill_domain_facts)`).
		Scan(&accounts, &facts, &references); err != nil {
		t.Fatal(err)
	}
	if accounts != want || facts != want || references != want {
		t.Fatalf("partial transaction: accounts=%d facts=%d references=%d; want %d", accounts, facts, references, want)
	}
}

func TestFactTransportAllProducersAreAtomic(t *testing.T) {
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
			// Publication fixture references are not production AP-06 originals.
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
			db := transportDatabase(t, store)
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
			assertTransportCounts(t, db, 1)
			var payload, metadata []byte
			var canonicalID string
			if err := db.QueryRowContext(t.Context(), `SELECT t.payload, t.metadata,
				d.envelope->>'domainEventId' FROM watermill_domain_facts t
				JOIN domain_events d ON d.id::text=t.uuid WHERE t.uuid=$1`, id.String()).
				Scan(&payload, &metadata, &canonicalID); err != nil {
				t.Fatal(err)
			}
			var reference map[string]any
			var meta map[string]any
			if err := json.Unmarshal(payload, &reference); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(metadata, &meta); err != nil {
				t.Fatal(err)
			}
			if len(reference) != 2 || reference["schemaVersion"] != float64(1) ||
				reference["domainEventId"] != id.String() || canonicalID != id.String() || len(meta) != 0 {
				t.Fatalf("unexpected transport body/metadata: %s %s canonical=%s", payload, metadata, canonicalID)
			}
			abort := errors.New("abort after publication")
			if err := write(uuid.New(), abort); !errors.Is(err, abort) {
				t.Fatalf("caller abort lost: %v", err)
			}
			assertTransportCounts(t, db, 1)
			if _, err := db.ExecContext(t.Context(), `ALTER TABLE watermill_domain_facts
				ADD CONSTRAINT fixture_transport_failure CHECK (false) NOT VALID`); err != nil {
				t.Fatal(err)
			}
			err = write(uuid.New(), nil)
			var pgError *pgconn.PgError
			if !errors.As(err, &pgError) || pgError.Code != "23514" {
				t.Fatalf("transport failure was hidden or changed: %v", err)
			}
			assertTransportCounts(t, db, 1)
			if _, err := db.ExecContext(t.Context(), `ALTER TABLE watermill_domain_facts
				DROP CONSTRAINT fixture_transport_failure`); err != nil {
				t.Fatal(err)
			}
			if err := write(id, nil); platform.FailureCode(err) != platform.CodeRevisionConflict {
				t.Fatalf("duplicate canonical fact did not abort: %v", err)
			}
			assertTransportCounts(t, db, 1)
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

// factSubscriber uses only migrated tables; it cannot initialize schema at runtime.
func factSubscriber(t *testing.T, db *sql.DB, group string) (*wmsql.Subscriber, <-chan *message.Message) {
	t.Helper()
	s, err := wmsql.NewSubscriber(wmsql.BeginnerFromStdSQL(db), wmsql.SubscriberConfig{
		ConsumerGroup: group, SchemaAdapter: wmsql.DefaultPostgreSQLSchema{SubscribeBatchSize: 1},
		OffsetsAdapter: wmsql.DefaultPostgreSQLOffsetsAdapter{},
		PollInterval:   10 * time.Millisecond, ResendInterval: 10 * time.Millisecond,
	}, watermill.NopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	messages, err := s.Subscribe(t.Context(), eventstore.FactTopic)
	if err != nil {
		t.Fatal(err)
	}
	return s, messages
}

// receiveFact validates the delivered reference against its stored canonical envelope.
func receiveFact(t *testing.T, db *sql.DB, messages <-chan *message.Message, id uuid.UUID) *message.Message {
	t.Helper()
	select {
	case msg, ok := <-messages:
		if !ok {
			t.Fatal("subscriber closed before delivery")
		}
		var reference eventstore.FactReference
		if err := json.Unmarshal(msg.Payload, &reference); err != nil {
			t.Fatal(err)
		}
		if msg.UUID != id.String() || reference.SchemaVersion != 1 || reference.DomainEventID != id {
			t.Fatalf("incorrect/redelivered different fact: %s %s", msg.UUID, msg.Payload)
		}
		var canonicalID string
		if err := db.QueryRowContext(t.Context(), `SELECT envelope->>'domainEventId'
			FROM domain_events WHERE id=$1`, reference.DomainEventID).Scan(&canonicalID); err != nil {
			t.Fatal(err)
		}
		if canonicalID != id.String() {
			t.Fatalf("canonical ID mismatch: %s", canonicalID)
		}
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for committed fact")
		return nil
	}
}

// waitFactAck observes the committed offset, not just the in-memory Ack signal.
func waitFactAck(t *testing.T, db *sql.DB, group string, id uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var persisted bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM watermill_offsets_domain_facts o
			JOIN watermill_domain_facts f ON f."offset"=o.offset_acked
				AND f.transaction_id=o.last_processed_transaction_id
			WHERE o.consumer_group=$1 AND f.uuid=$2)`, group, id.String()).Scan(&persisted); err != nil {
			t.Fatal(err)
		}
		if persisted {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("transport Ack never committed")
		case <-ticker.C:
		}
	}
}

func TestFactTransportCommitRedeliveryAndIndependentGroups(t *testing.T) {
	store := newStore(t)
	db := transportDatabase(t, store)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	first, messages := factSubscriber(t, db, "fixture_projection")
	append := func(tx platform.TransactionContext, id uuid.UUID) error {
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
	abortedID := uuid.New()
	abort := errors.New("abort fixture")
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if err := append(tx, abortedID); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if err := append(tx, id); err != nil {
			return err
		}
		var visible int
		if err := db.QueryRowContext(tx.Context(), "SELECT count(*) FROM watermill_domain_facts").Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			t.Fatalf("transport visible before commit: %d", visible)
		}
		select {
		case msg := <-messages:
			t.Fatalf("delivered rolled-back or uncommitted fact: %v", msg)
		case <-time.After(100 * time.Millisecond):
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	receiveFact(t, db, messages, id)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	_, restarted := factSubscriber(t, db, "fixture_projection")
	receiveFact(t, db, restarted, id).Ack()
	waitFactAck(t, db, "fixture_projection", id)
	_, independent := factSubscriber(t, db, "fixture_other_projection")
	receiveFact(t, db, independent, id).Ack()
	waitFactAck(t, db, "fixture_other_projection", id)
	var facts, references int
	if err := db.QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM domain_events),
		(SELECT count(*) FROM watermill_domain_facts)`).Scan(&facts, &references); err != nil {
		t.Fatal(err)
	}
	if facts != 1 || references != 1 {
		t.Fatalf("rollback/redelivery duplicated facts: %d %d", facts, references)
	}
}

func TestFactTransportMigrationUpgradeAndMissingStorage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "00008_fact_transport.sql" {
			continue
		}
		data, err := migrations.Files.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: data}
	}
	store := newStoreWithMigrations(t, previous)
	db := transportDatabase(t, store)
	s, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	legacyID := uuid.New()
	legacy := []byte(`{"fixture":"preexisting canonical fact"}`)
	if _, err := db.ExecContext(t.Context(), "INSERT INTO domain_events (id,envelope) VALUES ($1,$2)", legacyID, legacy); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(t.Context(), migrations.Files, platform.MigrationCheck); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("missing migration was accepted: %v", err)
	}
	if err := store.Migrate(t.Context(), migrations.Files, platform.MigrationApply); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(t.Context(), migrations.Files, platform.MigrationCheck); err != nil {
		t.Fatal(err)
	}
	var references, legacyFacts int
	if err := db.QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM watermill_domain_facts),
		(SELECT count(*) FROM domain_events WHERE id=$1)`, legacyID).Scan(&references, &legacyFacts); err != nil {
		t.Fatal(err)
	}
	if references != 0 || legacyFacts != 1 {
		t.Fatalf("migration replayed or removed legacy facts: %d %d", references, legacyFacts)
	}
	for _, drift := range []struct {
		name string
		sql  string
		code string
	}{
		{"incompatible column", "ALTER TABLE watermill_domain_facts RENAME COLUMN payload TO incompatible_payload", "42703"},
		{"missing table", "DROP TABLE watermill_domain_facts", "42P01"},
	} {
		t.Run(drift.name, func(t *testing.T) {
			if _, err := db.ExecContext(t.Context(), drift.sql); err != nil {
				t.Fatal(err)
			}
			err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				now, err := store.Now(tx.Context(), tx)
				if err != nil {
					return err
				}
				_, err = s.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
					ID: uuid.New(), Type: events.PrincipalAccessChanged, Aggregate: events.EntityRef{Type: "account", ID: uuid.New()},
					Revision: 1, OccurredAt: now, ChangeKind: events.GrantsChanged,
				})
				return err
			})
			var pgError *pgconn.PgError
			if !errors.As(err, &pgError) || pgError.Code != drift.code {
				t.Fatalf("storage drift did not fail publication: %v", err)
			}
			var facts int
			if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM domain_events").Scan(&facts); err != nil {
				t.Fatal(err)
			}
			if facts != 1 {
				t.Fatalf("invalid transport left orphan canonical facts: %d", facts)
			}
		})
	}
}
