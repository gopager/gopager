package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ErrLeaseLost reports a claimed write whose token is no longer current or whose lease expired.
var ErrLeaseLost error = platform.NewFailure(platform.CodeRevisionConflict, errors.New("delivery claim lost"))

// DefaultPollInterval is the fallback interval at which the runner looks for due Deliveries.
const DefaultPollInterval = 5 * time.Second

// claimBatch is the number of Deliveries one claim transaction takes.
const claimBatch = 10

// PlacementGrace is how long a due Delivery may wait before any node claims it regardless of
// placement, so that a Delivery no node can serve is deferred instead of stranded.
const PlacementGrace = 60 * time.Second

// RunnerOptions configures a Runner.
type RunnerOptions struct {
	// Limits are the sending admission limits.
	Limits AdmissionLimits
	// PollInterval is the fallback poll; zero selects DefaultPollInterval.
	PollInterval time.Duration
	// RenewInterval is how often the claim is renewed during a provider call; zero selects
	// RenewInterval. Production uses the default; a shorter value lets tests observe renewal and
	// the cancellation of a call after revocation without waiting. At least three renewals must
	// fit into LeaseDuration.
	RenewInterval time.Duration
	// Placement returns the target instances whose Deliveries this node can serve now. It is
	// called once per Tick; an error fails the Tick. Nil means no restriction: every due Delivery
	// is claimed. Otherwise only Deliveries of the returned instances are claimed, plus those
	// overdue by PlacementGrace, which this node then defers when it cannot serve them.
	Placement func(ctx context.Context) ([]uuid.UUID, error)
	// Wakes, when set, wakes the runner on new facts in addition to the poll.
	Wakes events.WakeSubscriber
	// Logger receives batch failures; nil selects slog.Default().
	Logger *slog.Logger
}

// Runner claims due Deliveries and attempts them. Each Delivery is checked and recorded as
// started in one transaction, the provider is called outside any transaction while the claim is
// renewed, and the outcome is written in another transaction under the same claim.
type Runner struct {
	deps         Dependencies
	transactions events.Transactor
	clock        platform.Clock
	admission    admission
	options      RunnerOptions
	logger       *slog.Logger
}

// NewRunner returns a Delivery runner. It fails with authz.ErrInvalidInput on a missing port or
// an invalid interval.
func NewRunner(deps Dependencies, transactions events.Transactor, clock platform.Clock, options RunnerOptions) (*Runner, error) {
	if err := deps.validate(); err != nil || transactions == nil || clock == nil || options.PollInterval < 0 ||
		options.RenewInterval < 0 || 3*options.RenewInterval > LeaseDuration {
		return nil, authz.ErrInvalidInput
	}
	if options.PollInterval == 0 {
		options.PollInterval = DefaultPollInterval
	}
	if options.RenewInterval == 0 {
		options.RenewInterval = RenewInterval
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{deps: deps, transactions: transactions, clock: clock,
		admission: admission{store: deps.Store, limits: options.Limits.normalized()}, options: options, logger: logger}, nil
}

// Run processes due Deliveries until ctx ends, then returns nil. It implements platform.Runner.
func (r *Runner) Run(ctx context.Context) error {
	var signals <-chan struct{}
	if r.options.Wakes != nil {
		var release func()
		signals, release = r.options.Wakes.Subscribe()
		defer release()
	}
	ticker := time.NewTicker(r.options.PollInterval)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil {
			more, err := r.Tick(ctx)
			if err != nil {
				if ctx.Err() == nil {
					r.logger.Error("delivery runner batch failed", "error", err)
				}
				break
			}
			if !more {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-signals:
		case <-ticker.C:
		}
	}
}

// Tick recovers lost claims, claims one batch of due Deliveries and processes it. It reports
// whether the batch was full, so that another may follow at once.
func (r *Runner) Tick(ctx context.Context) (bool, error) {
	placed, err := r.placement(ctx)
	if err != nil {
		return false, err
	}
	var claimed []DeliveryRecord
	err = r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := r.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		if _, err := r.deps.Store.ReclaimLost(tx.Context(), tx, now, claimBatch); err != nil {
			return err
		}
		filter := placed
		filter.OverdueBefore = now.Add(-PlacementGrace)
		claimed, err = r.deps.Store.ClaimDue(tx.Context(), tx, now, LeaseDuration, claimBatch, filter)
		return err
	})
	if err != nil {
		return false, err
	}
	for _, record := range claimed {
		if err := r.process(ctx, record); err != nil && !errors.Is(err, ErrLeaseLost) {
			r.logger.Error("delivery attempt failed", "delivery", record.ID, "error", err)
		}
	}
	return len(claimed) == claimBatch, nil
}

// placement returns the claim filter for one Tick, without its overdue boundary.
func (r *Runner) placement(ctx context.Context) (ClaimFilter, error) {
	if r.options.Placement == nil {
		return ClaimFilter{}, nil
	}
	instances, err := r.options.Placement(ctx)
	if err != nil {
		return ClaimFilter{}, err
	}
	return ClaimFilter{Restricted: true, Instances: instances}, nil
}

// started is what the start transaction hands to the invocation.
type started struct {
	invocation AttemptInvocation
	attempt    int
	record     DeliveryRecord
}

// process runs the start check, the invocation and the outcome of one claimed Delivery.
func (r *Runner) process(ctx context.Context, record DeliveryRecord) error {
	claim := Claim{DeliveryID: record.ID, Token: record.ClaimToken}
	var begun *started
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		begun = nil
		var err error
		begun, err = r.start(tx.Context(), tx, claim, record)
		return err
	})
	if err != nil || begun == nil {
		return err
	}
	result, callErr := r.invoke(ctx, claim, begun)
	return r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		return r.finish(tx.Context(), tx, claim, begun, result, callErr)
	})
}

// start performs the authoritative checks immediately before an attempt and either records the
// attempt as started (returned) or ends, defers or cancels the Delivery (nil).
func (r *Runner) start(ctx context.Context, tx platform.TransactionContext, claim Claim, record DeliveryRecord) (*started, error) {
	now, err := r.clock.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	end := func(state, code string) (*started, error) {
		return nil, r.deps.Store.SaveClaimed(ctx, tx, claim, closed(record, state, code, now), now)
	}
	defer_ := func(state, code string, due time.Time) (*started, error) {
		next := record
		next.State, next.Due = state, due
		next.OutcomeCode = &code
		return nil, r.deps.Store.SaveClaimed(ctx, tx, claim, next, now)
	}
	if !now.Before(record.ExpiresAt) {
		return end(Expired, CodeExpired)
	}
	// The share lock holds until the start commits: a pause or update either waits for this start
	// or is seen by it, so no attempt starts after a pause has been confirmed.
	subscription, err := r.deps.Store.Subscription(ctx, tx, record.SubscriptionID, ShareLock)
	switch {
	case errors.Is(err, authz.ErrNotFound):
		return end(Cancelled, CodeSubscriptionDeleted)
	case err != nil:
		return nil, err
	case subscription.State != StateActive:
		return end(Cancelled, CodeSubscriptionPaused)
	case subscription.Health == HealthInvalid:
		return end(Cancelled, CodeNoLongerMatching)
	case subscription.Revision != record.SubscriptionRevision:
		return end(Cancelled, CodeSubscriptionChanged)
	}
	target, ok := subscriptionTarget(subscription, record.TargetInstanceID)
	if !ok {
		return end(Cancelled, CodeSubscriptionChanged)
	}
	current, code, err := r.current(ctx, tx, now, subscription, target, record)
	if err != nil {
		return nil, err
	}
	switch code {
	case "":
	case CodeDependency:
		return defer_(record.State, CodeDependency, now.Add(UnavailableDelay))
	case CodeParameterConflict, CodeInputInvalid, CodeRenderLimit:
		return end(Failed, code)
	default:
		return end(Cancelled, code)
	}
	ok, next, err := r.admission.takeSend(ctx, tx, record.SubscriberID, record.TargetInstanceID,
		current.descriptor.SendsPerMinute, now)
	if err != nil {
		return nil, err
	}
	if !ok {
		return defer_(RetryPending, CodeRateLimited, next)
	}
	number := current.record.AttemptCount + 1
	attempt := AttemptRecord{Attempt: Attempt{Number: number, ID: uuid.New(), State: Started,
		InstanceRevision: current.instanceRevision, StartedAt: now}, AccessRevision: current.record.AccessRevision}
	if err := r.deps.Store.InsertAttempt(ctx, tx, claim, record.ID, attempt, now); err != nil {
		return nil, err
	}
	next_ := current.record
	next_.State, next_.AttemptCount, next_.OutcomeCode, next_.Due = Started, number, nil, now
	if err := r.deps.Store.SaveClaimed(ctx, tx, claim, next_, now); err != nil {
		return nil, err
	}
	if err := r.deps.Store.Renew(ctx, tx, claim, now, LeaseDuration); err != nil {
		return nil, err
	}
	return &started{record: next_, attempt: number, invocation: AttemptInvocation{Version: InvocationVersion,
		DeliveryID: record.ID, AttemptID: attempt.ID, AttemptNumber: number, InstanceID: record.TargetInstanceID,
		SubscriberID: record.SubscriberID, InstanceRevision: current.instanceRevision, Input: *next_.Input, CorrelationID: record.CorrelationID,
		CausationID: record.CausationID, Deadline: now.Add(AttemptDeadline)}}, nil
}

// currentState is a Delivery re-checked against current authority, data and target.
type currentState struct {
	record           DeliveryRecord
	descriptor       TargetDescriptor
	instanceRevision int64
}

// current re-checks the subscriber, the data and the target of a Delivery. It returns a code
// when the Delivery must not start now: a cancellation reason, a materialization failure or
// dependency-unavailable. The input is reused while the access and instance revisions it was
// materialized under are current, and materialized again otherwise.
func (r *Runner) current(ctx context.Context, tx platform.TransactionContext, now time.Time, subscription Subscription,
	target TargetReference, record DeliveryRecord) (currentState, string, error) {
	envelope, err := r.deps.Store.DomainEvent(ctx, tx, record.DomainEventID)
	missing := errors.Is(err, authz.ErrNotFound)
	if err != nil && !missing {
		return currentState{}, "", err
	}
	var subject *subject
	var check *compiled
	if !missing {
		event, err := parseEvent(record.DomainEventID, envelope)
		if err != nil {
			return currentState{}, "", err
		}
		resolved, err := resolveSubject(ctx, tx, r.deps, event, subscription.SubscriberID)
		if err != nil {
			return currentState{}, "", err
		}
		if resolved != nil {
			var reason string
			if check, reason, err = healthCheck(ctx, tx, r.deps, subscription, resolved.groups); err != nil {
				return currentState{}, "", err
			}
			if reason != "" {
				return currentState{}, CodeNoLongerMatching, markInvalid(ctx, tx, r.deps, subscription, reason, now)
			}
			if subject, err = resolved.project(ctx, tx, r.deps, now, subscription); err != nil {
				return currentState{}, "", err
			}
		}
	}
	authority, err := r.deps.Authority.ResolvePrincipalAuthority(ctx, tx, record.SubscriberID)
	if err != nil {
		if absentAuthority(err) {
			return currentState{}, CodeAuthorityRevoked, nil
		}
		return currentState{}, "", err
	}
	instance, err := r.deps.Instances.CurrentInstance(ctx, tx, record.TargetInstanceID)
	if errors.Is(err, authz.ErrNotFound) {
		return currentState{}, CodeDependency, nil
	}
	if err != nil {
		return currentState{}, "", err
	}
	if subject == nil {
		// The fact or its data is gone, or the subscriber may no longer see it. Self-contained
		// input may still be sent when the data was removed under unchanged authority.
		if record.MessageID == nil || record.Input == nil || authority.Revision != record.AccessRevision ||
			record.InstanceRevision != instance.Revision {
			return currentState{}, CodeAuthorityRevoked, nil
		}
		exists, err := r.deps.Store.MessageExists(ctx, tx, *record.MessageID)
		if err != nil {
			return currentState{}, "", err
		}
		if exists {
			return currentState{}, CodeAuthorityRevoked, nil
		}
		descriptor, code, err := r.describe(ctx, instance)
		if err != nil || code != "" {
			return currentState{}, code, err
		}
		return currentState{record: record, descriptor: descriptor, instanceRevision: instance.Revision}, "", nil
	}
	index := slices.IndexFunc(subject.splits, func(part split) bool { return part.key == record.SplitKey })
	if index < 0 {
		return currentState{}, CodeNoLongerMatching, nil
	}
	partView := subject.view(subject.splits[index], check.declarations)
	if !matches(check.filter, partView) {
		return currentState{}, CodeNoLongerMatching, nil
	}
	if record.Input != nil && authority.Revision == record.AccessRevision && record.InstanceRevision == instance.Revision {
		descriptor, code, err := r.describe(ctx, instance)
		if err != nil || code != "" {
			return currentState{}, code, err
		}
		return currentState{record: record, descriptor: descriptor, instanceRevision: instance.Revision}, "", nil
	}
	result, err := render(ctx, tx, r.deps, subscription, target, partView)
	if err != nil {
		return currentState{}, "", err
	}
	if !result.available {
		return currentState{}, CodeDependency, nil
	}
	if result.code != "" {
		return currentState{}, result.code, nil
	}
	updated := record
	updated.Input, updated.AccessRevision, updated.InstanceRevision = result.input, authority.Revision, result.instanceRevision
	updated.LayoutID, updated.LayoutVersion = result.layoutID, result.layoutVersion
	return currentState{record: updated, descriptor: result.descriptor, instanceRevision: result.instanceRevision}, "", nil
}

// describe returns the current descriptor of an enabled instance, or dependency-unavailable.
func (r *Runner) describe(ctx context.Context, instance processing.InstanceConfiguration) (TargetDescriptor, string, error) {
	if !instance.Enabled {
		return TargetDescriptor{}, CodeDependency, nil
	}
	descriptor, err := r.deps.Targets.Describe(ctx, instance)
	if err == nil {
		err = descriptor.validate(instance)
	}
	if err != nil {
		return TargetDescriptor{}, CodeDependency, nil
	}
	return descriptor, "", nil
}

// invoke calls the provider outside any transaction with the attempt deadline. It renews the
// claim every renewal interval and cancels the call when the claim is lost or the subscriber may
// no longer see the Delivery's data; a call cancelled after start is uncertain.
func (r *Runner) invoke(ctx context.Context, claim Claim, begun *started) (AttemptResult, error) {
	callCtx, cancel := context.WithDeadline(ctx, begun.invocation.Deadline)
	defer cancel()
	done := make(chan struct{})
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		ticker := time.NewTicker(r.options.RenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-callCtx.Done():
				return
			case <-ticker.C:
				if !r.keep(ctx, claim, begun.record) {
					cancel()
					return
				}
			}
		}
	}()
	result, err := r.deps.Targets.Attempt(callCtx, begun.invocation)
	close(done)
	<-watched
	return result, err
}

// keep renews the claim and reports whether the call may continue: the lease is still held, the
// subscriber's account still resolves and, once the access revision differs from the one the
// attempt started under, the subscriber may still see the Delivery's data. Unrelated grant
// changes elsewhere bump the same revision, so only a failed re-check cancels the call.
func (r *Runner) keep(ctx context.Context, claim Claim, record DeliveryRecord) bool {
	err := r.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := r.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		if err := r.deps.Store.Renew(tx.Context(), tx, claim, now, LeaseDuration); err != nil {
			return err
		}
		authority, err := r.deps.Authority.ResolvePrincipalAuthority(tx.Context(), tx, record.SubscriberID)
		if err != nil || authority.Revision == record.AccessRevision {
			return err
		}
		return r.stillVisible(tx.Context(), tx, now, record)
	})
	return err == nil
}

// errNoLongerVisible ends a running call whose data the subscriber may no longer see.
var errNoLongerVisible = errors.New("delivery data no longer visible to the subscriber")

// stillVisible re-projects the Delivery's fact for its subscriber and requires its split to be
// visible. A removed fact or Subscription counts as not visible.
func (r *Runner) stillVisible(ctx context.Context, tx platform.TransactionContext, now time.Time, record DeliveryRecord) error {
	subscription, err := r.deps.Store.Subscription(ctx, tx, record.SubscriptionID, NoLock)
	if errors.Is(err, authz.ErrNotFound) {
		return errNoLongerVisible
	}
	if err != nil {
		return err
	}
	envelope, err := r.deps.Store.DomainEvent(ctx, tx, record.DomainEventID)
	if errors.Is(err, authz.ErrNotFound) {
		return errNoLongerVisible
	}
	if err != nil {
		return err
	}
	event, err := parseEvent(record.DomainEventID, envelope)
	if err != nil {
		return err
	}
	current, err := observe(ctx, tx, r.deps, now, subscription, event)
	if err != nil {
		return err
	}
	if current == nil || !slices.ContainsFunc(current.splits, func(part split) bool { return part.key == record.SplitKey }) {
		return errNoLongerVisible
	}
	return nil
}

// finish records an attempt's outcome under the claim. A lost claim leaves the Delivery to
// ReclaimLost, which records it as uncertain.
func (r *Runner) finish(ctx context.Context, tx platform.TransactionContext, claim Claim, begun *started,
	result AttemptResult, callErr error) error {
	now, err := r.clock.Now(ctx, tx)
	if err != nil {
		return err
	}
	status := result.Status
	if callErr != nil || !slices.Contains([]AttemptStatus{StatusAccepted, StatusTransient, StatusPermanent, StatusUncertain}, status) {
		status = StatusUncertain
	}
	receipt := result.Receipt
	if callErr != nil || len(receipt) > MaxReceiptBytes || (len(receipt) > 0 && !json.Valid(receipt)) {
		receipt = nil
	}
	record := begun.record
	var attemptState, code string
	switch status {
	case StatusAccepted:
		attemptState, code = string(StatusAccepted), CodeAccepted
		record = closed(record, Accepted, CodeAccepted, now)
	case StatusPermanent:
		attemptState, code = string(StatusPermanent), CodePermanent
		record = closed(record, Failed, CodePermanent, now)
	case StatusUncertain:
		attemptState, code = string(StatusUncertain), CodeUncertain
		record = closed(record, Uncertain, CodeUncertain, now)
	case StatusTransient:
		attemptState, code = string(StatusTransient), CodeTransient
		record = r.reschedule(record, result.RetryAfter, now)
	}
	if err := r.deps.Store.FinishAttempt(ctx, tx, claim, record.ID, begun.attempt, attemptState, code, receipt, now); err != nil {
		return err
	}
	return r.deps.Store.SaveClaimed(ctx, tx, claim, record, now)
}

// reschedule schedules the next automatic attempt after a transient failure, or ends the
// Delivery when the attempts are used up or the next attempt would not precede its expiry.
func (r *Runner) reschedule(record DeliveryRecord, retryAfter time.Duration, now time.Time) DeliveryRecord {
	if record.AttemptCount >= processing.MaxAutomaticAttempts {
		return closed(record, Failed, CodeRetryExhausted, now)
	}
	due, more, err := processing.NextAutomaticRetry(record.AttemptCount, now)
	if err != nil || !more {
		return closed(record, Failed, CodeRetryExhausted, now)
	}
	if retryAfter > 0 && now.Add(retryAfter).After(due) {
		due = now.Add(retryAfter).Truncate(time.Microsecond)
	}
	if !due.Before(record.ExpiresAt) {
		return closed(record, Expired, CodeExpired, now)
	}
	code := CodeTransient
	record.State, record.OutcomeCode, record.Due = RetryPending, &code, due
	return record
}

// closed returns the record in a terminal state without its input.
func closed(record DeliveryRecord, state, code string, now time.Time) DeliveryRecord {
	record.State, record.Input = state, nil
	record.OutcomeCode = &code
	finished := now
	record.FinishedAt = &finished
	return record
}

// subscriptionTarget returns the Subscription's reference to a target instance.
func subscriptionTarget(subscription Subscription, instance uuid.UUID) (TargetReference, bool) {
	for _, target := range subscription.Targets {
		if target.InstanceID == instance {
			return target, true
		}
	}
	return TargetReference{}, false
}
