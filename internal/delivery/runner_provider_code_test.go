package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

// finishStore records what the runner writes when it finishes an attempt.
type finishStore struct {
	Store
	attempts []*string
	saved    []DeliveryRecord
}

func (s *finishStore) FinishAttempt(_ context.Context, _ platform.TransactionContext, _ Claim, _ uuid.UUID, _ int,
	_, _ string, providerCode *string, _ json.RawMessage, _ time.Time) error {
	s.attempts = append(s.attempts, providerCode)
	return nil
}

func (s *finishStore) SaveClaimed(_ context.Context, _ platform.TransactionContext, _ Claim, record DeliveryRecord, _ time.Time) error {
	s.saved = append(s.saved, record)
	return nil
}

// fixedClock answers one instant.
type fixedClock struct{ now time.Time }

func (c fixedClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return c.now, nil
}

func finishWith(t *testing.T, store *finishStore, record DeliveryRecord, result AttemptResult, callErr error) DeliveryRecord {
	t.Helper()
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	runner := &Runner{deps: Dependencies{Store: store}, clock: fixedClock{now}}
	record.ExpiresAt = now.Add(time.Hour)
	record.AttemptCount = 1
	if err := runner.finish(t.Context(), nil, Claim{}, &started{attempt: record.AttemptCount, record: record}, result, callErr); err != nil {
		t.Fatal(err)
	}
	return store.saved[len(store.saved)-1]
}

func codeOf(code *string) string {
	if code == nil {
		return "<nil>"
	}
	return *code
}

// TestFinishStoresTheProviderCodeOnAttemptAndDelivery checks that a safe provider code is kept next
// to the enumerated outcome.
func TestFinishStoresTheProviderCodeOnAttemptAndDelivery(t *testing.T) {
	t.Parallel()
	store := &finishStore{}
	record := finishWith(t, store, DeliveryRecord{}, AttemptResult{Status: StatusPermanent, Code: "recipient-missing"}, nil)
	if record.State != Failed || codeOf(record.OutcomeCode) != CodePermanent ||
		codeOf(record.ProviderCode) != "recipient-missing" || codeOf(store.attempts[0]) != "recipient-missing" {
		t.Fatalf("state %s outcome %s delivery code %s attempt code %s", record.State, codeOf(record.OutcomeCode),
			codeOf(record.ProviderCode), codeOf(store.attempts[0]))
	}
}

// TestFinishDropsUnsafeOrUnreliableProviderCodes checks that an invalid code or a code that comes
// with a call error is not stored.
func TestFinishDropsUnsafeOrUnreliableProviderCodes(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		result  AttemptResult
		callErr error
	}{
		"invalid characters": {AttemptResult{Status: StatusPermanent, Code: "Bad Code!"}, nil},
		"too long":           {AttemptResult{Status: StatusPermanent, Code: strings.Repeat("a", 65)}, nil},
		"call error":         {AttemptResult{Status: StatusPermanent, Code: "recipient-missing"}, errors.New("broken")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := &finishStore{}
			previous := "rate-limited"
			record := finishWith(t, store, DeliveryRecord{Delivery: Delivery{ProviderCode: &previous}}, test.result, test.callErr)
			if record.ProviderCode != nil || store.attempts[0] != nil {
				t.Fatalf("delivery code %s attempt code %s", codeOf(record.ProviderCode), codeOf(store.attempts[0]))
			}
		})
	}
}

// TestFinishClearsTheDeliveryCodeWhenALaterAttemptHasNone checks that the Delivery shows the code
// of its latest finished attempt while the earlier attempt keeps its own.
func TestFinishClearsTheDeliveryCodeWhenALaterAttemptHasNone(t *testing.T) {
	t.Parallel()
	store := &finishStore{}
	record := finishWith(t, store, DeliveryRecord{}, AttemptResult{Status: StatusTransient, Code: "rate-limited"}, nil)
	if record.State != RetryPending || codeOf(record.ProviderCode) != "rate-limited" {
		t.Fatalf("state %s code %s", record.State, codeOf(record.ProviderCode))
	}
	record = finishWith(t, store, record, AttemptResult{Status: StatusAccepted}, nil)
	if record.State != Accepted || record.ProviderCode != nil {
		t.Fatalf("state %s code %s", record.State, codeOf(record.ProviderCode))
	}
	if codeOf(store.attempts[0]) != "rate-limited" || store.attempts[1] != nil {
		t.Fatalf("attempt codes %s, %s", codeOf(store.attempts[0]), codeOf(store.attempts[1]))
	}
}
