package ingeststore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
	"github.com/gopager/gopager/internal/units"
)

func TestMain(m *testing.M) {
	os.Exit(testpostgres.Run(m))
}

var migratedTemplate = testpostgres.NewTemplate("ingeststore", func(ctx context.Context, dsn string) error {
	store, _, err := postgres.NewStore(dsn, true)
	if err != nil {
		return err
	}
	migrateErr := store.Migrate(ctx, migrations.Files, platform.MigrationApply)
	if err := store.Close(); err != nil && migrateErr == nil {
		return err
	}
	return migrateErr
})

type stateFixture struct {
	primary *postgres.Store
	store   *ingeststore.Store
	network uuid.UUID
}

func newStateFixture(t *testing.T) stateFixture {
	t.Helper()
	primary, _, err := postgres.NewStore(migratedTemplate.Database(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := primary.Close(); err != nil {
			t.Error(err)
		}
	})
	facts, err := eventstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	store, err := ingeststore.New(primary, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	fixture := stateFixture{primary: primary, store: store, network: uuid.New()}
	fixture.exec(t, `INSERT INTO agencies(id,name) VALUES($1,'Agency')`, uuid.New())
	fixture.exec(t, `INSERT INTO networks(id,name,protocol,agency_id) VALUES($1,'North','pocsag',(SELECT id FROM agencies))`,
		fixture.network)
	return fixture
}

func (f stateFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if err := f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.primary.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), query, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// gateway inserts a Gateway with the given state, enablement and last contact (nil: none).
func (f stateFixture) gateway(t *testing.T, state string, enabled bool, contact *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO gateways(id,name,state,enabled,last_contact_at) VALUES($1,'G',$2,$3,$4)`,
		id, state, enabled, contact)
	return id
}

type gatewayRecord struct {
	state    string
	revision int64
	history  []int64
	facts    []int64
}

func (f stateFixture) read(t *testing.T, gateway uuid.UUID) gatewayRecord {
	t.Helper()
	var record gatewayRecord
	if err := f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.primary.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT state,revision FROM gateways WHERE id=$1`, gateway).
			Scan(&record.state, &record.revision); err != nil {
			return err
		}
		for query, target := range map[string]*[]int64{
			`SELECT revision FROM gateway_state_history WHERE gateway_id=$1 ORDER BY revision`: &record.history,
			`SELECT (envelope->>'aggregateRevision')::bigint FROM domain_events
				WHERE type='gateway.state-changed' AND envelope->'aggregateRef'->>'id'=$1::text ORDER BY 1`: &record.facts,
		} {
			rows, err := executor.QueryContext(tx.Context(), query, gateway)
			if err != nil {
				return err
			}
			for rows.Next() {
				var revision int64
				if err := rows.Scan(&revision); err != nil {
					_ = rows.Close()
					return err
				}
				*target = append(*target, revision)
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return record
}

func (f stateFixture) report(ctx context.Context, gateway uuid.UUID, operation string, now time.Time) error {
	return f.primary.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		return f.store.ReportGatewayState(tx.Context(), tx, gateway, ingest.GatewayStateReport{SchemaVersion: 1,
			Operation: operation, ReasonCode: map[bool]string{true: "LOW_SIGNAL"}[operation == "degraded"]}, now)
	})
}

func (f stateFixture) scan(ctx context.Context, now time.Time, timeout time.Duration) (int, error) {
	var changed int
	err := f.primary.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		changed, err = f.store.MarkContactLost(tx.Context(), tx, now, now.Add(-timeout), 100)
		return err
	})
	return changed, err
}

func pointer(value time.Time) *time.Time { return &value }

// TestContactTimeoutBoundaryAndPrecedence covers F-90 and F-92: the inclusive boundary, the
// degraded-to-contact-lost precedence, and the Gateways that are never evaluated.
func TestContactTimeoutBoundaryAndPrecedence(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	timeout := 180 * time.Second
	atBoundary := f.gateway(t, ingest.StateActive, true, pointer(now.Add(-timeout)))
	inside := f.gateway(t, ingest.StateActive, true, pointer(now.Add(-timeout+time.Microsecond)))
	degraded := f.gateway(t, ingest.StateDegraded, true, pointer(now.Add(-time.Hour)))
	stopped := f.gateway(t, ingest.StateStopped, true, pointer(now.Add(-time.Hour)))
	disabled := f.gateway(t, ingest.StateActive, false, pointer(now.Add(-time.Hour)))
	neverSeen := f.gateway(t, ingest.StateNeverSeen, true, nil)
	deleted := f.gateway(t, ingest.StateActive, true, pointer(now.Add(-time.Hour)))
	f.exec(t, `UPDATE gateways SET deleted=true WHERE id=$1`, deleted)

	changed, err := f.scan(t.Context(), now, timeout)
	if err != nil || changed != 2 {
		t.Fatalf("scan changed %d, %v; want 2", changed, err)
	}
	for gateway, want := range map[uuid.UUID]string{atBoundary: ingest.StateContactLost, inside: ingest.StateActive,
		degraded: ingest.StateContactLost, stopped: ingest.StateStopped, disabled: ingest.StateActive,
		neverSeen: ingest.StateNeverSeen, deleted: ingest.StateActive} {
		record := f.read(t, gateway)
		if record.state != want {
			t.Errorf("gateway state %s, want %s", record.state, want)
		}
		if want == ingest.StateContactLost && (record.revision != 2 || !slices.Equal(record.history, []int64{2}) ||
			!slices.Equal(record.facts, []int64{2})) {
			t.Errorf("transition record %+v, want revision 2 with one history row and fact", record)
		}
	}
	var contactAt, contactTime time.Time
	var cause, oldState string
	if err := f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.primary.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT contact_at,cause,old_state FROM gateway_state_history
			WHERE gateway_id=$1`, degraded).Scan(&contactAt, &cause, &oldState); err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT (envelope->'data'->>'contactTime')::timestamptz
			FROM domain_events WHERE type='gateway.state-changed' AND envelope->'aggregateRef'->>'id'=$1::text`,
			degraded).Scan(&contactTime)
	}); err != nil {
		t.Fatal(err)
	}
	if !contactAt.Equal(now.Add(-time.Hour)) || !contactTime.Equal(contactAt) || cause != ingest.CauseTimeout ||
		oldState != ingest.StateDegraded {
		t.Fatalf("timeout transition contact=%s fact=%s cause=%s old=%s", contactAt, contactTime, cause, oldState)
	}
	if changed, err := f.scan(t.Context(), now, timeout); err != nil || changed != 0 {
		t.Fatalf("repeated scan changed %d, %v", changed, err)
	}
}

// TestFirstContactAndRecovery covers F-90 and F-91: the first contact leaves never-seen with
// one transition, and authenticated contact recovers contact-lost and stopped Gateways.
func TestFirstContactAndRecovery(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	gateway := f.gateway(t, ingest.StateNeverSeen, true, nil)
	steps := []struct {
		operation string
		want      string
		history   []int64
	}{
		{"heartbeat", ingest.StateActive, []int64{2}},
		{"heartbeat", ingest.StateActive, []int64{2}},
		{"terminated", ingest.StateStopped, []int64{2, 3}},
		{"heartbeat", ingest.StateActive, []int64{2, 3, 4}},
	}
	for index, step := range steps {
		if err := f.report(t.Context(), gateway, step.operation, now.Add(time.Duration(index)*time.Second)); err != nil {
			t.Fatal(err)
		}
		record := f.read(t, gateway)
		if record.state != step.want || !slices.Equal(record.history, step.history) || !slices.Equal(record.facts, step.history) {
			t.Fatalf("step %d %s: %+v", index, step.operation, record)
		}
	}
	if changed, err := f.scan(t.Context(), now.Add(time.Hour), 180*time.Second); err != nil || changed != 1 {
		t.Fatalf("timeout changed %d, %v", changed, err)
	}
	if err := f.report(t.Context(), gateway, "heartbeat", now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	record := f.read(t, gateway)
	if record.state != ingest.StateActive || !slices.Equal(record.history, []int64{2, 3, 4, 5, 6}) ||
		!slices.Equal(record.facts, record.history) {
		t.Fatalf("recovery after contact loss: %+v", record)
	}
}

// TestConcurrentContactAndTimeout covers F-93 in both lock orders.
func TestConcurrentContactAndTimeout(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	timeout := 180 * time.Second

	t.Run("contact holds the row", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		gateway := f.gateway(t, ingest.StateActive, true, pointer(now.Add(-time.Hour)))
		locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				if err := f.store.ReportGatewayState(tx.Context(), tx, gateway,
					ingest.GatewayStateReport{SchemaVersion: 1, Operation: "heartbeat"}, now); err != nil {
					return err
				}
				close(locked)
				<-release
				return nil
			})
		}()
		<-locked
		if changed, err := f.scan(t.Context(), now, timeout); err != nil || changed != 0 {
			t.Fatalf("scan beside a held contact changed %d, %v", changed, err)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if changed, err := f.scan(t.Context(), now, timeout); err != nil || changed != 0 {
			t.Fatalf("scan after the committed contact changed %d, %v", changed, err)
		}
		if record := f.read(t, gateway); record.state != ingest.StateActive || len(record.history) != 0 {
			t.Fatalf("contact first: %+v", record)
		}
	})

	t.Run("timeout holds the row", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		gateway := f.gateway(t, ingest.StateActive, true, pointer(now.Add(-time.Hour)))
		locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				changed, err := f.store.MarkContactLost(tx.Context(), tx, now, now.Add(-timeout), 100)
				if err != nil {
					return err
				}
				if changed != 1 {
					return errors.New("timeout did not change the Gateway")
				}
				close(locked)
				<-release
				return nil
			})
		}()
		<-locked
		reported := make(chan error, 1)
		go func() { reported <- f.report(t.Context(), gateway, "heartbeat", now) }()
		select {
		case err := <-reported:
			t.Fatalf("report did not wait for the timeout transaction: %v", err)
		case <-time.After(200 * time.Millisecond):
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := <-reported; err != nil {
			t.Fatal(err)
		}
		record := f.read(t, gateway)
		if record.state != ingest.StateActive || !slices.Equal(record.history, []int64{2, 3}) ||
			!slices.Equal(record.facts, []int64{2, 3}) {
			t.Fatalf("timeout first, then recovery: %+v", record)
		}
	})
}

// TestDuplicateScansAndRestart covers F-94: concurrent scans change an expired Gateway once,
// and a new monitor after a restart finds nothing to do.
func TestDuplicateScansAndRestart(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	gateway := f.gateway(t, ingest.StateActive, true, pointer(now.Add(-time.Hour)))
	var wait sync.WaitGroup
	results := make(chan int, 4)
	for range 4 {
		wait.Go(func() {
			changed, err := f.scan(t.Context(), now, 180*time.Second)
			if err != nil {
				t.Error(err)
			}
			results <- changed
		})
	}
	wait.Wait()
	close(results)
	total := 0
	for changed := range results {
		total += changed
	}
	if total != 1 {
		t.Fatalf("concurrent scans changed %d Gateways, want 1", total)
	}
	monitor, err := ingest.NewContactMonitor(f.primary, f.primary, f.store, ingest.ContactConfig{
		HeartbeatInterval: time.Second, Timeout: time.Second, ScanInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := monitor.RunOnce(t.Context()); err != nil || changed != 0 {
		t.Fatalf("restarted monitor changed %d, %v", changed, err)
	}
	if record := f.read(t, gateway); !slices.Equal(record.history, []int64{2}) || !slices.Equal(record.facts, []int64{2}) {
		t.Fatalf("duplicate scans: %+v", record)
	}
}

// TestTransitionRollback covers F-95 and F-96: a failing history insert rolls back the state,
// revision and fact, and a later attempt records exactly one transition.
func TestTransitionRollback(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	gateway := f.gateway(t, ingest.StateActive, true, pointer(now.Add(-time.Hour)))
	// A conflicting row for the next revision makes the history insert fail inside the scan.
	f.exec(t, `INSERT INTO gateway_state_history(id,gateway_id,revision,old_state,new_state,cause,occurred_at,contact_at)
		VALUES($1,$2,2,'active','degraded','report',$3,$3)`, uuid.New(), gateway, now)
	if _, err := f.scan(t.Context(), now, 180*time.Second); err == nil {
		t.Fatal("scan with a conflicting history row succeeded")
	}
	if err := f.report(t.Context(), gateway, "degraded", now); err == nil {
		t.Fatal("report with a conflicting history row succeeded")
	}
	record := f.read(t, gateway)
	if record.state != ingest.StateActive || record.revision != 1 || len(record.facts) != 0 {
		t.Fatalf("failed transitions left %+v", record)
	}
	f.exec(t, `DELETE FROM gateway_state_history WHERE gateway_id=$1`, gateway)
	if changed, err := f.scan(t.Context(), now, 180*time.Second); err != nil || changed != 1 {
		t.Fatalf("retry changed %d, %v", changed, err)
	}
	if err := f.report(t.Context(), gateway, "degraded", now); err != nil {
		t.Fatal(err)
	}
	record = f.read(t, gateway)
	if record.state != ingest.StateDegraded || record.revision != 3 || !slices.Equal(record.history, []int64{2, 3}) ||
		!slices.Equal(record.facts, record.history) {
		t.Fatalf("after retry: %+v", record)
	}
}

// TestContactMonitorUsesDatabaseClock runs the monitor against the advancing database clock:
// a Gateway whose last contact is older than the timeout becomes contact-lost, and the
// monitor stops on cancellation.
func TestContactMonitorUsesDatabaseClock(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	gateway := f.gateway(t, ingest.StateNeverSeen, true, nil)
	if err := f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		now, err := f.primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return f.store.ReportGatewayState(tx.Context(), tx, gateway,
			ingest.GatewayStateReport{SchemaVersion: 1, Operation: "heartbeat"}, now)
	}); err != nil {
		t.Fatal(err)
	}
	monitor, err := ingest.NewContactMonitor(f.primary, f.primary, f.store, ingest.ContactConfig{
		HeartbeatInterval: 500 * time.Millisecond, Timeout: time.Second, ScanInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := monitor.RunOnce(t.Context()); err != nil || changed != 0 {
		t.Fatalf("fresh contact changed %d, %v", changed, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- monitor.Run(ctx) }()
	deadline := time.Now().Add(10 * time.Second)
	for f.read(t, gateway).state != ingest.StateContactLost {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("monitor did not detect contact loss")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if err := <-stopped; !errors.Is(err, context.Canceled) {
		t.Fatalf("monitor stopped with %v", err)
	}
	if record := f.read(t, gateway); !slices.Equal(record.history, []int64{2, 3}) || !slices.Equal(record.facts, record.history) {
		t.Fatalf("database-clock run: %+v", record)
	}
}

// TestListsFailClosedForAnEmptyReach expects an empty Agency reach (for example a key whose
// scopes do not overlap the owner's grants) to list no Gateway and no Network, while a reach
// covering the Agency lists both.
func TestListsFailClosedForAnEmptyReach(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	gateway := f.gateway(t, ingest.StateActive, true, nil)
	f.exec(t, `INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
		VALUES($1,$2,'north',$3,0)`, uuid.New(), gateway, f.network)
	var agency uuid.UUID
	page := units.PageInput{Limit: 50}
	for _, reach := range []authz.AgencyReach{{}, {Agencies: []uuid.UUID{}}, {Agencies: []uuid.UUID{uuid.New()}}} {
		if err := f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			gateways, err := f.store.ListGateways(tx.Context(), tx, reach, page)
			if err != nil {
				return err
			}
			networks, err := f.store.ListNetworks(tx.Context(), tx, reach, page)
			if err != nil {
				return err
			}
			if gateways.Total != 0 || len(gateways.Items) != 0 || networks.Total != 0 || len(networks.Items) != 0 {
				return fmt.Errorf("reach %+v listed %d Gateways and %d Networks", reach, gateways.Total, networks.Total)
			}
			network, err := f.store.ReadNetworkRecord(tx.Context(), tx, f.network)
			agency = network.AgencyID
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		gateways, err := f.store.ListGateways(tx.Context(), tx, authz.AgencyReach{Agencies: []uuid.UUID{agency}}, page)
		if err != nil || gateways.Total != 1 {
			return errors.Join(err, fmt.Errorf("covering reach listed %d Gateways", gateways.Total))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
