package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// scriptedClock returns the given readings in order and fails the test when it runs out.
func scriptedClock(t *testing.T, readings ...time.Time) func(context.Context) (time.Time, error) {
	t.Helper()
	next := 0
	return func(context.Context) (time.Time, error) {
		if next >= len(readings) {
			t.Fatal("the clock was read more often than scripted")
		}
		next++
		return readings[next-1], nil
	}
}

func TestRunInOneAdmissionWindowDiscardsACrossingAttemptAndRetries(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Attempt 0 starts at 12:00:59 and ends at 12:01:01; attempt 1 stays inside 12:01.
	now := scriptedClock(t, base.Add(59*time.Second), base.Add(61*time.Second),
		base.Add(62*time.Second), base.Add(63*time.Second))
	var ran []int
	failure := errors.New("assertion of the crossing attempt")
	err := runInOneAdmissionWindow(t.Context(), now, func(attempt int) error {
		ran = append(ran, attempt)
		if attempt == 0 {
			return failure
		}
		return nil
	})
	if err != nil {
		t.Fatalf("the failure of the crossing attempt was applied: %v", err)
	}
	if len(ran) != 2 || ran[0] != 0 || ran[1] != 1 {
		t.Fatalf("attempts run = %v, want [0 1]", ran)
	}
}

func TestRunInOneAdmissionWindowAppliesTheErrorOfAnAttemptInsideOneWindow(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	failure := errors.New("assertion")
	err := runInOneAdmissionWindow(t.Context(), scriptedClock(t, base, base.Add(59*time.Second)),
		func(int) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the sequence's error", err)
	}
}

func TestRunInOneAdmissionWindowFailsWhenEveryAttemptCrosses(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 59, 0, time.UTC)
	var readings []time.Time
	for i := range admissionWindowAttempts {
		start := base.Add(time.Duration(i) * time.Minute)
		readings = append(readings, start, start.Add(2*time.Second))
	}
	runs := 0
	err := runInOneAdmissionWindow(t.Context(), scriptedClock(t, readings...), func(int) error {
		runs++
		return nil
	})
	if !errors.Is(err, errAdmissionWindowCrossed) || runs != admissionWindowAttempts {
		t.Fatalf("err = %v after %d runs, want errAdmissionWindowCrossed after %d", err, runs, admissionWindowAttempts)
	}
}
