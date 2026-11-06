package postgres

import (
	"context"
	"testing"
	"time"
)

// receive waits for one signal or fails the test.
func receive(t *testing.T, signals <-chan struct{}, within time.Duration, what string) {
	t.Helper()
	select {
	case _, ok := <-signals:
		if !ok {
			t.Fatalf("listener closed while waiting for %s", what)
		}
	case <-time.After(within):
		t.Fatalf("no signal for %s within %s", what, within)
	}
}

func TestListenDeliversNotificationsAndSurvivesConnectionLoss(t *testing.T) {
	store := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	signals, err := store.Listen(ctx, "gopager_listen_test")
	if err != nil {
		t.Fatal(err)
	}
	// A signal follows every (re)connect because notifications sent while disconnected are lost.
	receive(t, signals, 10*time.Second, "initial connect")
	if _, err := store.db.ExecContext(ctx, `SELECT pg_notify('gopager_listen_test', '')`); err != nil {
		t.Fatal(err)
	}
	receive(t, signals, 5*time.Second, "notification")
	if _, err := store.db.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE application_name = $1`, listenApplicationName); err != nil {
		t.Fatal(err)
	}
	receive(t, signals, 15*time.Second, "reconnect")
	if _, err := store.db.ExecContext(ctx, `SELECT pg_notify('gopager_listen_test', '')`); err != nil {
		t.Fatal(err)
	}
	receive(t, signals, 5*time.Second, "notification after reconnect")
	cancel()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-signals:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("listener did not end with its context")
		}
	}
}

func TestListenRejectsEmptyChannel(t *testing.T) {
	store := &Store{}
	if _, err := store.Listen(t.Context(), ""); err == nil {
		t.Fatal("empty channel accepted")
	}
}
