package delivery

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// Default sending admission limits per one-minute window.
const (
	// DefaultSubscriberSendsPerMinute bounds the sends and send requests of one subscriber.
	DefaultSubscriberSendsPerMinute = 60
	// DefaultTargetSendsPerMinute bounds the sends to one target instance; a smaller limit in
	// the instance's descriptor lowers it.
	DefaultTargetSendsPerMinute = 600
)

// AdmissionLimits are the configured sending limits per minute.
type AdmissionLimits struct {
	Subscriber int
	Target     int
}

// normalized replaces non-positive limits by the defaults.
func (l AdmissionLimits) normalized() AdmissionLimits {
	if l.Subscriber <= 0 {
		l.Subscriber = DefaultSubscriberSendsPerMinute
	}
	if l.Target <= 0 {
		l.Target = DefaultTargetSendsPerMinute
	}
	return l
}

// admission counts sends in fixed one-minute windows, keyed by a hash of the subscriber or the
// target instance, never by a network location.
type admission struct {
	store  Store
	limits AdmissionLimits
}

func sendBucket(kind string, id uuid.UUID) [32]byte {
	return sha256.Sum256([]byte("delivery-send\x00" + kind + "\x00" + id.String()))
}

// window returns the start of the admission window of now and of the next one.
func window(now time.Time) (time.Time, time.Time) {
	start := now.UTC().Truncate(time.Minute)
	return start, start.Add(time.Minute)
}

// takeSubscriber counts one send request of the subscriber or returns *identity.RateLimited.
func (a admission) takeSubscriber(ctx context.Context, tx platform.TransactionContext, subscriber uuid.UUID, now time.Time) error {
	start, next := window(now)
	ok, err := a.store.TakeSendAttempt(ctx, tx, sendBucket("subscriber", subscriber), start, a.limits.Subscriber)
	if err != nil {
		return err
	}
	if !ok {
		return &identity.RateLimited{RetryAfter: next.Sub(now)}
	}
	return nil
}

// takeSend counts one background send against the subscriber and the target instance. It
// reports false and the start of the next window when either bucket is full; then neither
// bucket keeps the count.
func (a admission) takeSend(ctx context.Context, tx platform.TransactionContext, subscriber, instance uuid.UUID,
	descriptorLimit int, now time.Time) (bool, time.Time, error) {
	start, next := window(now)
	if subscriber == uuid.Nil || instance == uuid.Nil {
		return false, time.Time{}, authz.ErrInvalidInput
	}
	subscriberBucket := sendBucket("subscriber", subscriber)
	ok, err := a.store.TakeSendAttempt(ctx, tx, subscriberBucket, start, a.limits.Subscriber)
	if err != nil || !ok {
		return false, next, err
	}
	limit := a.limits.Target
	if descriptorLimit > 0 && descriptorLimit < limit {
		limit = descriptorLimit
	}
	ok, err = a.store.TakeSendAttempt(ctx, tx, sendBucket("target", instance), start, limit)
	if err != nil {
		return false, next, err
	}
	if !ok {
		return false, next, a.store.ReleaseSendAttempt(ctx, tx, subscriberBucket, start)
	}
	return true, next, nil
}
