// Package delivery owns Subscriptions and Deliveries: the selection of Domain events, the
// materialization of an authorized, rendered input per Subscription, target and split, the
// claimed and fenced attempts against a target provider, sending admission, and the management
// of Delivery results. It never holds a SQL transaction across provider I/O.
package delivery

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
)

// Subscription kinds.
const (
	// KindPersonal is a Subscription that acts as its owning User.
	KindPersonal = "personal"
	// KindGlobal is a Subscription that acts as an assigned technical account.
	KindGlobal = "global"
)

// Subscription states.
const (
	// StateActive Subscriptions create Deliveries.
	StateActive = "active"
	// StatePaused Subscriptions create none and let no Delivery start.
	StatePaused = "paused"
)

// Subscription health.
const (
	// HealthValid Subscriptions compile against the current field registry.
	HealthValid = "valid"
	// HealthInvalid Subscriptions create no Deliveries until a save that compiles.
	HealthInvalid = "invalid"
)

// Reasons a Subscription is invalid.
const (
	// ReasonFilterInvalid means the filter fails to compile for another reason than below.
	ReasonFilterInvalid = "filter-invalid"
	// ReasonFieldUnavailable means a field is unknown, deprecated or in a group the subscriber
	// can no longer read.
	ReasonFieldUnavailable = "field-unavailable"
	// ReasonLayoutInvalid means the Layout fails to compile.
	ReasonLayoutInvalid = "layout-invalid"
	// ReasonResourceLimit means a bound of the filter or Layout is exceeded.
	ReasonResourceLimit = "resource-limit"
)

// CapabilityDelivery is the capability kind of a Delivery target reference.
const CapabilityDelivery = "delivery"

// Delivery states. Prepared, started and retry-pending are open; the others are terminal.
const (
	Prepared     = "prepared"
	Started      = "started"
	RetryPending = "retry-pending"
	Accepted     = "accepted"
	Failed       = "failed"
	Uncertain    = "uncertain"
	Expired      = "expired"
	Cancelled    = "cancelled"
)

// Delivery origins.
const (
	// OriginAutomatic Deliveries are created by event fan-out, once per identity.
	OriginAutomatic = "automatic"
	// OriginRedelivery Deliveries are created by an explicit, confirmed redelivery.
	OriginRedelivery = "redelivery"
)

// Outcome codes recorded on a Delivery or attempt.
const (
	CodeAccepted            = "accepted"
	CodeTransient           = "transient"
	CodePermanent           = "permanent"
	CodeRetryExhausted      = "retry-exhausted"
	CodeUncertain           = "uncertain"
	CodeLeaseLost           = "lease-lost"
	CodeExpired             = "expired"
	CodeSubscriptionPaused  = "subscription-paused"
	CodeSubscriptionChanged = "subscription-changed"
	CodeSubscriptionDeleted = "subscription-deleted"
	CodeAuthorityRevoked    = "authority-revoked"
	CodeNoLongerMatching    = "no-longer-matching"
	CodeParameterConflict   = "parameter-conflict"
	CodeInputInvalid        = "input-invalid"
	CodeRenderLimit         = "render-limit"
	CodeRateLimited         = "rate-limited"
	CodeDependency          = "dependency-unavailable"
)

// Limits and timings.
const (
	MaxEventTypes = 16
	MaxTargets    = 8
	// MaxTargetInputs bounds the constant inputs of one Subscription target.
	MaxTargetInputs = 64
	// MaxAgeSeconds is the largest optional Subscription age limit.
	MaxAgeSeconds = 86400
	// FanOutWindow is how long after its recording a fact may still create Deliveries, and how
	// long after creation a Delivery's input lives at most.
	FanOutWindow = 24 * time.Hour
	// MaxBodyBytes bounds a rendered Delivery body.
	MaxBodyBytes = processing.MaxLayoutOutputBytes
	// LeaseDuration is the claim lease of a Delivery; RenewInterval its renewal during a call.
	LeaseDuration = 60 * time.Second
	RenewInterval = 20 * time.Second
	// AttemptDeadline bounds one provider call.
	AttemptDeadline = 30 * time.Second
	// UnavailableDelay defers a Delivery whose target provider is unavailable.
	UnavailableDelay = 60 * time.Second
	// MaxReceiptBytes bounds a stored provider receipt.
	MaxReceiptBytes = 4 << 10
)

// TargetReference names one configured instance with the Delivery capability. Revision is the
// instance revision current when the Subscription was saved; Inputs are typed constants for
// inputs the instance's descriptor declares. InputBindings name, per declared input, the Unit
// attribute field whose value fills it; a constant of the same input is the fallback.
type TargetReference struct {
	InstanceID     uuid.UUID                      `json:"instanceId"`
	CapabilityKind string                         `json:"capabilityKind"`
	Revision       int64                          `json:"revision,string"`
	Inputs         map[string]processing.Value    `json:"inputs"`
	InputBindings  map[string]processing.FieldRef `json:"inputBindings,omitempty"`
}

// Subscription selects Domain event types for a subscriber and sends authorized, rendered
// input to its targets.
type Subscription struct {
	ID            uuid.UUID             `json:"id"`
	Revision      int64                 `json:"revision,string"`
	Kind          string                `json:"kind"`
	SubscriberID  uuid.UUID             `json:"subscriberId"`
	State         string                `json:"state"`
	EventTypes    []string              `json:"eventTypes"`
	Filter        *processing.Predicate `json:"filter,omitempty"`
	Targets       []TargetReference     `json:"targets"`
	LayoutID      *uuid.UUID            `json:"layoutId,omitempty"`
	SplitByUnit   bool                  `json:"splitByUnit"`
	MaxAgeSeconds *int                  `json:"maxAgeSeconds,omitempty"`
	// Health is HealthValid or HealthInvalid; HealthReason names why while invalid.
	Health       string  `json:"health"`
	HealthReason *string `json:"healthReason,omitempty"`
	// HealthChangedAt is when the health last changed; it is not part of the API view.
	HealthChangedAt *time.Time `json:"-"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	// CreatedBy is the account that created the Subscription; it is not part of the API view.
	CreatedBy uuid.UUID `json:"-"`
}

// SubscriptionInput is the writable part of a Subscription. SubscriberID is required for a
// global Subscription and must be empty for a personal one. A target's Revision may be zero or
// the instance's current revision.
type SubscriptionInput struct {
	Kind          string                `json:"kind"`
	SubscriberID  *uuid.UUID            `json:"subscriberId,omitempty"`
	State         string                `json:"state"`
	EventTypes    []string              `json:"eventTypes"`
	Filter        *processing.Predicate `json:"filter,omitempty"`
	Targets       []TargetReference     `json:"targets"`
	LayoutID      *uuid.UUID            `json:"layoutId,omitempty"`
	SplitByUnit   bool                  `json:"splitByUnit"`
	MaxAgeSeconds *int                  `json:"maxAgeSeconds,omitempty"`
}

// Delivery is the inspectable state of one Delivery. Input and authorization context are never
// part of it.
type Delivery struct {
	ID                   uuid.UUID  `json:"id"`
	Revision             int64      `json:"revision,string"`
	Origin               string     `json:"origin"`
	RedeliveryOf         *uuid.UUID `json:"redeliveryOf,omitempty"`
	SubscriptionID       uuid.UUID  `json:"subscriptionId"`
	SubscriptionRevision int64      `json:"subscriptionRevision,string"`
	SubscriberID         uuid.UUID  `json:"subscriberId"`
	DomainEventID        uuid.UUID  `json:"domainEventId"`
	EventType            string     `json:"eventType"`
	TargetInstanceID     uuid.UUID  `json:"targetInstanceId"`
	SplitKey             string     `json:"splitKey"`
	State                string     `json:"state"`
	OutcomeCode          *string    `json:"outcomeCode,omitempty"`
	AttemptCount         int        `json:"attemptCount"`
	LayoutID             *uuid.UUID `json:"layoutId,omitempty"`
	LayoutVersion        int64      `json:"layoutVersion,string"`
	MessageID            *uuid.UUID `json:"messageId,omitempty"`
	NextAttemptAt        *time.Time `json:"nextAttemptAt,omitempty"`
	ExpiresAt            time.Time  `json:"expiresAt"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	FinishedAt           *time.Time `json:"finishedAt,omitempty"`
	// Attempts is set on single-Delivery reads only.
	Attempts []Attempt `json:"attempts,omitempty"`
}

// Attempt is one recorded provider invocation of a Delivery.
type Attempt struct {
	Number           int             `json:"number"`
	ID               uuid.UUID       `json:"id"`
	State            string          `json:"state"`
	OutcomeCode      *string         `json:"outcomeCode,omitempty"`
	InstanceRevision int64           `json:"instanceRevision,string"`
	StartedAt        time.Time       `json:"startedAt"`
	FinishedAt       *time.Time      `json:"finishedAt,omitempty"`
	Receipt          json.RawMessage `json:"receipt,omitempty"`
}

// terminal reports whether a Delivery state is final.
func terminal(state string) bool {
	switch state {
	case Accepted, Failed, Uncertain, Expired, Cancelled:
		return true
	}
	return false
}
