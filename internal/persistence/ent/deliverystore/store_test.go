package deliverystore_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/persistence/ent/deliverystore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

func TestMain(m *testing.M) { os.Exit(testpostgres.Run(m)) }

var migratedTemplate = testpostgres.NewTemplate("deliverystore", func(ctx context.Context, dsn string) error {
	store, _, err := postgres.NewStore(dsn, true)
	if err != nil {
		return err
	}
	migrateErr := store.Migrate(ctx, migrations.Files, platform.MigrationApply)
	if err := store.Close(); err != nil && migrateErr == nil {
		return err
	}
	return migrateErr
})

// world is a migrated database with one Subscription of a subscriber.
type world struct {
	t            *testing.T
	primary      *postgres.Store
	store        *deliverystore.Store
	subscriber   uuid.UUID
	subscription delivery.Subscription
	instance     uuid.UUID
}

func newWorld(t *testing.T) *world {
	t.Helper()
	primary, _, err := postgres.NewStore(migratedTemplate.Database(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := primary.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := deliverystore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, primary: primary, store: store, subscriber: uuid.New(), instance: uuid.New()}
	w.saveInstance()
	now := w.now()
	w.subscription = delivery.Subscription{ID: uuid.New(), Revision: 1, Kind: delivery.KindPersonal, SubscriberID: w.subscriber,
		State: delivery.StateActive, EventTypes: []string{"message.created"}, CreatedBy: w.subscriber,
		Targets:   []delivery.TargetReference{{InstanceID: w.instance, CapabilityKind: delivery.CapabilityDelivery, Revision: 1}},
		CreatedAt: now, UpdatedAt: now}
	w.within(func(tx platform.TransactionContext) error {
		return store.InsertSubscription(tx.Context(), tx, w.subscription)
	})
	return w
}

// saveInstance stores the target instance through the instance store.
func (w *world) saveInstance() {
	w.t.Helper()
	instances, err := processingstore.New(w.primary)
	if err != nil {
		w.t.Fatal(err)
	}
	actor := uuid.New()
	authority := authz.Authority{PrincipalID: actor.String(), Revision: 1, Active: true,
		Grants: []authz.Grant{{SubjectID: actor.String(), SubjectKind: "principal", Scope: authz.Scope{Kind: "all"},
			Actions: []string{"plugin.configure"}}}}
	config, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
	if err != nil {
		w.t.Fatal(err)
	}
	value := processing.InstanceConfiguration{ID: w.instance, Revision: 1, Enabled: true, ExecutionPrincipalID: uuid.New(),
		CreatedBy: actor, Configuration: config, Main: processing.ProvisionedArtifact{ManifestPath: "target/manifest.json",
			Pin: processing.ArtifactPin{PluginID: "target", Version: "1", CapabilityID: "deliver", Digest: strings.Repeat("d", 64)}}}
	w.within(func(tx platform.TransactionContext) error {
		now, err := w.primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		value.CreatedAt = now
		return instances.SaveInstance(tx.Context(), tx, authority, value, 0)
	})
}

func (w *world) within(operation func(tx platform.TransactionContext) error) {
	w.t.Helper()
	if err := w.primary.WithinTransaction(context.Background(), operation); err != nil {
		w.t.Fatalf("%v: %v", err, errors.Unwrap(err))
	}
}

func (w *world) now() time.Time {
	w.t.Helper()
	var now time.Time
	w.within(func(tx platform.TransactionContext) error {
		var err error
		now, err = w.primary.Now(tx.Context(), tx)
		return err
	})
	return now
}

// prepared inserts a prepared automatic Delivery for a new Domain event.
func (w *world) prepared(now time.Time) delivery.DeliveryRecord {
	w.t.Helper()
	record := delivery.DeliveryRecord{Delivery: delivery.Delivery{ID: uuid.New(), Revision: 1, Origin: delivery.OriginAutomatic,
		SubscriptionID: w.subscription.ID, SubscriptionRevision: 1, SubscriberID: w.subscriber, DomainEventID: uuid.New(),
		EventType: "message.created", TargetInstanceID: w.instance, State: delivery.Prepared,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now},
		Input:     &delivery.DeliveryInput{OutputKind: "text", Body: "x"},
		AgeAnchor: now, AccessRevision: 1, InstanceRevision: 1, CausationID: uuid.New(), Due: now}
	w.within(func(tx platform.TransactionContext) error {
		inserted, err := w.store.InsertDelivery(tx.Context(), tx, record)
		if err == nil && !inserted {
			err = errors.New("not inserted")
		}
		return err
	})
	return record
}

// TestClaimsAreFencedByTokenAndLease proves that a claim is exclusive, that a write with a
// superseded token or an expired lease is rejected, and that an expired started claim becomes
// uncertain with lease-lost.
func TestClaimsAreFencedByTokenAndLease(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	now := w.now()
	record := w.prepared(now)
	var first []delivery.DeliveryRecord
	w.within(func(tx platform.TransactionContext) error {
		var err error
		first, err = w.store.ClaimDue(tx.Context(), tx, now, delivery.LeaseDuration, 10, nil)
		return err
	})
	if len(first) != 1 || first[0].ID != record.ID || first[0].ClaimToken != 1 {
		t.Fatalf("first claim: %+v", first)
	}
	w.within(func(tx platform.TransactionContext) error {
		again, err := w.store.ClaimDue(tx.Context(), tx, now, delivery.LeaseDuration, 10, nil)
		if err == nil && len(again) != 0 {
			err = errors.New("claimed twice under a live lease")
		}
		return err
	})
	other := []uuid.UUID{uuid.New()}
	w.within(func(tx platform.TransactionContext) error {
		filtered, err := w.store.ClaimDue(tx.Context(), tx, now.Add(2*delivery.LeaseDuration), delivery.LeaseDuration, 10, other)
		if err == nil && len(filtered) != 0 {
			err = errors.New("instance filter ignored")
		}
		return err
	})
	// The lease expired: another worker claims with a new token; the old token is superseded.
	later := now.Add(2 * delivery.LeaseDuration)
	var second []delivery.DeliveryRecord
	w.within(func(tx platform.TransactionContext) error {
		var err error
		second, err = w.store.ClaimDue(tx.Context(), tx, later, delivery.LeaseDuration, 10, []uuid.UUID{w.instance})
		return err
	})
	if len(second) != 1 || second[0].ClaimToken != 2 {
		t.Fatalf("second claim: %+v", second)
	}
	stale := delivery.Claim{DeliveryID: record.ID, Token: 1}
	err := w.primary.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		return w.store.SaveClaimed(tx.Context(), tx, stale, second[0], later)
	})
	if !errors.Is(err, delivery.ErrLeaseLost) {
		t.Fatalf("superseded token wrote: %v", err)
	}
	current := delivery.Claim{DeliveryID: record.ID, Token: 2}
	started := second[0]
	started.State, started.AttemptCount = delivery.Started, 1
	w.within(func(tx platform.TransactionContext) error {
		if err := w.store.InsertAttempt(tx.Context(), tx, current, record.ID, delivery.AttemptRecord{
			Attempt: delivery.Attempt{Number: 1, ID: uuid.New(), InstanceRevision: 1, StartedAt: later}, AccessRevision: 1}, later); err != nil {
			return err
		}
		return w.store.SaveClaimed(tx.Context(), tx, current, started, later)
	})
	expired := later.Add(2 * delivery.LeaseDuration)
	err = w.primary.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		return w.store.Renew(tx.Context(), tx, current, expired, delivery.LeaseDuration)
	})
	if !errors.Is(err, delivery.ErrLeaseLost) {
		t.Fatalf("expired lease renewed: %v", err)
	}
	w.within(func(tx platform.TransactionContext) error {
		ended, err := w.store.ReclaimLost(tx.Context(), tx, expired, 10)
		if err == nil && ended != 1 {
			err = errors.New("lost start not reclaimed")
		}
		return err
	})
	w.within(func(tx platform.TransactionContext) error {
		stored, err := w.store.Delivery(tx.Context(), tx, record.ID, false)
		if err != nil {
			return err
		}
		attempts, err := w.store.Attempts(tx.Context(), tx, record.ID)
		if err != nil {
			return err
		}
		if stored.State != delivery.Uncertain || stored.OutcomeCode == nil || *stored.OutcomeCode != delivery.CodeLeaseLost ||
			stored.Input != nil || len(attempts) != 1 || attempts[0].State != delivery.Uncertain {
			return errors.New("lost start not recorded as uncertain")
		}
		return nil
	})
}

// TestAutomaticIdentityIsUniqueAndRedeliveryIsNot proves that one automatic Delivery exists per
// Subscription, Domain event, target and split, while a redelivery of the same identity is a new
// Delivery.
func TestAutomaticIdentityIsUniqueAndRedeliveryIsNot(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	now := w.now()
	original := w.prepared(now)
	duplicate := original
	duplicate.ID = uuid.New()
	redelivery := original
	redelivery.ID, redelivery.Origin, redelivery.RedeliveryOf = uuid.New(), delivery.OriginRedelivery, &original.ID
	w.within(func(tx platform.TransactionContext) error {
		inserted, err := w.store.InsertDelivery(tx.Context(), tx, duplicate)
		if err != nil || inserted {
			return errors.Join(err, errors.New("duplicate automatic Delivery stored"))
		}
		inserted, err = w.store.InsertDelivery(tx.Context(), tx, redelivery)
		if err != nil || !inserted {
			return errors.Join(err, errors.New("redelivery not stored"))
		}
		return nil
	})
}

// TestDeleteAccountDataRemovesTheSubscribersRows proves that account deletion removes the
// account's Subscriptions, Deliveries and attempts and nothing of another account.
func TestDeleteAccountDataRemovesTheSubscribersRows(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	now := w.now()
	mine := w.prepared(now)
	other := delivery.Subscription{ID: uuid.New(), Revision: 1, Kind: delivery.KindPersonal, SubscriberID: uuid.New(),
		State: delivery.StateActive, EventTypes: []string{"message.created"}, CreatedBy: uuid.New(),
		Targets:   []delivery.TargetReference{{InstanceID: w.instance, CapabilityKind: delivery.CapabilityDelivery, Revision: 1}},
		CreatedAt: now, UpdatedAt: now}
	w.within(func(tx platform.TransactionContext) error {
		if err := w.store.InsertSubscription(tx.Context(), tx, other); err != nil {
			return err
		}
		return w.store.DeleteAccountData(tx.Context(), tx, w.subscriber)
	})
	w.within(func(tx platform.TransactionContext) error {
		if _, err := w.store.Subscription(tx.Context(), tx, w.subscription.ID, delivery.NoLock); !errors.Is(err, authz.ErrNotFound) {
			return errors.Join(err, errors.New("subscription kept"))
		}
		if _, err := w.store.Delivery(tx.Context(), tx, mine.ID, false); !errors.Is(err, authz.ErrNotFound) {
			return errors.Join(err, errors.New("delivery kept"))
		}
		_, err := w.store.Subscription(tx.Context(), tx, other.ID, delivery.NoLock)
		return err
	})
}

// TestTakeSendAttemptSweepsExpiredWindowsOfOtherBuckets proves that admitted sends delete
// expired windows of buckets that never send again, a bounded number per send, and keep current
// windows of other buckets.
func TestTakeSendAttemptSweepsExpiredWindowsOfOtherBuckets(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	window := w.now().UTC().Truncate(time.Minute)
	current := sha256.Sum256([]byte("current"))
	w.within(func(tx platform.TransactionContext) error {
		executor, err := w.primary.Resolve(tx)
		if err != nil {
			return err
		}
		for i := range 150 {
			bucket := sha256.Sum256([]byte{byte(i), byte(i >> 8)})
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO delivery_admissions (bucket, window_start, count)
				VALUES ($1, $2, 1)`, bucket[:], window.Add(-time.Hour)); err != nil {
				return err
			}
		}
		_, err = w.store.TakeSendAttempt(tx.Context(), tx, current, window, 10)
		return err
	})
	count := func() int {
		var expired int
		w.within(func(tx platform.TransactionContext) error {
			executor, err := w.primary.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM delivery_admissions WHERE window_start < $1`,
				window).Scan(&expired)
		})
		return expired
	}
	if expired := count(); expired != 50 {
		t.Fatalf("after one admitted send %d expired windows remain, want 50", expired)
	}
	other := sha256.Sum256([]byte("other"))
	w.within(func(tx platform.TransactionContext) error {
		ok, err := w.store.TakeSendAttempt(tx.Context(), tx, other, window, 10)
		if err == nil && !ok {
			err = errors.New("send not admitted")
		}
		return err
	})
	if expired := count(); expired != 0 {
		t.Fatalf("after two admitted sends %d expired windows remain", expired)
	}
	w.within(func(tx platform.TransactionContext) error {
		ok, err := w.store.TakeSendAttempt(tx.Context(), tx, current, window, 2)
		if err == nil && !ok {
			err = errors.New("current window of the first bucket was swept")
		}
		return err
	})
}

// TestReleaseSendAttemptTakesBackExactlyOneUnit proves that releasing a send takes back one unit
// of the window and removes the row only with its last unit.
func TestReleaseSendAttemptTakesBackExactlyOneUnit(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	window := w.now().UTC().Truncate(time.Minute)
	bucket := sha256.Sum256([]byte("release"))
	take := func() bool {
		var ok bool
		w.within(func(tx platform.TransactionContext) error {
			var err error
			ok, err = w.store.TakeSendAttempt(tx.Context(), tx, bucket, window, 2)
			return err
		})
		return ok
	}
	release := func() {
		w.within(func(tx platform.TransactionContext) error {
			return w.store.ReleaseSendAttempt(tx.Context(), tx, bucket, window)
		})
	}
	if first, second := take(), take(); !first || !second {
		t.Fatal("two sends within the limit were refused")
	}
	release()
	if !take() {
		t.Fatal("a released unit was not available again")
	}
	if take() {
		t.Fatal("releasing one unit erased the other admitted send")
	}
	release()
	release()
	if first, second, third := take(), take(), take(); !first || !second || third {
		t.Fatal("releasing the last units did not restore exactly the limit")
	}
}

// TestSubscriptionWithAVanishedLayoutIsInvalidInput proves that a Subscription naming a layout
// removed after it was checked is rejected as invalid input, not as a storage failure.
func TestSubscriptionWithAVanishedLayoutIsInvalidInput(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	now := w.now()
	gone := uuid.New()
	subscription := w.subscription
	subscription.ID, subscription.LayoutID, subscription.CreatedAt, subscription.UpdatedAt = uuid.New(), &gone, now, now
	err := w.primary.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		return w.store.InsertSubscription(tx.Context(), tx, subscription)
	})
	if !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("insert with a vanished layout: %v", err)
	}
	updated := w.subscription
	updated.Revision, updated.LayoutID = 2, &gone
	err = w.primary.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		return w.store.UpdateSubscription(tx.Context(), tx, updated, 1)
	})
	if !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("update with a vanished layout: %v", err)
	}
}

// TestSubscriptionHealthIsRecordedWithoutARevisionAndHidesTheSubscriptionFromFanOut proves that
// marking a Subscription invalid keeps its revision, reports a change only once, removes it from
// the active listing, and that a save makes it valid again.
func TestSubscriptionHealthIsRecordedWithoutARevisionAndHidesTheSubscriptionFromFanOut(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	now := w.now()
	var changed, repeated, other bool
	w.within(func(tx platform.TransactionContext) error {
		var err error
		if changed, err = w.store.SetSubscriptionHealth(tx.Context(), tx, w.subscription.ID, delivery.HealthInvalid,
			delivery.ReasonFieldUnavailable, now); err != nil {
			return err
		}
		if repeated, err = w.store.SetSubscriptionHealth(tx.Context(), tx, w.subscription.ID, delivery.HealthInvalid,
			delivery.ReasonFieldUnavailable, now); err != nil {
			return err
		}
		other, err = w.store.SetSubscriptionHealth(tx.Context(), tx, w.subscription.ID, delivery.HealthInvalid,
			delivery.ReasonLayoutInvalid, now)
		return err
	})
	if !changed || repeated || !other {
		t.Fatalf("changed = %v, repeated = %v, other reason = %v", changed, repeated, other)
	}
	w.within(func(tx platform.TransactionContext) error {
		stored, err := w.store.Subscription(tx.Context(), tx, w.subscription.ID, delivery.NoLock)
		if err != nil {
			return err
		}
		if stored.Revision != 1 || stored.Health != delivery.HealthInvalid || stored.HealthReason == nil ||
			*stored.HealthReason != delivery.ReasonLayoutInvalid || stored.HealthChangedAt == nil {
			t.Errorf("stored = %+v", stored)
		}
		active, err := w.store.ActiveSubscriptions(tx.Context(), tx, "message.created", now.Add(time.Hour))
		if err != nil || len(active) != 0 {
			t.Errorf("active = %+v, %v", active, err)
		}
		return nil
	})

	saved := w.subscription
	saved.Revision, saved.UpdatedAt = 2, now
	w.within(func(tx platform.TransactionContext) error {
		return w.store.UpdateSubscription(tx.Context(), tx, saved, 1)
	})
	w.within(func(tx platform.TransactionContext) error {
		stored, err := w.store.Subscription(tx.Context(), tx, w.subscription.ID, delivery.NoLock)
		if err != nil {
			return err
		}
		if stored.Health != delivery.HealthValid || stored.HealthReason != nil {
			t.Errorf("stored after save = %+v", stored)
		}
		active, err := w.store.ActiveSubscriptions(tx.Context(), tx, "message.created", now.Add(time.Hour))
		if err != nil || len(active) != 1 {
			t.Errorf("active after save = %+v, %v", active, err)
		}
		return nil
	})
}

// TestTargetInputBindingsRoundTrip proves that a target's input bindings are stored with the
// Subscription and read back, and that a target without bindings reads back none.
func TestTargetInputBindingsRoundTrip(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	bindings := map[string]processing.FieldRef{
		"recipient": {Namespace: "unit", Path: []string{"attributes", "contact", "pager"}}}
	updated := w.subscription
	updated.Revision = 2
	updated.Targets = []delivery.TargetReference{{InstanceID: w.instance, CapabilityKind: delivery.CapabilityDelivery,
		Revision: 1, InputBindings: bindings}}
	w.within(func(tx platform.TransactionContext) error {
		return w.store.UpdateSubscription(tx.Context(), tx, updated, 1)
	})
	w.within(func(tx platform.TransactionContext) error {
		stored, err := w.store.Subscription(tx.Context(), tx, w.subscription.ID, delivery.NoLock)
		if err != nil {
			return err
		}
		if len(stored.Targets) != 1 || !reflect.DeepEqual(stored.Targets[0].InputBindings, bindings) {
			t.Errorf("stored bindings = %+v", stored.Targets)
		}
		return nil
	})
	plain := updated
	plain.Revision, plain.Targets = 3, []delivery.TargetReference{{InstanceID: w.instance, CapabilityKind: delivery.CapabilityDelivery, Revision: 1}}
	w.within(func(tx platform.TransactionContext) error {
		return w.store.UpdateSubscription(tx.Context(), tx, plain, 2)
	})
	w.within(func(tx platform.TransactionContext) error {
		stored, err := w.store.Subscription(tx.Context(), tx, w.subscription.ID, delivery.NoLock)
		if err != nil {
			return err
		}
		if len(stored.Targets[0].InputBindings) != 0 {
			t.Errorf("bindings after a save without any = %+v", stored.Targets[0].InputBindings)
		}
		return nil
	})
}
