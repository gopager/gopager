package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/deliverystore"
	"github.com/gopager/gopager/internal/persistence/ent/layoutstore"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// deliveryConfig carries the Delivery settings of the process.
type deliveryConfig struct {
	// limits are the sending admission limits; zero values select the defaults.
	limits delivery.AdmissionLimits
	// targets is the Delivery target port; nil selects delivery.UnavailableTargets, so that
	// without a target provider every target is visibly unavailable.
	targets delivery.Targets
	// pollInterval is the Delivery runner's fallback poll; zero selects the default.
	pollInterval time.Duration
}

// deliveryRuntime is the composed Delivery side shared by the routes, the fan-out consumer and
// the runner.
type deliveryRuntime struct {
	deps          delivery.Dependencies
	subscriptions *delivery.Subscriptions
	deliveries    *delivery.Deliveries
	layouts       *presentation.Service
	fanOut        *delivery.FanOut
}

// composeDelivery returns the process's Delivery side, composing it on first use. It is called
// during startup composition only, before any runner or route runs.
func composeDelivery(c *core) (*deliveryRuntime, error) {
	if c.delivery != nil {
		return c.delivery, nil
	}
	side, err := composeReadSide(c)
	if err != nil {
		return nil, err
	}
	store, err := deliverystore.New(c.store)
	if err != nil {
		return nil, err
	}
	layouts, err := layoutstore.New(c.store)
	if err != nil {
		return nil, err
	}
	resolver, err := presentation.NewResolver(layouts, delivery.DefaultLayouts())
	if err != nil {
		return nil, err
	}
	targets := c.deliveryConfig.targets
	if targets == nil {
		targets = delivery.UnavailableTargets{}
	}
	deps := delivery.Dependencies{Store: store, Instances: c.processing, Targets: targets, Layouts: resolver,
		Projector: side.service, Fields: c.fields, Facts: c.facts, Authority: c.resolver}
	runtime := &deliveryRuntime{deps: deps}
	if runtime.subscriptions, err = delivery.NewSubscriptions(deps, c.store, c.resolver, c.store, c.receipts,
		c.identity, c.deliveryConfig.limits); err != nil {
		return nil, err
	}
	if runtime.deliveries, err = delivery.NewDeliveries(deps, c.store, c.resolver, c.store, c.receipts,
		c.deliveryConfig.limits); err != nil {
		return nil, err
	}
	if runtime.layouts, err = presentation.NewService(c.store, c.resolver, c.store, layouts, c.receipts, c.fields); err != nil {
		return nil, err
	}
	if runtime.fanOut, err = delivery.NewFanOut(deps, c.store); err != nil {
		return nil, err
	}
	c.delivery = runtime
	return runtime, nil
}

// composeDeliveryRunner returns the worker role's Delivery runner, woken by new facts.
func composeDeliveryRunner(c *core, wakes events.WakeSubscriber) (*delivery.Runner, error) {
	runtime, err := composeDelivery(c)
	if err != nil {
		return nil, err
	}
	return delivery.NewRunner(runtime.deps, c.store, c.store, delivery.RunnerOptions{Limits: c.deliveryConfig.limits,
		PollInterval: c.deliveryConfig.pollInterval, Wakes: wakes, Logger: slog.Default()})
}

// registerDeliveryRoutes binds the Subscription, Delivery and layout routes.
func registerDeliveryRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator httpapi.Authenticator, c *core) error {
	runtime, err := composeDelivery(c)
	if err != nil {
		return err
	}
	httpapi.RegisterSubscriptionRoutes(mux, transport, authenticator, runtime.subscriptions)
	httpapi.RegisterDeliveryRoutes(mux, transport, authenticator, runtime.deliveries)
	httpapi.RegisterLayoutRoutes(mux, transport, authenticator, runtime.layouts)
	return nil
}
