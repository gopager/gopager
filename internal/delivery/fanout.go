package delivery

import (
	"context"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
)

// FanOutConsumerName is the consumer name under which Delivery fan-out keeps its log offset.
const FanOutConsumerName = "delivery-fanout"

// FanOut is the event consumer that turns deliverable facts into Deliveries. Each Delivery is
// inserted in the runner's batch transaction together with the offset; its unique identity
// (Subscription, Domain event, target and split) makes a repeated record a no-op.
type FanOut struct {
	deps  Dependencies
	clock platform.Clock
}

var _ events.Consumer = (*FanOut)(nil)

// NewFanOut returns the fan-out consumer. It fails with authz.ErrInvalidInput on a missing port.
func NewFanOut(deps Dependencies, clock platform.Clock) (*FanOut, error) {
	if err := deps.validate(); err != nil || clock == nil {
		return nil, authz.ErrInvalidInput
	}
	return &FanOut{deps: deps, clock: clock}, nil
}

// Name implements events.Consumer.
func (f *FanOut) Name() string { return FanOutConsumerName }

// Supports implements events.Consumer: deliverable catalog types in schema version 1. Another
// version of a deliverable type blocks the consumer instead of being skipped.
func (f *FanOut) Supports(eventType string, schemaVersion int) events.Support {
	entry, ok := events.LookupType(eventType)
	if !ok || !entry.Deliverable {
		return events.Unsupported
	}
	if schemaVersion != entry.SchemaVersion {
		return events.UnknownVersion
	}
	return events.Supported
}

// Handle implements events.Consumer. A fact of another replay mode than normal, a fact recorded
// more than the fan-out window ago and Subscriptions created after the fact create nothing.
func (f *FanOut) Handle(ctx context.Context, tx platform.TransactionContext, record events.Record) error {
	event, err := parseEvent(record.ID, record.Envelope)
	if err != nil {
		return err
	}
	if event.ReplayMode != events.ReplayNormal || event.Type != record.Type {
		return nil
	}
	now, err := f.clock.Now(ctx, tx)
	if err != nil {
		return err
	}
	if event.RecordedAt.IsZero() || event.RecordedAt.Before(now.Add(-FanOutWindow)) {
		return nil
	}
	subscriptions, err := f.deps.Store.ActiveSubscriptions(ctx, tx, event.Type, event.RecordedAt)
	if err != nil {
		return err
	}
	for _, subscription := range subscriptions {
		records, err := materialize(ctx, tx, f.deps, now, subscription, event)
		if err != nil {
			return err
		}
		for _, delivery := range records {
			if _, err := f.deps.Store.InsertDelivery(ctx, tx, delivery); err != nil {
				return err
			}
		}
	}
	return nil
}
