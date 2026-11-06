package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

// newTestStore opens a store on a fresh empty database, dropped when the test finishes.
func newTestStore(t testing.TB) *Store {
	t.Helper()
	return newStoreAt(t, testpostgres.EmptyDatabase(t))
}

// newStoreAt opens a pool on the given test database.
func newStoreAt(t testing.TB, databaseURL string) *Store {
	t.Helper()
	return newStoreAtWithParams(t, databaseURL, nil)
}

// newStoreAtWithParams binds test-only connection settings on every pooled session.
func newStoreAtWithParams(t testing.TB, databaseURL string, params map[string]string) *Store {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
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
	return &Store{db: db, config: config}
}

// storeTestDSN returns the DSN of the store's isolated database for a second pool.
func storeTestDSN(store *Store) string {
	return store.config.ConnString()
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
