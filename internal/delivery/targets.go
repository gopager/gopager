package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// DescriptorVersion and InvocationVersion are the versions of the target descriptor and of the
// attempt invocation this Core produces and accepts.
const (
	DescriptorVersion = 1
	InvocationVersion = 1
)

// InputField declares one typed input a target accepts: a unique name and a scalar kind (string,
// boolean, integer, number, timestamp or bytes). MaxBytes bounds the encoded value; zero means
// the Delivery body bound.
type InputField struct {
	Name     string               `json:"name"`
	Kind     processing.ValueKind `json:"kind"`
	Required bool                 `json:"required"`
	MaxBytes int                  `json:"maxBytes,omitempty"`
}

// TargetDependency classifies something a target needs at run time, for example `network` or
// `secret`, with a description that reveals no Secret or destination.
type TargetDependency struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// TargetDescriptor is what a configured instance offers as a Delivery target at one revision:
// the output kinds a layout may render for it, its typed inputs, its classified dependencies and
// an optional own sending limit. ConfigurationAuthority names who configured it (`admin`).
type TargetDescriptor struct {
	Version                int                `json:"version"`
	InstanceID             uuid.UUID          `json:"instanceId"`
	InstanceRevision       int64              `json:"instanceRevision,string"`
	CapabilityKind         string             `json:"capabilityKind"`
	ConfigurationAuthority string             `json:"configurationAuthority"`
	OutputKinds            []string           `json:"outputKinds"`
	Inputs                 []InputField       `json:"inputs"`
	Dependencies           []TargetDependency `json:"dependencies"`
	// SendsPerMinute lowers the per-instance sending admission when positive.
	SendsPerMinute int `json:"sendsPerMinute,omitempty"`
	// Idempotent reports that the provider deduplicates by the invocation's Delivery ID.
	Idempotent bool `json:"idempotent"`
}

// validate checks that the descriptor answers for exactly this instance revision.
func (d TargetDescriptor) validate(instance processing.InstanceConfiguration) error {
	if d.Version != DescriptorVersion || d.InstanceID != instance.ID || d.InstanceRevision != instance.Revision ||
		d.CapabilityKind != CapabilityDelivery || d.ConfigurationAuthority == "" || len(d.OutputKinds) == 0 ||
		d.SendsPerMinute < 0 {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("target descriptor does not match the instance"))
	}
	for _, kind := range d.OutputKinds {
		if !slices.Contains([]string{processing.LayoutText, processing.LayoutMarkdown, processing.LayoutJSON}, kind) {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("unknown target output kind"))
		}
	}
	names := map[string]bool{}
	for _, field := range d.Inputs {
		if field.Name == "" || names[field.Name] || !slices.Contains(scalarInputKinds, field.Kind) || field.MaxBytes < 0 {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid target input declaration"))
		}
		names[field.Name] = true
	}
	return nil
}

// scalarInputKinds are the value kinds a descriptor input may declare. A kind alone cannot
// describe the elements or properties of a list or object, so composite inputs are not offered.
var scalarInputKinds = []processing.ValueKind{processing.StringValue, processing.BooleanValue, processing.IntegerValue,
	processing.NumberValue, processing.TimestampValue, processing.BytesValue}

// input returns the declaration of a named input.
func (d TargetDescriptor) input(name string) (InputField, bool) {
	for _, field := range d.Inputs {
		if field.Name == name {
			return field, true
		}
	}
	return InputField{}, false
}

// DeliveryInput is the materialized, bounded input of a Delivery: the rendered body in one
// output kind and the typed inputs after precedence.
type DeliveryInput struct {
	OutputKind string                      `json:"outputKind"`
	Body       string                      `json:"body"`
	Inputs     map[string]processing.Value `json:"inputs"`
}

// AttemptInvocation is one provider call for a Delivery. DeliveryID is stable across attempts
// and serves as the idempotency key for providers that support one. SubscriberID is the
// Delivery's subscriber; the provider uses it to select that subscriber's personal configuration.
type AttemptInvocation struct {
	Version          int           `json:"version"`
	DeliveryID       uuid.UUID     `json:"deliveryId"`
	AttemptID        uuid.UUID     `json:"attemptId"`
	AttemptNumber    int           `json:"attemptNumber"`
	InstanceID       uuid.UUID     `json:"instanceId"`
	SubscriberID     uuid.UUID     `json:"subscriberId"`
	InstanceRevision int64         `json:"instanceRevision,string"`
	Input            DeliveryInput `json:"input"`
	// CorrelationID and CausationID come from the triggering fact; CausationID is the fact's own
	// ID when it records no causation.
	CorrelationID *uuid.UUID `json:"correlationId,omitempty"`
	CausationID   uuid.UUID  `json:"causationId"`
	Deadline      time.Time  `json:"deadline"`
}

// AttemptStatus is a provider's classification of one attempt.
type AttemptStatus string

// Attempt statuses.
const (
	// StatusAccepted means the provider accepted the input; it says nothing about reading.
	StatusAccepted AttemptStatus = "accepted"
	// StatusTransient means the provider did not perform the effect and a later attempt may succeed.
	StatusTransient AttemptStatus = "transient"
	// StatusPermanent means the provider did not perform the effect and never will.
	StatusPermanent AttemptStatus = "permanent"
	// StatusUncertain means the effect may or may not have happened.
	StatusUncertain AttemptStatus = "uncertain"
)

// AttemptResult is a provider's answer. Code is a safe classification without Secrets or input;
// Receipt is optional bounded JSON a provider deems safe to show.
type AttemptResult struct {
	Status     AttemptStatus   `json:"status"`
	Code       string          `json:"code,omitempty"`
	RetryAfter time.Duration   `json:"retryAfter,omitempty"`
	Receipt    json.RawMessage `json:"receipt,omitempty"`
}

// Targets is the Core port to Delivery target providers. Describe answers the descriptor of an
// instance revision, or a failure when no provider can serve it. Attempt performs one
// invocation; any error after the call was made is treated as an uncertain outcome.
type Targets interface {
	Describe(ctx context.Context, instance processing.InstanceConfiguration) (TargetDescriptor, error)
	Attempt(ctx context.Context, invocation AttemptInvocation) (AttemptResult, error)
}

// ErrTargetUnavailable reports that no provider can serve a target instance.
var ErrTargetUnavailable error = platform.NewFailure(platform.CodeAuthorityUnavailable,
	errors.New("delivery target unavailable"))

// UnavailableTargets is the Targets of a process without a target provider: every instance is
// unavailable, so no Subscription can select a target and no Delivery can start.
type UnavailableTargets struct{}

// Describe implements Targets and always fails with ErrTargetUnavailable.
func (UnavailableTargets) Describe(context.Context, processing.InstanceConfiguration) (TargetDescriptor, error) {
	return TargetDescriptor{}, ErrTargetUnavailable
}

// Attempt implements Targets and always fails with ErrTargetUnavailable.
func (UnavailableTargets) Attempt(context.Context, AttemptInvocation) (AttemptResult, error) {
	return AttemptResult{}, ErrTargetUnavailable
}
