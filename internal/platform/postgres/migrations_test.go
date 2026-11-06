package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/gopager/gopager/internal/platform"
)

func TestMigrationCheckWithReadOnlyConnection(t *testing.T) {
	store := newTestStore(t)
	// This setting applies to every physical test connection, not just one pooled session.
	readOnly := newStoreAtWithParams(t, storeTestDSN(store), map[string]string{"default_transaction_read_only": "on"})
	if err := readOnly.Migrate(context.Background(), nil, platform.MigrationCheck); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("read-only missing ledger: %v", err)
	}
	if err := store.Migrate(context.Background(), nil, platform.MigrationApply); err != nil {
		t.Fatal(err)
	}
	if err := readOnly.Migrate(context.Background(), nil, platform.MigrationCheck); err != nil {
		t.Fatalf("check attempted schema/history writes: %v", err)
	}
	if err := readOnly.CheckWriteAuthority(context.Background()); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("read-only connection acquired write authority: %v", err)
	}
}

func TestMigrationLockReleasedAfterCancelledWait(t *testing.T) {
	store := newTestStore(t)
	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := store.Migrate(ctx, nil, platform.MigrationApply); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait cancellation lost: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if err := store.Migrate(ctx, nil, platform.MigrationApply); err != nil {
		t.Fatalf("cancelled session leaked its lock: %v", err)
	}
}

func TestMigrationConcurrencyDoesNotExhaustPool(t *testing.T) {
	store := newTestStore(t)
	store.db.SetMaxOpenConns(2)
	files := migrationFiles("SELECT pg_sleep(0.02)")
	results := make(chan error, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for range 10 {
		go func() { results <- store.Migrate(ctx, files, platform.MigrationApply) }()
	}
	for range 10 {
		if err := <-results; err != nil {
			t.Fatal(fmt.Errorf("concurrent migration with bounded pool: %w", err))
		}
	}
}

func TestMigrationFailureMappingRetainsLibraryCause(t *testing.T) {
	err := &goose.PartialError{Err: errors.New("synthetic transport failure")}
	if !isUnknownCommit(err) {
		t.Fatal("untyped migration transport outcome was assumed rolled back")
	}
}
