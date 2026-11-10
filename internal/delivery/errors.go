package delivery

import (
	"errors"

	"github.com/google/uuid"
)

// ErrDependencyUnavailable reports that a redelivery cannot be prepared because what it needs
// is gone or no longer readable for the subscriber: the original input ceiling has passed, the
// Domain event was removed, or the data is not visible any more.
var ErrDependencyUnavailable = errors.New("the data or target a Delivery needs is no longer available")

// ErrExternalUncertain reports a retry request for a Delivery whose external effect is
// uncertain; only a confirmed redelivery may send it again.
var ErrExternalUncertain = errors.New("the external outcome of this Delivery is uncertain")

// DependencyError is an ErrDependencyUnavailable that names the safe kind of the missing
// dependency: input-ceiling, subscription, domain-event, data or input.
type DependencyError struct {
	Dependency string
}

// Error returns the message of ErrDependencyUnavailable.
func (e *DependencyError) Error() string { return ErrDependencyUnavailable.Error() }

// Is matches ErrDependencyUnavailable.
func (e *DependencyError) Is(target error) bool { return target == ErrDependencyUnavailable }

// UncertainError is an ErrExternalUncertain that names the Delivery whose outcome is uncertain.
type UncertainError struct {
	DeliveryID uuid.UUID
}

// Error returns the message of ErrExternalUncertain.
func (e *UncertainError) Error() string { return ErrExternalUncertain.Error() }

// Is matches ErrExternalUncertain.
func (e *UncertainError) Is(target error) bool { return target == ErrExternalUncertain }

// TargetUnavailableError rejects a Subscription target whose instance is missing, disabled or
// without a provider. Its failure code is AUTHORITY_UNAVAILABLE; the target status is
// "unavailable".
type TargetUnavailableError struct {
	InstanceID uuid.UUID
}

// Error returns a message without configuration details.
func (e *TargetUnavailableError) Error() string { return "delivery target unavailable" }

// Unwrap exposes the failure code to platform.FailureCode.
func (e *TargetUnavailableError) Unwrap() error { return ErrTargetUnavailable }

// Status is the target status reported with the error.
func (e *TargetUnavailableError) Status() string { return "unavailable" }
