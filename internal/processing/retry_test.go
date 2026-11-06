package processing

import (
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

func TestProcessingRetryExactSymmetricBoundsPrimaryPrecisionAndExhaustion(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 1, 12, 0, 0, 123000, time.FixedZone("source-offset", 2*60*60))
	bounds := [][2]time.Duration{{24 * time.Second, 36 * time.Second},
		{96 * time.Second, 144 * time.Second}, {8 * time.Minute, 12 * time.Minute},
		{24 * time.Minute, 36 * time.Minute}}
	for i, interval := range bounds {
		for _, upper := range []bool{false, true} {
			due, retry, err := nextAutomaticRetry(i+1, now, func(choices int64) int64 {
				if upper {
					return choices - 1
				}
				return 0
			})
			expected := interval[0]
			if upper {
				expected = interval[1]
			}
			if err != nil || !retry || due.Sub(now) != expected || due.Location() != time.UTC ||
				due.Nanosecond()%1000 != 0 {
				t.Fatalf("attempt=%d upper=%v: due=%s retry=%v error=%v", i+1, upper, due, retry, err)
			}
		}
		for range 100 {
			due, retry, err := NextAutomaticRetry(i+1, now)
			if err != nil || !retry || due.Sub(now) < interval[0] || due.Sub(now) > interval[1] {
				t.Fatalf("real random schedule escaped attempt %d boundary: %s, %v", i+1, due, err)
			}
		}
	}
	called := false
	due, retry, err := nextAutomaticRetry(MaxAutomaticAttempts, now, func(int64) int64 {
		called = true
		return 0
	})
	if err != nil || retry || !due.IsZero() || called {
		t.Fatalf("exhausted work obtained another automatic attempt: %s %v %v", due, retry, err)
	}
}

func TestProcessingRetryRejectsInvalidAttemptsClockAndJitter(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	for _, attempts := range []int{-1, 0, MaxAutomaticAttempts + 1} {
		if _, _, err := NextAutomaticRetry(attempts, now); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("invalid attempts became terminal success: %d, %v", attempts, err)
		}
	}
	for _, timestamp := range []time.Time{{}, now.Add(time.Nanosecond)} {
		if _, _, err := NextAutomaticRetry(1, timestamp); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("invalid primary time became a schedule: %s, %v", timestamp, err)
		}
	}
	for _, choose := range []func(int64) int64{nil, func(int64) int64 { return -1 }, func(n int64) int64 { return n }} {
		if _, _, err := nextAutomaticRetry(1, now, choose); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("unbounded jitter became a schedule: %v", err)
		}
	}
}
