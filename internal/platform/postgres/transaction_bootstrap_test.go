package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

// TestTransactionDoesNotWaitForMigrationLock holds the migration advisory lock in a
// separate session and expects an ordinary transaction to proceed anyway: only startup,
// apply and the readiness recheck take that lock.
func TestTransactionDoesNotWaitForMigrationLock(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	migrations := migrationFiles("CREATE TABLE synthetic_state (id int PRIMARY KEY)")
	if err := store.Migrate(ctx, migrations, platform.MigrationApply); err != nil {
		t.Fatalf("apply: %v", err)
	}
	holder, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatalf("dedicated connection: %v", err)
	}
	defer holder.Close()
	if _, err := holder.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		t.Fatalf("hold migration lock: %v", err)
	}
	defer func() {
		_, _ = holder.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	err = store.WithinTransaction(bounded, func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), "INSERT INTO synthetic_state (id) VALUES (1)")
		return err
	})
	if err != nil {
		t.Fatalf("transaction while the migration lock is held = %v, want success", err)
	}
}

// TestTransactionDetectsForeignMigrationWithoutLock keeps the runtime guarantee: a
// process whose registered collection is behind the database fails INCOMPATIBLE_VERSION
// at transaction start.
func TestTransactionDetectsForeignMigrationWithoutLock(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := migrationFiles("CREATE TABLE synthetic_state (id int PRIMARY KEY)")
	if err := store.Migrate(ctx, first, platform.MigrationApply); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	newer := newStoreInSchema(t, testDatabaseURL(t), storeTestSchema(t, store))
	both := migrationFiles("CREATE TABLE synthetic_state (id int PRIMARY KEY)", "CREATE TABLE synthetic_outbox (id int PRIMARY KEY)")
	if err := newer.Migrate(ctx, both, platform.MigrationApply); err != nil {
		t.Fatalf("apply v2 from another process: %v", err)
	}
	err := store.WithinTransaction(ctx, func(platform.TransactionContext) error { return nil })
	if platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("transaction on a newer database = %v, want INCOMPATIBLE_VERSION", err)
	}
}

// TestReadinessCheckKeepsPooledConnections expects the locked check to return its
// physical session to the pool after a successful unlock instead of discarding it.
func TestReadinessCheckKeepsPooledConnections(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	migrations := migrationFiles("CREATE TABLE synthetic_state (id int PRIMARY KEY)")
	if err := store.Migrate(ctx, migrations, platform.MigrationApply); err != nil {
		t.Fatalf("apply: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := store.Migrate(ctx, migrations, platform.MigrationCheck); err != nil {
			t.Fatalf("warm-up check %d: %v", i, err)
		}
	}
	newest := func() time.Time {
		var started time.Time
		if err := store.db.QueryRowContext(ctx, `SELECT max(backend_start) FROM pg_stat_activity
			WHERE datname = current_database() AND backend_type = 'client backend'`).Scan(&started); err != nil {
			t.Fatalf("read backend starts: %v", err)
		}
		return started
	}
	before := newest()
	for i := 0; i < 20; i++ {
		if err := store.Migrate(ctx, migrations, platform.MigrationCheck); err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
	}
	if after := newest(); after.After(before) {
		t.Fatalf("readiness checks opened new backends (newest backend_start moved from %v to %v)", before, after)
	}
}
