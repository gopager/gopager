package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

func TestNewStoreRequiresVerifiedTLSForRemoteHosts(t *testing.T) {
	insecureURL := "postgres://user:secret-marker@db.example.test/gopager?sslmode=disable"
	if _, _, err := NewStore(insecureURL, false); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("remote non-TLS DSN error = %v, want INVALID_INPUT", err)
	} else if strings.Contains(err.Error(), "secret-marker") {
		t.Fatalf("safe configuration error exposed a credential: %q", err)
	}
	if _, _, err := NewStore("postgres://user:secret-marker@db.example.test/gopager", false); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("remote default TLS mode error = %v, want INVALID_INPUT", err)
	}

	store, insecure, err := NewStore("postgres://user:secret-marker@db.example.test/gopager?sslmode=verify-full", false)
	if err != nil {
		t.Fatalf("verified TLS configuration rejected: %v", err)
	}
	_ = store.Close()
	if insecure {
		t.Fatal("verified TLS connection was marked insecure")
	}

	store, insecure, err = NewStore(insecureURL, true)
	if err != nil {
		t.Fatalf("explicitly approved insecure connection rejected: %v", err)
	}
	_ = store.Close()
	if !insecure {
		t.Fatal("remote unverified connection did not request an Operator warning")
	}
}

func TestMigrationsAgainstPostgreSQL(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := migrationFiles("CREATE TABLE synthetic_one (id BIGINT PRIMARY KEY)")
	migrations := migrationFiles(
		"CREATE TABLE synthetic_one (id BIGINT PRIMARY KEY)",
		"CREATE TABLE synthetic_two (id BIGINT PRIMARY KEY)",
	)

	if err := store.Migrate(ctx, first, platform.MigrationCheck); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("check on missing ledger error = %v, want INCOMPATIBLE_VERSION", err)
	}
	var ledger sql.NullString
	if err := store.db.QueryRowContext(ctx, "SELECT to_regclass('platform_schema_migrations')").Scan(&ledger); err != nil {
		t.Fatalf("check made an unexpected schema change: %v", err)
	}
	if ledger.Valid {
		t.Fatal("check mode created the migration ledger")
	}

	if err := store.Migrate(ctx, first, platform.MigrationApply); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	if err := store.Migrate(ctx, migrations, platform.MigrationCheck); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("check with pending v2 error = %v, want INCOMPATIBLE_VERSION", err)
	}
	if err := store.Migrate(ctx, migrations, platform.MigrationApply); err != nil {
		t.Fatalf("apply v2: %v", err)
	}
	if err := store.Migrate(ctx, migrations, platform.MigrationApply); err != nil {
		t.Fatalf("repeat apply: %v", err)
	}
	if err := store.Migrate(ctx, migrations, platform.MigrationCheck); err != nil {
		t.Fatalf("check current: %v", err)
	}

	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM platform_schema_migrations WHERE version_id > 0").Scan(&count); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("applied migration count = %d, want 2", count)
	}

	changed := migrationFiles(
		"CREATE TABLE synthetic_one (id BIGINT PRIMARY KEY, extra TEXT)",
		"CREATE TABLE synthetic_two (id BIGINT PRIMARY KEY)",
	)
	if err := store.Migrate(ctx, changed, platform.MigrationApply); err != nil {
		t.Fatalf("historical content correction rejected: %v", err)
	}
	var extra int
	if err := store.db.QueryRow(`SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'synthetic_one' AND column_name = 'extra'`).Scan(&extra); err != nil || extra != 0 {
		t.Fatalf("historical correction re-executed: extra=%d err=%v", extra, err)
	}
}

func TestConcurrentMigrationApplicationsSerialize(t *testing.T) {
	store := newTestStore(t)
	second := newStoreInSchema(t, testDatabaseURL(t), storeTestSchema(t, store))
	migrations := migrationFiles("CREATE TABLE synthetic_once (id BIGINT PRIMARY KEY)")

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, current := range []*Store{store, second} {
		wait.Add(1)
		go func(current *Store) {
			defer wait.Done()
			<-start
			results <- current.Migrate(context.Background(), migrations, platform.MigrationApply)
		}(current)
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent migration failed: %v", err)
		}
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM platform_schema_migrations WHERE version_id > 0").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("applied migration count = %d, want 1", count)
	}
}

func TestMigrationFailureRollsBackDDLAndLedgerRow(t *testing.T) {
	store := newTestStore(t)
	migrations := migrationFiles(
		"CREATE TABLE synthetic_kept (id BIGINT PRIMARY KEY); INSERT INTO synthetic_kept VALUES (1)",
		"CREATE TABLE synthetic_rolled_back (id BIGINT);\nUPDATE synthetic_kept SET id = 2;\nSELECT * FROM synthetic_missing",
	)
	err := store.Migrate(context.Background(), migrations, platform.MigrationApply)
	if platform.FailureCode(err) != platform.CodeStorageUnavailable {
		t.Fatalf("failed migration error = %v, want STORAGE_UNAVAILABLE", err)
	}
	var kept, rolledBack bool
	if err := store.db.QueryRow(`
		SELECT to_regclass('synthetic_kept') IS NOT NULL,
		       to_regclass('synthetic_rolled_back') IS NOT NULL
	`).Scan(&kept, &rolledBack); err != nil {
		t.Fatalf("inspect migration tables: %v", err)
	}
	if !kept || rolledBack {
		t.Fatalf("successful migration kept=%t, failed migration rolled back=%t", kept, rolledBack)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM platform_schema_migrations WHERE version_id > 0").Scan(&count); err != nil {
		t.Fatalf("inspect ledger: %v", err)
	}
	if count != 1 {
		t.Fatalf("ledger count = %d, want only successful version", count)
	}
	var id int
	if err := store.db.QueryRow("SELECT id FROM synthetic_kept").Scan(&id); err != nil || id != 1 {
		t.Fatalf("failed DML did not roll back: id=%d err=%v", id, err)
	}
	repaired := migrationFiles(
		"CREATE TABLE synthetic_kept (id BIGINT PRIMARY KEY); INSERT INTO synthetic_kept VALUES (99)",
		"UPDATE synthetic_kept SET id = 2",
		"UPDATE synthetic_kept SET id = 3",
	)
	if err := store.Migrate(context.Background(), repaired, platform.MigrationApply); err != nil {
		t.Fatalf("apply repaired pending migration and corrective successor: %v", err)
	}
	if err := store.db.QueryRow("SELECT id FROM synthetic_kept").Scan(&id); err != nil || id != 3 {
		t.Fatalf("corrective migration: id=%d err=%v", id, err)
	}
}

func TestMigrationLockWaitHonorsCancellation(t *testing.T) {
	store := newTestStore(t)
	waitingStore := newStoreInSchema(t, testDatabaseURL(t), storeTestSchema(t, store))
	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var ignored any
	if err := conn.QueryRowContext(context.Background(), "SELECT pg_advisory_lock($1)", migrationLockKey).Scan(&ignored); err != nil {
		t.Fatalf("acquire migration lock: %v", err)
	}
	defer func() {
		if err := conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey).Scan(&ignored); err != nil {
			t.Errorf("release migration lock: %v", err)
		}
	}()
	probe, err := waitingStore.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var acquired bool
	if err := probe.QueryRowContext(context.Background(), "SELECT pg_try_advisory_lock($1)", migrationLockKey).Scan(&acquired); err != nil {
		t.Fatal(err)
	}
	if acquired {
		_, _ = probe.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
		t.Fatal("second PostgreSQL session unexpectedly acquired the held migration lock")
	}
	_ = probe.Close()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- waitingStore.Migrate(ctx, nil, platform.MigrationCheck)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if platform.FailureCode(err) != platform.CodeStorageUnavailable {
			t.Fatalf("cancelled lock wait error = %v, want STORAGE_UNAVAILABLE", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled lock wait lost its cancellation cause: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("migration lock wait did not stop after cancellation")
	}
}

func TestSyntheticParticipantsCommitAtomically(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, statement := range []string{
		"CREATE TABLE synthetic_state (operation TEXT PRIMARY KEY)",
		"CREATE TABLE synthetic_outbox (operation TEXT PRIMARY KEY)",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("create synthetic participant: %v", err)
		}
	}

	calls := 0
	for _, failAfterOutbox := range []bool{false, true} {
		err := store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			calls++
			if _, err := executor.ExecContext(tx.Context(), "INSERT INTO synthetic_state(operation) VALUES ($1)", "A"); err != nil {
				return err
			}
			if failAfterOutbox {
				if _, err := executor.ExecContext(tx.Context(), "INSERT INTO synthetic_outbox(operation) VALUES ($1)", "A"); err != nil {
					return err
				}
			}
			return errors.New("synthetic precommit failure")
		})
		if err == nil {
			t.Fatal("injected precommit failure unexpectedly committed")
		}
		assertRowCounts(t, store, 0, 0)
	}

	wakeups := 0
	err := store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		calls++
		if _, err := executor.ExecContext(tx.Context(), "INSERT INTO synthetic_state(operation) VALUES ($1)", "A"); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), "INSERT INTO synthetic_outbox(operation) VALUES ($1)", "A")
		return err
	})
	if err != nil {
		t.Fatalf("commit synthetic participants: %v", err)
	}
	wakeups++
	if calls != 3 || wakeups != 1 {
		t.Fatalf("callback calls=%d, wakeups=%d", calls, wakeups)
	}
	assertRowCounts(t, store, 1, 1)
}

func TestPossibleCommitAcknowledgementLossRequiresReceiptLookup(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.db.Exec("CREATE TABLE synthetic_receipts (operation TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create synthetic receipt table: %v", err)
	}
	calls := 0
	err := store.withinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		calls++
		_, err = executor.ExecContext(tx.Context(), "INSERT INTO synthetic_receipts(operation) VALUES ($1)", "operation-A")
		return err
	}, func(ctx context.Context) (databaseTransaction, error) {
		tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return nil, err
		}
		return lostAcknowledgementTransaction{Tx: tx}, nil
	})
	if platform.FailureCode(err) != platform.CodeCommitUncertain {
		t.Fatalf("lost commit acknowledgement error = %v, want COMMIT_UNCERTAIN", err)
	}
	if calls != 1 {
		t.Fatalf("transaction callback calls = %d, want exactly 1", calls)
	}
	var exists bool
	if err := store.db.QueryRow("SELECT EXISTS (SELECT 1 FROM synthetic_receipts WHERE operation = $1)", "operation-A").Scan(&exists); err != nil {
		t.Fatalf("lookup same-operation receipt: %v", err)
	}
	if !exists {
		t.Fatal("same-operation lookup did not find committed receipt")
	}
}

func TestFullStartupExposesSafeHealthAndShutdown(t *testing.T) {
	store := newTestStore(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve API port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release API port: %v", err)
	}
	config := platform.Config{
		Profile:       "full",
		Roles:         []platform.Role{platform.RoleAPI, platform.RoleWorker},
		MigrationMode: platform.MigrationApply,
		DatabaseURL:   testDatabaseURL(t),
		ListenAddress: address,
	}
	handle, err := platform.Start(context.Background(), config, nil, store, nil)
	if err != nil {
		t.Fatalf("start Full API+worker composition: %v", err)
	}
	status := handle.ReadStatus()
	if !status.Live || !status.Ready || status.Profile != "full" || len(status.Roles) != 2 {
		t.Fatalf("startup status = %+v", status)
	}
	response, err := http.Get("http://" + address + "/health/ready")
	if err != nil {
		t.Fatalf("GET health readiness: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read health response: read=%v close=%v", readErr, closeErr)
	}
	if response.StatusCode != http.StatusOK || string(body) != "{\"ready\":true}\n" {
		t.Fatalf("health response = %d %q", response.StatusCode, body)
	}
	if _, err := store.db.Exec("INSERT INTO platform_schema_migrations(version_id,is_applied) VALUES (999,true)"); err != nil {
		t.Fatal(err)
	}
	if err := handle.Recheck(context.Background()); platform.FailureCode(err) != platform.CodeIncompatibleVersion || handle.ReadStatus().Ready {
		t.Fatalf("running composition accepted incompatible history: %v", err)
	}
	if err := handle.WithinTransaction(context.Background(), func(platform.TransactionContext) error {
		t.Error("not-ready runtime admitted a transaction")
		return nil
	}); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("admission after schema failure: %v", err)
	}
	if _, err := store.db.Exec("DELETE FROM platform_schema_migrations WHERE version_id=999"); err != nil {
		t.Fatal(err)
	}
	if err := handle.Recheck(context.Background()); err != nil || !handle.ReadStatus().Ready {
		t.Fatalf("runtime failed to recover after explicit schema repair: %v", err)
	}
	if err := handle.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown Full composition: %v", err)
	}
}

func TestSyntheticLeaseFencingAndExactMicrosecondBoundary(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.db.Exec(`CREATE TABLE synthetic_boundary_lease (
			id INTEGER PRIMARY KEY, token BIGINT NOT NULL, expires_at TIMESTAMPTZ NOT NULL);
			INSERT INTO synthetic_boundary_lease VALUES (1, 8, '2026-10-26T12:01:00Z')`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name          string
		token         int64
		authorityTime string
		want          int64
	}{
		{"superseded", 7, "2026-10-26T12:00:59.999999Z", 0},
		{"current before expiry", 8, "2026-10-26T12:00:59.999999Z", 1},
		{"exact expiry", 8, "2026-10-26T12:01:00Z", 0},
		{"after expiry", 8, "2026-10-26T12:01:00.000001Z", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Fixed primary-time input characterizes the strict predicate; the other
			// lease test separately verifies actual clock_timestamp() inside a transaction.
			result, err := store.db.Exec(`UPDATE synthetic_boundary_lease SET token=token
					WHERE id=1 AND token=$1 AND $2::timestamptz < expires_at`, test.token, test.authorityTime)
			if err != nil {
				t.Fatal(err)
			}
			if rows, err := result.RowsAffected(); err != nil || rows != test.want {
				t.Fatalf("conditional update=%d err=%v, want %d", rows, err, test.want)
			}
		})
	}
}

func TestPrimaryClockAndLeaseExpiryUseLiveDatabaseTime(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE synthetic_lease (
			id BIGINT PRIMARY KEY,
			token BIGINT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		t.Fatalf("create synthetic lease: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO synthetic_lease(id, token, expires_at)
		VALUES (1, 8, clock_timestamp() + interval '250 milliseconds')
	`); err != nil {
		t.Fatalf("insert synthetic lease: %v", err)
	}

	err := store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		clock := store
		first, err := clock.Now(ctx, tx)
		if err != nil {
			return err
		}
		time.Sleep(350 * time.Millisecond)
		second, err := clock.Now(ctx, tx)
		if err != nil {
			return err
		}
		if !second.After(first) {
			return fmt.Errorf("primary clock did not advance in the transaction")
		}
		var transactionTime, wallTime time.Time
		if err := executor.QueryRowContext(tx.Context(), "SELECT transaction_timestamp(), clock_timestamp()").Scan(&transactionTime, &wallTime); err != nil {
			return err
		}
		if !wallTime.After(transactionTime) {
			return fmt.Errorf("transaction clock did not diverge from wall clock")
		}
		result, err := executor.ExecContext(tx.Context(), `
			UPDATE synthetic_lease SET token = 9
			WHERE id = 1 AND token = 8 AND clock_timestamp() < expires_at
		`)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated != 0 {
			return fmt.Errorf("expired lease was updated")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("primary clock/expiry check: %v", err)
	}
	var token int64
	if err := store.db.QueryRowContext(ctx, "SELECT token FROM synthetic_lease WHERE id = 1").Scan(&token); err != nil {
		t.Fatalf("read lease token: %v", err)
	}
	if token != 8 {
		t.Fatalf("superseded/expired token changed to %d", token)
	}
	if next, err := platform.NextFencingToken(token); err != nil || next != 9 {
		t.Fatalf("fencing increment = %d, %v", next, err)
	}
}

func TestLedgerShapeConflictFailsClosed(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.db.Exec(`
		CREATE TABLE platform_schema_migrations (
			version BIGINT PRIMARY KEY,
			owner TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		t.Fatalf("create incompatible ledger: %v", err)
	}
	err := store.Migrate(context.Background(), nil, platform.MigrationCheck)
	if platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("incompatible ledger error = %v, want INCOMPATIBLE_VERSION", err)
	}
}
