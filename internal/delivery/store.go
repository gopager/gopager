package delivery

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// DeliveryRecord is a stored Delivery with its private input and authorization context.
type DeliveryRecord struct {
	Delivery
	// Input is the materialized input; nil once the Delivery ended or expired.
	Input *DeliveryInput
	// UnitIDs and AgencyIDs are the Delivery's relevant Units and the Agencies of its data.
	UnitIDs   []uuid.UUID
	AgencyIDs []uuid.UUID
	// AgeAnchor is the earliest first observation of the Delivery's data.
	AgeAnchor time.Time
	// AccessRevision is the access revision the input was materialized under.
	AccessRevision int64
	// InstanceRevision is the target instance revision the input was materialized for.
	InstanceRevision int64
	CorrelationID    *uuid.UUID
	CausationID      uuid.UUID
	// Due is when the Delivery may start next; meaningful only while it is open.
	Due            time.Time
	ClaimToken     int64
	ClaimExpiresAt *time.Time
}

// SubscriptionListQuery selects Subscriptions: the personal ones of Owner (uuid.Nil: none), all
// global ones with Global, otherwise the global ones of GlobalSubscribers.
type SubscriptionListQuery struct {
	Owner             uuid.UUID
	Global            bool
	GlobalSubscribers []uuid.UUID
	Offset            int
	Limit             int
}

// DeliveryListQuery selects the Deliveries an access permits, optionally of one Subscription
// and state, newest first.
type DeliveryListQuery struct {
	Access         authz.ActionAccess
	SubscriptionID *uuid.UUID
	State          string
	Offset         int
	Limit          int
}

// Claim is a fenced claim on a Delivery.
type Claim struct {
	DeliveryID uuid.UUID
	Token      int64
}

// AttemptRecord is a recorded attempt with the context it started under.
type AttemptRecord struct {
	Attempt
	AccessRevision int64
}

// Store is the persistence port of Subscriptions and Deliveries. Every method runs in the
// caller's transaction. Claimed writes need the claim's current token and an unexpired lease on
// the primary clock and fail with ErrLeaseLost otherwise.
type Store interface {
	// InsertSubscription stores a new Subscription with its targets.
	InsertSubscription(ctx context.Context, tx platform.TransactionContext, subscription Subscription) error
	// Subscription returns a Subscription or authz.ErrNotFound; lock takes a row lock for update.
	Subscription(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, lock bool) (Subscription, error)
	// ListSubscriptions returns one page ordered by creation then ID, and the total.
	ListSubscriptions(ctx context.Context, tx platform.TransactionContext, query SubscriptionListQuery) ([]Subscription, int, error)
	// UpdateSubscription replaces the Subscription at the expected revision (else
	// authz.ErrRevisionConflict); subscription.Revision is the new revision.
	UpdateSubscription(ctx context.Context, tx platform.TransactionContext, subscription Subscription, expected int64) error
	// DeleteSubscription removes it at the expected revision, else authz.ErrRevisionConflict.
	DeleteSubscription(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64) error
	// ActiveSubscriptions returns the active Subscriptions selecting the type that were created
	// at or before recordedAt, ordered by ID, each share-locked against a concurrent change.
	ActiveSubscriptions(ctx context.Context, tx platform.TransactionContext, eventType string, recordedAt time.Time) ([]Subscription, error)

	// InsertDelivery stores a Delivery. An automatic Delivery whose identity exists already is
	// not stored and reports false.
	InsertDelivery(ctx context.Context, tx platform.TransactionContext, record DeliveryRecord) (bool, error)
	// CancelOpen ends every unstarted Delivery of a Subscription as cancelled with code.
	CancelOpen(ctx context.Context, tx platform.TransactionContext, subscriptionID uuid.UUID, code string, now time.Time) error
	// ClaimDue claims up to limit due, unclaimed open Deliveries, optionally only of the given
	// target instances, with a new token and a lease until now plus lease.
	ClaimDue(ctx context.Context, tx platform.TransactionContext, now time.Time, lease time.Duration, limit int,
		instances []uuid.UUID) ([]DeliveryRecord, error)
	// ReclaimLost ends started Deliveries whose lease expired as uncertain with lease-lost and
	// finishes their open attempt the same way. It returns how many it ended.
	ReclaimLost(ctx context.Context, tx platform.TransactionContext, now time.Time, limit int) (int, error)
	// Renew extends a claim's lease to now plus lease.
	Renew(ctx context.Context, tx platform.TransactionContext, claim Claim, now time.Time, lease time.Duration) error
	// SaveClaimed writes the Delivery's state, outcome, counters, due time, input, materialization
	// context and finish time under the claim and increments its revision. Any state other than
	// started releases the claim.
	SaveClaimed(ctx context.Context, tx platform.TransactionContext, claim Claim, record DeliveryRecord, now time.Time) error
	// InsertAttempt records a started attempt under the claim.
	InsertAttempt(ctx context.Context, tx platform.TransactionContext, claim Claim, deliveryID uuid.UUID, attempt AttemptRecord, now time.Time) error
	// FinishAttempt records an attempt's outcome under the claim.
	FinishAttempt(ctx context.Context, tx platform.TransactionContext, claim Claim, deliveryID uuid.UUID, number int,
		state, code string, receipt json.RawMessage, now time.Time) error
	// Delivery returns one Delivery or authz.ErrNotFound; lock takes a row lock for update.
	Delivery(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, lock bool) (DeliveryRecord, error)
	// Attempts returns the attempts of a Delivery in order.
	Attempts(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) ([]Attempt, error)
	// ListDeliveries returns one page of the permitted Deliveries and the total.
	ListDeliveries(ctx context.Context, tx platform.TransactionContext, query DeliveryListQuery) ([]DeliveryRecord, int, error)
	// UnitCollections returns the Unit collections the Units belong to.
	UnitCollections(ctx context.Context, tx platform.TransactionContext, units []uuid.UUID) ([]uuid.UUID, error)
	// SetRetry makes an unclaimed failed Delivery retry-pending, due now, at the expected
	// revision (else authz.ErrRevisionConflict).
	SetRetry(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64, now time.Time) error

	// UnitParameters returns the parameters of a target instance for the Units, by Unit and key.
	UnitParameters(ctx context.Context, tx platform.TransactionContext, instance uuid.UUID, units []uuid.UUID) (map[uuid.UUID]map[string]processing.Value, error)
	// Gateway returns a Gateway's identity, name and Agencies, or authz.ErrNotFound.
	Gateway(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (GatewayView, error)
	// DomainEvent returns a recorded fact's envelope, or authz.ErrNotFound once it is gone.
	DomainEvent(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (json.RawMessage, error)
	// TakeSendAttempt counts one send in the bucket's window when it stays within limit.
	TakeSendAttempt(ctx context.Context, tx platform.TransactionContext, bucket [32]byte, window time.Time, limit int) (bool, error)
	// ReleaseSendAttempt takes back one send counted in the bucket's window.
	ReleaseSendAttempt(ctx context.Context, tx platform.TransactionContext, bucket [32]byte, window time.Time) error
	// MessageExists reports whether a Message is still stored.
	MessageExists(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (bool, error)
	// DeleteAccountData removes the Subscriptions, Deliveries and attempts of an account.
	DeleteAccountData(ctx context.Context, tx platform.TransactionContext, account uuid.UUID) error
}

// Instances reads target instances without an administrator's authority, for the Delivery
// side's own checks.
type Instances interface {
	// CurrentInstance returns the current revision of an instance, or authz.ErrNotFound.
	CurrentInstance(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (processing.InstanceConfiguration, error)
}

// PrincipalAuthority resolves the current authority of an account for background work.
// *identity.Resolver implements it.
type PrincipalAuthority interface {
	ResolvePrincipalAuthority(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (authz.Authority, error)
}

// Accounts reads the kind and state of an account. identitystore implements it.
type Accounts interface {
	Account(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (identity.AccountRecord, error)
}
