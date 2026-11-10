package delivery

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// Delivery actions.
const (
	ReadAction      = "delivery.read"
	RetryAction     = "delivery.retry"
	RedeliverAction = "delivery.redeliver"
)

// DeliveryFilter narrows a Delivery list.
type DeliveryFilter struct {
	SubscriptionID *uuid.UUID
	State          string
}

// Deliveries reads Delivery results and runs the explicit retry and redelivery commands under
// the caller's current authority. Create it with NewDeliveries; it is safe for concurrent use.
type Deliveries struct {
	deps         Dependencies
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
	receipts     commands.Repository
	admission    admission
}

// NewDeliveries composes the Delivery management service. It fails with authz.ErrInvalidInput
// on a missing dependency.
func NewDeliveries(deps Dependencies, transactions identity.Transactions, resolver identity.AuthorityResolver,
	clock platform.Clock, receipts commands.Repository, limits AdmissionLimits) (*Deliveries, error) {
	if err := deps.validate(); err != nil || transactions == nil || resolver == nil || clock == nil || receipts == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Deliveries{deps: deps, transactions: transactions, resolver: resolver, clock: clock, receipts: receipts,
		admission: admission{store: deps.Store, limits: limits.normalized()}}, nil
}

// within resolves the caller and runs operation in the same transaction; mutations exclude the
// guest.
func (d *Deliveries) within(ctx context.Context, proof identity.Proof, mutation bool, check func(identity.Authentication) error,
	operation func(tx platform.TransactionContext, authority authz.Authority, principal uuid.UUID, now time.Time) error) error {
	var authentication identity.Authentication
	remember := func(value identity.Authentication) error {
		authentication = value
		if check != nil {
			return check(value)
		}
		return nil
	}
	return identity.WithAuthority(ctx, d.transactions, d.clock, d.resolver, proof, mutation, remember,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			principal, err := uuid.Parse(authority.PrincipalID)
			if err != nil || principal == uuid.Nil {
				return authz.ErrForbidden
			}
			if mutation && authentication.Kind == identity.AnonymousProof {
				return identity.ErrLoginRequired
			}
			return operation(tx, authority, principal, now.Truncate(time.Microsecond))
		})
}

// List returns one page of the Deliveries the caller may read, newest first.
func (d *Deliveries) List(ctx context.Context, proof identity.Proof, filter DeliveryFilter, page units.PageInput) (units.Page[Delivery], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Delivery]{}, err
	}
	if filter.State != "" && !slices.Contains([]string{Prepared, Started, RetryPending, Accepted, Failed, Uncertain, Expired,
		Cancelled}, filter.State) {
		return units.Page[Delivery]{}, authz.ErrInvalidInput
	}
	result := units.Page[Delivery]{Items: []Delivery{}}
	err = d.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, _ uuid.UUID, now time.Time) error {
		result = units.Page[Delivery]{Items: []Delivery{}}
		access, err := authz.NewDeliveryAccess(authority, ReadAction, now)
		if err != nil {
			return err
		}
		if !access.Granted() {
			return authz.ErrForbidden
		}
		records, total, err := d.deps.Store.ListDeliveries(tx.Context(), tx, DeliveryListQuery{Access: access,
			SubscriptionID: filter.SubscriptionID, State: filter.State, Offset: page.Offset, Limit: page.Limit})
		if err != nil {
			return err
		}
		result.Total = total
		for _, record := range records {
			result.Items = append(result.Items, record.view())
		}
		return nil
	})
	return result, err
}

// Read returns one Delivery with its attempts; a Delivery the caller may not read is
// authz.ErrNotFound.
func (d *Deliveries) Read(ctx context.Context, proof identity.Proof, id uuid.UUID) (Delivery, error) {
	if id == uuid.Nil {
		return Delivery{}, authz.ErrInvalidInput
	}
	var result Delivery
	err := d.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, _ uuid.UUID, now time.Time) error {
		record, err := d.authorized(tx.Context(), tx, authority, now, id, ReadAction, false)
		if err != nil {
			return err
		}
		result, err = d.withAttempts(tx.Context(), tx, record)
		return err
	})
	return result, err
}

// Retry makes a Delivery that failed with a known transient cause due again before its expiry.
// An uncertain Delivery fails with ErrExternalUncertain, any other state with
// authz.ErrRevisionConflict.
func (d *Deliveries) Retry(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (Delivery, error) {
	if id == uuid.Nil || expected <= 0 {
		return Delivery{}, authz.ErrInvalidInput
	}
	var result Delivery
	err := d.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, _ uuid.UUID, now time.Time) error {
		record, err := d.authorized(tx.Context(), tx, authority, now, id, RetryAction, true)
		if err != nil {
			return err
		}
		if record.Revision != expected {
			return authz.ErrRevisionConflict
		}
		code := ""
		if record.OutcomeCode != nil {
			code = *record.OutcomeCode
		}
		switch {
		case record.State == Uncertain:
			return &UncertainError{DeliveryID: record.ID}
		case record.State != Failed || (code != CodeRetryExhausted && code != CodeTransient) || !now.Before(record.ExpiresAt):
			return authz.ErrRevisionConflict
		}
		if err := d.deps.Store.SetRetry(tx.Context(), tx, id, expected, now); err != nil {
			return err
		}
		record, err = d.deps.Store.Delivery(tx.Context(), tx, id, false)
		if err != nil {
			return err
		}
		result, err = d.withAttempts(tx.Context(), tx, record)
		return err
	})
	return result, err
}

// Redeliver deliberately sends a terminal Delivery again as a new Delivery, materialized from
// the original Domain event under the subscriber's current authority and Subscription. The caller
// must confirm the duplicate risk; the command is idempotent per caller and command ID. It fails
// with ErrDependencyUnavailable when the original input ceiling has passed or the data is no
// longer available to the subscriber, and takes one unit of the subscriber's sending admission.
func (d *Deliveries) Redeliver(ctx context.Context, proof identity.Proof, id, command uuid.UUID, confirmDuplicateRisk bool,
	check func(identity.Authentication) error) (Delivery, error) {
	if id == uuid.Nil || command == uuid.Nil || !confirmDuplicateRisk {
		return Delivery{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(struct {
		ID      uuid.UUID `json:"id"`
		Confirm bool      `json:"confirm"`
	}{id, confirmDuplicateRisk})
	if err != nil {
		return Delivery{}, authz.ErrInvalidInput
	}
	var result Delivery
	err = d.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, principal uuid.UUID, now time.Time) error {
		receipt, found, err := d.receipts.ReadCommandReceipt(tx.Context(), tx, principal, command)
		if err != nil {
			return err
		}
		if found {
			stored, err := receipt.Check("delivery.redeliver", fingerprint)
			if err != nil {
				return err
			}
			record, err := d.deps.Store.Delivery(tx.Context(), tx, stored.ID, false)
			if err != nil {
				return err
			}
			result, err = d.withAttempts(tx.Context(), tx, record)
			return err
		}
		original, err := d.authorized(tx.Context(), tx, authority, now, id, RedeliverAction, true)
		if err != nil {
			return err
		}
		record, err := d.prepareRedelivery(tx.Context(), tx, original, now)
		if err != nil {
			return err
		}
		if err := d.admission.takeSubscriber(tx.Context(), tx, record.SubscriberID, now); err != nil {
			return err
		}
		if _, err := d.deps.Store.InsertDelivery(tx.Context(), tx, record); err != nil {
			return err
		}
		result = record.view()
		return d.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: principal, CommandID: command,
			Operation: "delivery.redeliver", Fingerprint: fingerprint,
			Result: commands.Result{ID: record.ID, Revision: record.Revision}, RecordedAt: now})
	})
	return result, err
}

// prepareRedelivery materializes a new Delivery for the original's Subscription, Domain event,
// target and split, bounded by the original Delivery's input ceiling.
func (d *Deliveries) prepareRedelivery(ctx context.Context, tx platform.TransactionContext, original DeliveryRecord,
	now time.Time) (DeliveryRecord, error) {
	if !terminal(original.State) {
		return DeliveryRecord{}, authz.ErrRevisionConflict
	}
	ceiling := original.CreatedAt.Add(FanOutWindow)
	if !now.Before(ceiling) {
		return DeliveryRecord{}, &DependencyError{Dependency: "input-ceiling"}
	}
	subscription, err := d.deps.Store.Subscription(ctx, tx, original.SubscriptionID, false)
	if errors.Is(err, authz.ErrNotFound) {
		return DeliveryRecord{}, &DependencyError{Dependency: "subscription"}
	}
	if err != nil {
		return DeliveryRecord{}, err
	}
	if subscription.State != StateActive {
		return DeliveryRecord{}, authz.ErrRevisionConflict
	}
	target, ok := subscriptionTarget(subscription, original.TargetInstanceID)
	if !ok {
		return DeliveryRecord{}, &DependencyError{Dependency: "subscription"}
	}
	envelope, err := d.deps.Store.DomainEvent(ctx, tx, original.DomainEventID)
	if errors.Is(err, authz.ErrNotFound) {
		return DeliveryRecord{}, &DependencyError{Dependency: "domain-event"}
	}
	if err != nil {
		return DeliveryRecord{}, err
	}
	event, err := parseEvent(original.DomainEventID, envelope)
	if err != nil {
		return DeliveryRecord{}, err
	}
	current, err := observe(ctx, tx, d.deps, now, subscription, event)
	if err != nil {
		return DeliveryRecord{}, err
	}
	if current == nil {
		return DeliveryRecord{}, &DependencyError{Dependency: "data"}
	}
	index := slices.IndexFunc(current.splits, func(part split) bool { return part.key == original.SplitKey })
	if index < 0 {
		return DeliveryRecord{}, &DependencyError{Dependency: "data"}
	}
	part := current.splits[index]
	expires := expiry(subscription, part, now)
	if ceiling.Before(expires) {
		expires = ceiling
	}
	if !expires.After(now) {
		return DeliveryRecord{}, &DependencyError{Dependency: "input-ceiling"}
	}
	fields := newFieldSet(ctx, tx, d.deps.Results)
	rendered, err := render(ctx, tx, d.deps, subscription, target, current.view(part, fields))
	if err != nil {
		return DeliveryRecord{}, err
	}
	if !rendered.available {
		return DeliveryRecord{}, &TargetUnavailableError{InstanceID: target.InstanceID}
	}
	if rendered.code != "" {
		return DeliveryRecord{}, &DependencyError{Dependency: "input"}
	}
	record := newRecord(subscription, current, part, target, rendered, now, expires)
	originalID := original.ID
	record.Origin, record.RedeliveryOf = OriginRedelivery, &originalID
	return record, nil
}

// authorized loads a Delivery the caller may act on with action. Without that right it is
// authz.ErrNotFound, unless the caller may read it, which makes a command authz.ErrForbidden.
func (d *Deliveries) authorized(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, now time.Time,
	id uuid.UUID, action string, lock bool) (DeliveryRecord, error) {
	record, err := d.deps.Store.Delivery(ctx, tx, id, lock)
	if err != nil {
		return DeliveryRecord{}, err
	}
	collections, err := d.deps.Store.UnitCollections(ctx, tx, record.UnitIDs)
	if err != nil {
		return DeliveryRecord{}, err
	}
	target := authz.DeliveryTarget{SubscriberID: record.SubscriberID, Agencies: record.AgencyIDs, Units: record.UnitIDs,
		UnitCollections: collections, AgeAnchor: record.AgeAnchor}
	allowed := func(action string) (bool, error) {
		access, err := authz.NewDeliveryAccess(authority, action, now)
		if err != nil {
			return false, err
		}
		return access.AllowsDelivery(target), nil
	}
	ok, err := allowed(action)
	if err != nil || ok {
		return record, err
	}
	if action != ReadAction {
		if readable, err := allowed(ReadAction); err != nil || readable {
			if err != nil {
				return DeliveryRecord{}, err
			}
			return DeliveryRecord{}, authz.ErrForbidden
		}
	}
	return DeliveryRecord{}, authz.ErrNotFound
}

// withAttempts returns the view of a record with its attempts.
func (d *Deliveries) withAttempts(ctx context.Context, tx platform.TransactionContext, record DeliveryRecord) (Delivery, error) {
	view := record.view()
	attempts, err := d.deps.Store.Attempts(ctx, tx, record.ID)
	if err != nil {
		return Delivery{}, err
	}
	view.Attempts = attempts
	return view, nil
}

// view returns the inspectable part of a record; the due time shows only while it is open.
func (r DeliveryRecord) view() Delivery {
	view := r.Delivery
	view.Attempts = nil
	if !terminal(r.State) && r.State != Started {
		due := r.Due
		view.NextAttemptAt = &due
	}
	return view
}
