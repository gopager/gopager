package main

import (
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

// startCommand runs the actual command in a child process until it logs its startup and
// returns a function that interrupts it and requires a clean exit.
func startCommand(t *testing.T, roles, dsn, address, extra string) func() {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestRunProcessHelper$")
	command.Env = append(os.Environ(), "GOPAGER_TEST_PROCESS_HELPER="+roles, "GOPAGER_DATABASE_URL="+dsn,
		"GOPAGER_TEST_LISTEN="+address, "GOPAGER_TEST_EXTRA_ARGS="+extra)
	output := startupOutput{ready: make(chan struct{})}
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	select {
	case <-output.ready:
	case err := <-exited:
		t.Fatalf("%s command exited during startup: %v output=%s", roles, err, output.buffer.String())
	case <-time.After(platform.DefaultCallTimeout + time.Second):
		_ = command.Process.Kill()
		t.Fatalf("%s command did not start: %s", roles, output.buffer.String())
	}
	return func() {
		t.Helper()
		if err := command.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-exited:
			if err != nil {
				t.Fatalf("%s command exit %v output=%s", roles, err, output.buffer.String())
			}
		case <-time.After(5 * time.Second):
			_ = command.Process.Kill()
			t.Fatalf("%s command did not drain after the signal", roles)
		}
	}
}

// TestContactMonitorRunsOnlyInWorkerRole starts the real command: the api role leaves an
// expired Gateway untouched, the worker role marks it contact-lost through its registered
// monitor against the database clock, and both stop cleanly on a signal.
func TestContactMonitorRunsOnlyInWorkerRole(t *testing.T) {
	t.Parallel()
	if os.Getenv("GOPAGER_TEST_DATABASE_URL") == "" {
		t.Skip("GOPAGER_TEST_DATABASE_URL is not set; command/PostgreSQL process evidence not run")
	}
	dsn := testpostgres.EmptyDatabase(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	contact := "--gateway-heartbeat-interval=100ms --gateway-contact-timeout=1s --gateway-contact-scan-interval=100ms"
	stopAPI := startCommand(t, "api", dsn, address, contact)

	store, _, err := postgres.NewStore(dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	gateway := uuid.New()
	state := func() string {
		t.Helper()
		var value string
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT state FROM gateways WHERE id=$1`, gateway).Scan(&value)
		}); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,name,state,last_contact_at)
			VALUES($1,'Expired','active',now() - interval '1 hour')`, gateway)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if got := state(); got != "active" {
		t.Fatalf("api role changed the Gateway to %s", got)
	}
	stopAPI()

	stopWorker := startCommand(t, "worker", dsn, "", contact)
	deadline := time.Now().Add(10 * time.Second)
	for state() != "contact-lost" {
		if time.Now().After(deadline) {
			stopWorker()
			t.Fatal("worker role did not mark the expired Gateway contact-lost")
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopWorker()
}
