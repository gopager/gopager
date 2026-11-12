package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/readmodel"
)

// admissionWindowAttempts is the number of times a window-dependent sequence is run before the
// test fails because a window boundary interrupted every run.
const admissionWindowAttempts = 3

// errAdmissionWindowCrossed reports that every attempt of a sequence straddled a window boundary.
var errAdmissionWindowCrossed = errors.New("the sequence crossed an admission window boundary on every attempt")

// newAdmissionClock returns a reader of the primary database's clock, read the way the
// admissions read it: through Store.Now, which evaluates clock_timestamp() on the primary. The
// store may belong to any database of the same server, because the clock is the server's.
func newAdmissionClock(store *postgres.Store) func(context.Context) (time.Time, error) {
	return func(ctx context.Context) (time.Time, error) {
		var now time.Time
		err := store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			var err error
			now, err = store.Now(tx.Context(), tx)
			return err
		})
		return now, err
	}
}

// newAdmissionClockStore returns a store whose only purpose is to read the primary clock for
// withinOneAdmissionWindow, so a test can read it before it has built its own fixture.
func newAdmissionClockStore(t *testing.T) *postgres.Store {
	t.Helper()
	store, _, err := postgres.NewStore(migratedTemplate.Database(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

// runInOneAdmissionWindow runs sequence until one run starts and ends inside the same admission
// window and returns that run's error. Admissions count requests in fixed windows read from the
// primary clock, so a test that expects the request after N admitted ones to be refused gets
// the opposite when a window boundary falls between them. Freezing the clock is not an option
// because the same clock decides authorization. Instead the clock is read before the sequence
// and after its last request; only a run whose two readings fall into the same window is
// authoritative, and a run that crossed a boundary is discarded together with its error. Every
// run gets its attempt number (from 0) and must use state no earlier run touched, such as a new
// fixture or new accounts, so that no counter carries over.
//
// It fails with errAdmissionWindowCrossed after admissionWindowAttempts discarded runs and
// returns clock failures as they are.
func runInOneAdmissionWindow(ctx context.Context, now func(context.Context) (time.Time, error),
	sequence func(attempt int) error) error {
	for attempt := range admissionWindowAttempts {
		started, err := now(ctx)
		if err != nil {
			return fmt.Errorf("reading the clock before attempt %d: %w", attempt, err)
		}
		outcome := sequence(attempt)
		finished, err := now(ctx)
		if err != nil {
			return fmt.Errorf("reading the clock after attempt %d: %w", attempt, err)
		}
		if started.Truncate(readmodel.AdmissionWindow).Equal(finished.Truncate(readmodel.AdmissionWindow)) {
			return outcome
		}
	}
	return errAdmissionWindowCrossed
}

// withinOneAdmissionWindow is runInOneAdmissionWindow for a test: it fails the test with the
// error of the first run that stayed inside one admission window, or when none did. The
// sequence reports failures by returning an error, not through t, so that the failures of a
// discarded run are not applied.
func withinOneAdmissionWindow(t *testing.T, store *postgres.Store, sequence func(attempt int) error) {
	t.Helper()
	if err := runInOneAdmissionWindow(t.Context(), newAdmissionClock(store), sequence); err != nil {
		t.Fatal(err)
	}
}
