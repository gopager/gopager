package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/events"
)

// fakeSource is a WakeSource whose signals the test controls.
type fakeSource struct {
	signals chan struct{}
	channel string
}

func newFakeSource() *fakeSource { return &fakeSource{signals: make(chan struct{}, 1)} }

func (s *fakeSource) Listen(ctx context.Context, channel string) (<-chan struct{}, error) {
	s.channel = channel
	out := make(chan struct{})
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.signals:
				select {
				case out <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// expectSignal fails unless a signal arrives soon.
func expectSignal(t *testing.T, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(2 * time.Second):
		t.Fatal("no signal")
	}
}

// expectNoSignal fails if a signal is pending.
func expectNoSignal(t *testing.T, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
		t.Fatal("unexpected signal")
	case <-time.After(50 * time.Millisecond):
	}
}

func startHub(t *testing.T, source *fakeSource) *events.WakeHub {
	t.Helper()
	hub, err := events.NewWakeHub(source, events.NotifyChannel)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- hub.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return hub
}

func TestWakeHubRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	if _, err := events.NewWakeHub(nil, events.NotifyChannel); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := events.NewWakeHub(newFakeSource(), ""); err == nil {
		t.Fatal("empty channel accepted")
	}
}

func TestWakeHubSubscriberIsWokenOnceAtSubscription(t *testing.T) {
	t.Parallel()
	hub := startHub(t, newFakeSource())
	signals, release := hub.Subscribe()
	defer release()
	expectSignal(t, signals)
	expectNoSignal(t, signals)
}

func TestWakeHubFansOutAndCoalesces(t *testing.T) {
	t.Parallel()
	source := newFakeSource()
	hub := startHub(t, source)
	first, releaseFirst := hub.Subscribe()
	defer releaseFirst()
	second, releaseSecond := hub.Subscribe()
	defer releaseSecond()
	expectSignal(t, first)
	expectSignal(t, second)

	source.signals <- struct{}{}
	expectSignal(t, first)
	expectSignal(t, second)

	// Several signals while a subscriber is busy collapse into one. The second subscriber
	// acknowledges each broadcast, and releasing it takes the hub's lock, so it returns only
	// after the last broadcast has also reached the first subscriber.
	for range 3 {
		source.signals <- struct{}{}
		expectSignal(t, second)
	}
	releaseSecond()
	expectSignal(t, first)
	expectNoSignal(t, first)
}

func TestWakeHubUnsubscribeReleases(t *testing.T) {
	t.Parallel()
	source := newFakeSource()
	hub := startHub(t, source)
	released, release := hub.Subscribe()
	kept, releaseKept := hub.Subscribe()
	defer releaseKept()
	expectSignal(t, released)
	expectSignal(t, kept)
	release()
	release()

	source.signals <- struct{}{}
	expectSignal(t, kept)
	expectNoSignal(t, released)
}

func TestWakeHubSlowSubscriberDoesNotBlockOthers(t *testing.T) {
	t.Parallel()
	source := newFakeSource()
	hub := startHub(t, source)
	_, releaseSlow := hub.Subscribe()
	defer releaseSlow()
	fast, releaseFast := hub.Subscribe()
	defer releaseFast()
	expectSignal(t, fast)
	for range 5 {
		source.signals <- struct{}{}
		expectSignal(t, fast)
	}
}
