package main

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
)

// wakeFixture counts selections; it plays transactions, repository and executor for the dispatcher.
type wakeFixture struct{ selections atomic.Int32 }

type wakeTransaction struct{ ctx context.Context }

func (tx wakeTransaction) Context() context.Context { return tx.ctx }

func (f *wakeFixture) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	return callback(wakeTransaction{ctx: ctx})
}

func (f *wakeFixture) SelectDueWork(context.Context, platform.TransactionContext, int) ([]processing.Candidate, error) {
	f.selections.Add(1)
	return nil, nil
}

func (f *wakeFixture) SupersedeCandidate(context.Context, platform.TransactionContext, uuid.UUID, int64) (processing.Candidate, error) {
	return processing.Candidate{}, nil
}

func (f *wakeFixture) ExecuteWork(context.Context, processing.Candidate) error { return nil }

// channelListener hands the test control over the notification stream.
type channelListener struct {
	signals chan struct{}
	channel chan string
}

func (l channelListener) Listen(ctx context.Context, channel string) (<-chan struct{}, error) {
	l.channel <- channel
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-l.signals:
				out <- struct{}{}
			}
		}
	}()
	return out, nil
}

func TestNotificationWakesDispatcherBeforePollInterval(t *testing.T) {
	fixture := &wakeFixture{}
	dispatcher, err := worker.NewDispatcher(fixture, fixture, fixture,
		slog.New(slog.NewTextHandler(io.Discard, nil)), worker.DispatchConfig{Concurrency: 1, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	listener := channelListener{signals: make(chan struct{}), channel: make(chan string, 1)}
	runner := notifiedDispatcher{dispatcher: dispatcher, listener: listener}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	if channel := <-listener.channel; channel != processing.DueNotificationChannel {
		t.Fatalf("listened on %q", channel)
	}
	waitForSelections(t, fixture, 1)
	listener.signals <- struct{}{}
	waitForSelections(t, fixture, 2)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not stop with its context")
	}
}

// waitForSelections fails unless the dispatcher has selected at least want times soon.
func waitForSelections(t *testing.T, fixture *wakeFixture, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for fixture.selections.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("selections=%d want %d well before the 1 h poll interval", fixture.selections.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
