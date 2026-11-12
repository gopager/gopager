package deliverystore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
)

// TestProviderCodeRoundTripsOnDeliveryAndAttempt proves that the provider's safe code written when
// an attempt finishes is read back on the Delivery and its attempt, and that the columns accept
// only the safe shape.
func TestProviderCodeRoundTripsOnDeliveryAndAttempt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	now := w.now()
	record := w.prepared(now)
	var claimed []delivery.DeliveryRecord
	w.within(func(tx platform.TransactionContext) error {
		var err error
		claimed, err = w.store.ClaimDue(tx.Context(), tx, now, delivery.LeaseDuration, 10, delivery.ClaimFilter{})
		return err
	})
	claim := delivery.Claim{DeliveryID: record.ID, Token: claimed[0].ClaimToken}
	code := "recipient-missing"
	w.within(func(tx platform.TransactionContext) error {
		if err := w.store.InsertAttempt(tx.Context(), tx, claim, record.ID, delivery.AttemptRecord{
			Attempt: delivery.Attempt{Number: 1, ID: uuid.New(), InstanceRevision: 1, StartedAt: now}, AccessRevision: 1}, now); err != nil {
			return err
		}
		if err := w.store.FinishAttempt(tx.Context(), tx, claim, record.ID, 1, "permanent", delivery.CodePermanent, &code, nil, now); err != nil {
			return err
		}
		finished := claimed[0]
		finished.State, finished.AttemptCount, finished.Input = delivery.Failed, 1, nil
		permanent, finishedAt := delivery.CodePermanent, now
		finished.OutcomeCode, finished.FinishedAt, finished.ProviderCode = &permanent, &finishedAt, &code
		return w.store.SaveClaimed(tx.Context(), tx, claim, finished, now)
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
		if stored.ProviderCode == nil || *stored.ProviderCode != code || len(attempts) != 1 ||
			attempts[0].ProviderCode == nil || *attempts[0].ProviderCode != code {
			return errors.New("provider code not read back on the Delivery and the attempt")
		}
		return nil
	})
	for _, statement := range []string{
		`UPDATE deliveries SET provider_code = 'Bad Code!' WHERE id = $1`,
		`UPDATE delivery_attempts SET provider_code = 'Bad Code!' WHERE delivery_id = $1`,
	} {
		err := w.primary.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
			executor, err := w.primary.Resolve(tx)
			if err != nil {
				return err
			}
			_, err = executor.ExecContext(tx.Context(), statement, record.ID)
			return err
		})
		if err == nil {
			t.Fatalf("%s accepted an unsafe code", statement)
		}
	}
}
