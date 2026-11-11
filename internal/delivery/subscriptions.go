package delivery

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/units"
)

// Subscription actions.
const (
	ManageOwnAction    = "subscription.manage-own"
	ManageGlobalAction = "subscription.manage-global"
)

// Subscriptions manages personal and global Subscriptions over the caller's current authority.
// Every change writes a configuration.changed fact in the same transaction. Create it with
// NewSubscriptions; it is safe for concurrent use.
type Subscriptions struct {
	deps         Dependencies
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
	receipts     commands.Repository
	accounts     Accounts
	admission    admission
}

// NewSubscriptions composes the Subscription service. It fails with authz.ErrInvalidInput on a
// missing dependency.
func NewSubscriptions(deps Dependencies, transactions identity.Transactions, resolver identity.AuthorityResolver,
	clock platform.Clock, receipts commands.Repository, accounts Accounts,
	limits AdmissionLimits) (*Subscriptions, error) {
	if err := deps.validate(); err != nil || transactions == nil || resolver == nil || clock == nil ||
		receipts == nil || accounts == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Subscriptions{deps: deps, transactions: transactions, resolver: resolver, clock: clock, receipts: receipts,
		accounts: accounts, admission: admission{store: deps.Store, limits: limits.normalized()}}, nil
}

// manager is what one Subscription operation knows about its caller.
type manager struct {
	authority authz.Authority
	principal uuid.UUID
	guest     bool
	now       time.Time
}

// ownsPersonal reports whether the caller may manage its own personal Subscriptions.
func (m manager) ownsPersonal() bool {
	return !m.guest && authz.Authorize(m.authority, ManageOwnAction, nil, authz.Target{AccountID: m.principal}, m.now) == nil
}

// managesGlobal reports whether the caller may manage global Subscriptions of the subscriber.
func (m manager) managesGlobal(subscriber uuid.UUID) bool {
	return !m.guest && authz.Authorize(m.authority, ManageGlobalAction, nil, authz.Target{AccountID: subscriber}, m.now) == nil
}

// visible reports whether the caller may see and change a Subscription.
func (m manager) visible(subscription Subscription) bool {
	if subscription.Kind == KindPersonal {
		return subscription.SubscriberID == m.principal && m.ownsPersonal()
	}
	return m.managesGlobal(subscription.SubscriberID)
}

// within resolves the caller and runs operation in the same transaction.
func (s *Subscriptions) within(ctx context.Context, proof identity.Proof, mutation bool, check func(identity.Authentication) error,
	operation func(tx platform.TransactionContext, who manager) error) error {
	var authentication identity.Authentication
	remember := func(value identity.Authentication) error {
		authentication = value
		if check != nil {
			return check(value)
		}
		return nil
	}
	return identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, mutation, remember,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			principal, err := uuid.Parse(authority.PrincipalID)
			if err != nil || principal == uuid.Nil {
				return authz.ErrForbidden
			}
			who := manager{authority: authority, principal: principal, now: now.Truncate(time.Microsecond),
				guest: authentication.Kind == identity.AnonymousProof}
			if mutation && who.guest {
				return identity.ErrLoginRequired
			}
			return operation(tx, who)
		})
}

// List returns the caller's personal Subscriptions and the global ones it manages.
func (s *Subscriptions) List(ctx context.Context, proof identity.Proof, page units.PageInput) (units.Page[Subscription], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Subscription]{}, err
	}
	result := units.Page[Subscription]{Items: []Subscription{}}
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who manager) error {
		result = units.Page[Subscription]{Items: []Subscription{}}
		query := SubscriptionListQuery{Offset: page.Offset, Limit: page.Limit}
		if who.ownsPersonal() {
			query.Owner = who.principal
		}
		query.Global, query.GlobalSubscribers = globalReach(who)
		if query.Owner == uuid.Nil && !query.Global && len(query.GlobalSubscribers) == 0 {
			return nil
		}
		items, total, err := s.deps.Store.ListSubscriptions(tx.Context(), tx, query)
		if err != nil {
			return err
		}
		result.Items, result.Total = items, total
		return nil
	})
	return result, err
}

// globalReach returns whether the caller manages every global Subscription, or else the
// subscribers whose global Subscriptions it manages.
func globalReach(who manager) (bool, []uuid.UUID) {
	if who.guest {
		return false, nil
	}
	var subscribers []uuid.UUID
	for _, grant := range who.authority.Grants {
		if !slices.Contains(grant.Actions, ManageGlobalAction) {
			continue
		}
		if grant.Scope.Kind == "all" {
			// A restricting key may still narrow an unrestricted grant; any account stands in.
			return who.managesGlobal(uuid.New()), nil
		}
		for _, reference := range grant.Scope.References {
			if id, err := uuid.Parse(reference); err == nil && who.managesGlobal(id) {
				subscribers = appendUnique(subscribers, id)
			}
		}
	}
	return false, subscribers
}

// Read returns one Subscription the caller may manage; anything else is authz.ErrNotFound.
func (s *Subscriptions) Read(ctx context.Context, proof identity.Proof, id uuid.UUID) (Subscription, error) {
	if id == uuid.Nil {
		return Subscription{}, authz.ErrInvalidInput
	}
	var result Subscription
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who manager) error {
		subscription, err := s.deps.Store.Subscription(tx.Context(), tx, id, NoLock)
		if err != nil {
			return err
		}
		if !who.visible(subscription) {
			return authz.ErrNotFound
		}
		result = subscription
		return nil
	})
	return result, err
}

// Create stores a Subscription with actor-bound command idempotency and takes one unit of the
// subscriber's sending admission (*identity.RateLimited when exhausted). An identical replay
// returns the Subscription as it is now, or authz.ErrNotFound when the caller may no longer see it.
func (s *Subscriptions) Create(ctx context.Context, proof identity.Proof, command uuid.UUID, input SubscriptionInput,
	check func(identity.Authentication) error) (Subscription, error) {
	if command == uuid.Nil {
		return Subscription{}, authz.ErrInvalidInput
	}
	input, err := canonical(input)
	if err != nil {
		return Subscription{}, err
	}
	fingerprint, err := commands.Fingerprint(input)
	if err != nil {
		return Subscription{}, authz.ErrInvalidInput
	}
	var result Subscription
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who manager) error {
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, who.principal, command)
		if err != nil {
			return err
		}
		if found {
			stored, err := receipt.Check("subscription.create", fingerprint)
			if err != nil {
				return err
			}
			result, err = s.deps.Store.Subscription(tx.Context(), tx, stored.ID, NoLock)
			if err == nil && !who.visible(result) {
				result, err = Subscription{}, authz.ErrNotFound
			}
			return err
		}
		subscription := Subscription{ID: uuid.New(), Revision: 1, Health: HealthValid, CreatedAt: who.now,
			UpdatedAt: who.now, CreatedBy: who.principal}
		if err := s.apply(tx.Context(), tx, who, &subscription, input); err != nil {
			return err
		}
		if err := s.admission.takeSubscriber(tx.Context(), tx, subscription.SubscriberID, who.now); err != nil {
			return err
		}
		if err := s.deps.Store.InsertSubscription(tx.Context(), tx, subscription); err != nil {
			return err
		}
		if err := s.fact(tx.Context(), tx, subscription.ID, 1, "created", who.now); err != nil {
			return err
		}
		result = subscription
		return s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: who.principal, CommandID: command,
			Operation: "subscription.create", Fingerprint: fingerprint,
			Result: commands.Result{ID: subscription.ID, Revision: 1}, RecordedAt: who.now})
	})
	return result, err
}

// Update replaces a Subscription at the expected revision. Kind and subscriber cannot change.
// Unstarted Deliveries of the old revision are cancelled; pausing records the change kind paused.
func (s *Subscriptions) Update(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input SubscriptionInput,
	check func(identity.Authentication) error) (Subscription, error) {
	if id == uuid.Nil {
		return Subscription{}, authz.ErrInvalidInput
	}
	input, err := canonical(input)
	if err != nil {
		return Subscription{}, err
	}
	var result Subscription
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who manager) error {
		current, err := s.deps.Store.Subscription(tx.Context(), tx, id, UpdateLock)
		if err != nil {
			return err
		}
		if !who.visible(current) {
			return authz.ErrNotFound
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if input.Kind != current.Kind || (input.SubscriberID != nil && *input.SubscriberID != current.SubscriberID) {
			return authz.ErrInvalidInput
		}
		input.SubscriberID = &current.SubscriberID
		updated := current
		updated.Revision, updated.UpdatedAt = next, who.now
		if err := s.apply(tx.Context(), tx, who, &updated, input); err != nil {
			return err
		}
		if err := s.deps.Store.UpdateSubscription(tx.Context(), tx, updated, expected); err != nil {
			return err
		}
		change, code := "updated", CodeSubscriptionChanged
		if current.State == StateActive && updated.State == StatePaused {
			change, code = "paused", CodeSubscriptionPaused
		}
		if err := s.deps.Store.CancelOpen(tx.Context(), tx, id, code, who.now); err != nil {
			return err
		}
		result = updated
		if err := s.fact(tx.Context(), tx, id, next, change, who.now); err != nil {
			return err
		}
		if current.Health != HealthInvalid {
			return nil
		}
		return appendHealthFact(tx.Context(), tx, s.deps, updated, HealthValid, "", who.now)
	})
	return result, err
}

// Delete removes a Subscription at the expected revision and cancels its unstarted Deliveries.
// Delivery history keeps the Subscription ID.
func (s *Subscriptions) Delete(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who manager) error {
		current, err := s.deps.Store.Subscription(tx.Context(), tx, id, UpdateLock)
		if err != nil {
			return err
		}
		if !who.visible(current) {
			return authz.ErrNotFound
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.deps.Store.CancelOpen(tx.Context(), tx, id, CodeSubscriptionDeleted, who.now); err != nil {
			return err
		}
		if err := s.deps.Store.DeleteSubscription(tx.Context(), tx, id, expected); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return s.fact(tx.Context(), tx, id, next, "deleted", who.now)
	})
	return result, err
}

// fact appends the configuration.changed fact of a Subscription change.
func (s *Subscriptions) fact(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, revision int64,
	change string, now time.Time) error {
	_, err := s.deps.Facts.AppendSubscriptionFact(ctx, tx, events.SubscriptionFact{ID: uuid.New(), Subscription: id,
		Revision: revision, OccurredAt: now, ChangeKind: change})
	return err
}

// canonical validates the shape of an input and returns it in canonical form: sorted distinct
// event types and an explicit capability kind.
func canonical(input SubscriptionInput) (SubscriptionInput, error) {
	if input.Kind != KindPersonal && input.Kind != KindGlobal {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	if input.State != StateActive && input.State != StatePaused {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	if (input.Kind == KindGlobal) != (input.SubscriberID != nil) || (input.SubscriberID != nil && *input.SubscriberID == uuid.Nil) {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	if len(input.EventTypes) == 0 || len(input.EventTypes) > MaxEventTypes {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	types := slices.Clone(input.EventTypes)
	slices.Sort(types)
	if len(slices.Compact(slices.Clone(types))) != len(types) {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	for _, eventType := range types {
		if entry, ok := events.LookupType(eventType); !ok || !entry.Deliverable {
			return SubscriptionInput{}, authz.ErrInvalidInput
		}
	}
	input.EventTypes = types
	if len(input.Targets) == 0 || len(input.Targets) > MaxTargets {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	seen := map[uuid.UUID]bool{}
	targets := make([]TargetReference, 0, len(input.Targets))
	for _, target := range input.Targets {
		if target.InstanceID == uuid.Nil || seen[target.InstanceID] || target.Revision < 0 ||
			(target.CapabilityKind != "" && target.CapabilityKind != CapabilityDelivery) || len(target.Inputs) > MaxTargetInputs {
			return SubscriptionInput{}, authz.ErrInvalidInput
		}
		seen[target.InstanceID] = true
		for name := range target.Inputs {
			if name == "" || len(name) > 128 {
				return SubscriptionInput{}, authz.ErrInvalidInput
			}
		}
		target.CapabilityKind = CapabilityDelivery
		if target.Inputs == nil {
			target.Inputs = map[string]processing.Value{}
		}
		targets = append(targets, target)
	}
	input.Targets = targets
	if input.MaxAgeSeconds != nil && (*input.MaxAgeSeconds < 1 || *input.MaxAgeSeconds > MaxAgeSeconds) {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	if input.LayoutID != nil && *input.LayoutID == uuid.Nil {
		return SubscriptionInput{}, authz.ErrInvalidInput
	}
	return input, nil
}

// apply authorizes and validates input against the current subscriber, field catalog, layout and
// targets, and writes it into subscription.
func (s *Subscriptions) apply(ctx context.Context, tx platform.TransactionContext, who manager, subscription *Subscription,
	input SubscriptionInput) error {
	subscriber := who.principal
	if input.Kind == KindGlobal {
		subscriber = *input.SubscriberID
	}
	switch input.Kind {
	case KindPersonal:
		if !who.ownsPersonal() {
			return authz.ErrForbidden
		}
	case KindGlobal:
		if !who.managesGlobal(subscriber) {
			return authz.ErrForbidden
		}
		account, err := s.accounts.Account(ctx, tx, subscriber)
		if err != nil || account.Kind != "technical" || account.State != "active" {
			if err != nil && !errors.Is(err, authz.ErrNotFound) && !absentAuthority(err) {
				return err
			}
			return authz.ErrInvalidInput
		}
	}
	authority := who.authority
	if subscriber != who.principal {
		var err error
		authority, err = s.deps.Authority.ResolvePrincipalAuthority(ctx, tx, subscriber)
		if err != nil {
			if absentAuthority(err) {
				return authz.ErrInvalidInput
			}
			return err
		}
	}
	readable, err := authz.ReadableFieldGroups(authority)
	if err != nil {
		return err
	}
	if input.LayoutID != nil {
		// The author chooses the Layout; for a global Subscription that is the manager, not the
		// technical account that is only the subscriber.
		if err := s.deps.Layouts.CheckUse(ctx, tx, who.authority, *input.LayoutID, who.now); err != nil {
			return err
		}
	}
	if err := s.deps.Fields.LockCatalog(ctx, tx); err != nil {
		return err
	}
	check, err := compileSelection(ctx, tx, s.deps, selection{filter: input.Filter, layoutID: input.LayoutID,
		splitByUnit: input.SplitByUnit})
	var failure *fault
	if errors.As(err, &failure) {
		return failure.err
	}
	if err != nil {
		return err
	}
	if input.LayoutID != nil && check.layoutKind == "" {
		return authz.ErrInvalidInput
	}
	if check.unreadable(groupSet(readable)) {
		return authz.ErrInvalidInput
	}
	layoutKind := check.layoutKind
	targets, err := s.checkTargets(ctx, tx, input.Targets, layoutKind)
	if err != nil {
		return err
	}
	subscription.Kind, subscription.SubscriberID, subscription.State = input.Kind, subscriber, input.State
	subscription.EventTypes, subscription.Filter, subscription.Targets = input.EventTypes, input.Filter, targets
	subscription.LayoutID, subscription.SplitByUnit, subscription.MaxAgeSeconds = input.LayoutID, input.SplitByUnit, input.MaxAgeSeconds
	subscription.Health, subscription.HealthReason = HealthValid, nil
	return nil
}

// checkTargets requires every target to be the current revision of an enabled instance with an
// available descriptor that supports the layout's output kind and declares the given inputs.
func (s *Subscriptions) checkTargets(ctx context.Context, tx platform.TransactionContext, targets []TargetReference,
	layoutKind string) ([]TargetReference, error) {
	checked := make([]TargetReference, 0, len(targets))
	for _, target := range targets {
		instance, err := s.deps.Instances.CurrentInstance(ctx, tx, target.InstanceID)
		if errors.Is(err, authz.ErrNotFound) {
			return nil, &TargetUnavailableError{InstanceID: target.InstanceID}
		}
		if err != nil {
			return nil, err
		}
		if !instance.Enabled {
			return nil, &TargetUnavailableError{InstanceID: target.InstanceID}
		}
		descriptor, err := s.deps.Targets.Describe(ctx, instance)
		if err == nil {
			err = descriptor.validate(instance)
		}
		if err != nil {
			return nil, &TargetUnavailableError{InstanceID: target.InstanceID}
		}
		if target.Revision != 0 && target.Revision != instance.Revision {
			return nil, authz.ErrRevisionConflict
		}
		if layoutKind != "" && !slices.Contains(descriptor.OutputKinds, layoutKind) {
			return nil, authz.ErrInvalidInput
		}
		for name, value := range target.Inputs {
			field, declared := descriptor.input(name)
			if !declared || validInput(field, value) != nil {
				return nil, authz.ErrInvalidInput
			}
		}
		target.Revision = instance.Revision
		checked = append(checked, target)
	}
	return checked, nil
}

// groupSet returns the field groups as a set.
func groupSet(groups []string) map[string]bool {
	set := make(map[string]bool, len(groups))
	for _, group := range groups {
		set[group] = true
	}
	return set
}
