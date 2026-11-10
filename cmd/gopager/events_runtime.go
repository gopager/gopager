package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
)

// eventsRuntime holds the process-wide event wake-up hub and the consumer runners. The hub
// is shared by every runner and, in the api role, by live streams.
type eventsRuntime struct {
	// hub fans the database wake-ups for new facts out to all subscribers.
	hub *events.WakeHub
	// runners are drained by the platform, which accepts runners only with the worker role: the
	// hub, one runner per registered consumer and the Delivery runner. They are empty without
	// that role.
	runners []platform.Runner
}

// registeredEventConsumers returns the durable event consumers the worker role runs: the
// Delivery fan-out.
func registeredEventConsumers(c *core) ([]events.Consumer, error) {
	runtime, err := composeDelivery(c)
	if err != nil {
		return nil, err
	}
	return []events.Consumer{runtime.fanOut}, nil
}

// composeEventsRuntime builds the wake hub on the domain-event channel and, when withConsumers
// is set (worker role), one runner per registered consumer polling at pollInterval.
func composeEventsRuntime(c *core, withConsumers bool, pollInterval time.Duration) (*eventsRuntime, error) {
	hub, err := events.NewWakeHub(c.store, events.NotifyChannel)
	if err != nil {
		return nil, err
	}
	runtime := &eventsRuntime{hub: hub}
	if !withConsumers {
		return runtime, nil
	}
	runtime.runners = append(runtime.runners, hub)
	consumers, err := registeredEventConsumers(c)
	if err != nil {
		return nil, err
	}
	for _, consumer := range consumers {
		runner, err := events.NewRunner(consumer, c.facts, c.store, hub, slog.Default(), pollInterval)
		if err != nil {
			return nil, err
		}
		runtime.runners = append(runtime.runners, runner)
	}
	deliveries, err := composeDeliveryRunner(c, hub)
	if err != nil {
		return nil, err
	}
	runtime.runners = append(runtime.runners, deliveries)
	return runtime, nil
}

// startStandaloneHub runs the hub in its own goroutine when the platform does not drain it,
// that is in a process without the worker role, and returns the function that stops it and
// waits for it to end. With the worker role it does nothing.
func (r *eventsRuntime) startStandaloneHub(ctx context.Context) (stop func() error) {
	if len(r.runners) > 0 {
		return func() error { return nil }
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- r.hub.Run(ctx) }()
	return func() error {
		cancel()
		return <-done
	}
}
