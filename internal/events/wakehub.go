package events

import (
	"context"
	"errors"
	"sync"

	"github.com/gopager/gopager/internal/platform"
)

// WakeSource delivers coalesced wake-up signals for a notification channel. The returned
// channel receives a signal after every (re)connect and for each burst of notifications, and
// is closed when ctx ends. *postgres.Store implements it.
type WakeSource interface {
	Listen(ctx context.Context, channel string) (<-chan struct{}, error)
}

// WakeSubscriber hands out wake-up subscriptions. *WakeHub implements it.
type WakeSubscriber interface {
	// Subscribe returns a channel that carries a signal when work may be available and a
	// function that releases the subscription. The channel already holds one signal, so the
	// subscriber checks for work once right away.
	Subscribe() (<-chan struct{}, func())
}

// WakeHub fans one wake-up signal stream out to any number of subscribers, so a process
// needs only one database listener connection however many consumers and live streams it
// runs. A signal is only a hint: a lost or coalesced signal delays work, never loses it.
type WakeHub struct {
	source  WakeSource
	channel string

	mu          sync.Mutex
	subscribers map[*wakeSubscription]struct{}
}

// wakeSubscription is one subscriber's size-1 coalescing signal channel.
type wakeSubscription struct {
	signals chan struct{}
}

// notify delivers a signal unless one is already pending.
func (s *wakeSubscription) notify() {
	select {
	case s.signals <- struct{}{}:
	default:
	}
}

// NewWakeHub returns a hub that listens on channel through source once Run is called. It
// fails with an invalid-input failure for a nil source or an empty channel.
func NewWakeHub(source WakeSource, channel string) (*WakeHub, error) {
	if source == nil || channel == "" {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid wake hub"))
	}
	return &WakeHub{source: source, channel: channel, subscribers: map[*wakeSubscription]struct{}{}}, nil
}

// Subscribe registers a subscriber. The returned channel is buffered with size 1 and
// coalesces signals; it is pre-filled with one signal and is never closed. The returned
// function releases the subscription and is safe to call more than once.
func (h *WakeHub) Subscribe() (<-chan struct{}, func()) {
	subscription := &wakeSubscription{signals: make(chan struct{}, 1)}
	subscription.notify()
	h.mu.Lock()
	h.subscribers[subscription] = struct{}{}
	h.mu.Unlock()
	return subscription.signals, func() {
		h.mu.Lock()
		delete(h.subscribers, subscription)
		h.mu.Unlock()
	}
}

// broadcast signals every current subscriber without blocking.
func (h *WakeHub) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscription := range h.subscribers {
		subscription.notify()
	}
}

// Run listens on the source and fans each signal out to all subscribers until ctx ends. It
// returns the error of the listen request, or nil after a clean stop. It implements
// platform.Runner.
func (h *WakeHub) Run(ctx context.Context) error {
	signals, err := h.source.Listen(ctx, h.channel)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-signals:
			if !ok {
				return nil
			}
			h.broadcast()
		}
	}
}
