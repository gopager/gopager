package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// newTestStore opens a store in a fresh schema, removed when the test finishes.
func newTestStore(t testing.TB) *Store {
	t.Helper()
	return newStoreInSchema(t, testDatabaseURL(t), createTestSchema(t))
}

// newStoreInSchema opens another pool in the same isolated test schema.
func newStoreInSchema(t testing.TB, databaseURL, schema string) *Store {
	t.Helper()
	return newStoreInSchemaWithParams(t, databaseURL, schema, nil)
}

// newStoreInSchemaWithParams binds test-only connection settings on every pooled session.
func newStoreInSchemaWithParams(t testing.TB, databaseURL, schema string, params map[string]string) *Store {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	config.RuntimeParams["search_path"] = schema
	for key, value := range params {
		config.RuntimeParams[key] = value
	}
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(8)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return &Store{db: db}
}

// testDatabaseURL skips database-dependent cases only in the optional quick run.
func testDatabaseURL(t testing.TB) string {
	t.Helper()
	value := os.Getenv("GOPAGER_TEST_DATABASE_URL")
	if value == "" {
		t.Skip("PostgreSQL integration disabled; run with GOPAGER_INTEGRATION=1")
	}
	return value
}

// createTestSchema allocates only this test's schema and registers its cleanup.
func createTestSchema(t testing.TB) string {
	t.Helper()
	config, err := pgx.ParseConfig(testDatabaseURL(t))
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	db := stdlib.OpenDB(*config)
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate test schema name: %v", err)
	}
	schema := fmt.Sprintf("test_%x", random[:])
	if _, err := db.ExecContext(context.Background(), `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("drop isolated test schema: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Errorf("close schema setup database: %v", err)
		}
	})
	return schema
}

// storeTestSchema returns the store's isolated schema for a second pool.
func storeTestSchema(t testing.TB, store *Store) string {
	t.Helper()
	var schema string
	if err := store.db.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("read isolated schema name: %v", err)
	}
	return schema
}

// assertRowCounts checks both synthetic participants after a transaction outcome.
func assertRowCounts(t testing.TB, store *Store, state, outbox int) {
	t.Helper()
	var stateRows, outboxRows int
	if err := store.db.QueryRow("SELECT count(*) FROM synthetic_state").Scan(&stateRows); err != nil {
		t.Fatalf("count synthetic state: %v", err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM synthetic_outbox").Scan(&outboxRows); err != nil {
		t.Fatalf("count synthetic outbox: %v", err)
	}
	if stateRows != state || outboxRows != outbox {
		t.Fatalf("row counts state=%d outbox=%d, want state=%d outbox=%d", stateRows, outboxRows, state, outbox)
	}
}

type lostAcknowledgementTransaction struct {
	*sql.Tx
}

// Commit persists the real transaction, then simulates a lost acknowledgement.
func (t lostAcknowledgementTransaction) Commit() error {
	if err := t.Tx.Commit(); err != nil {
		return err
	}
	return errors.New("synthetic lost commit acknowledgement")
}

// migrationFiles builds isolated release artifacts for PostgreSQL conformance tests.
func migrationFiles(statements ...string) fstest.MapFS {
	files := make(fstest.MapFS, len(statements))
	for i, statement := range statements {
		files[fmt.Sprintf("%05d_test.sql", i+1)] = &fstest.MapFile{
			Data: []byte("-- +goose Up\n" + statement + ";\n"),
		}
	}
	return files
}
