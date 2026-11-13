package platform_test

import (
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

func TestBackoffDoublesFromItsBaseUpToItsLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		attempt     int
		base, limit time.Duration
		want        time.Duration
	}{
		{0, time.Second, 30 * time.Second, time.Second},
		{1, time.Second, 30 * time.Second, time.Second},
		{2, time.Second, 30 * time.Second, 2 * time.Second},
		{5, time.Second, 30 * time.Second, 16 * time.Second},
		{6, time.Second, 30 * time.Second, 30 * time.Second},
		{60, time.Second, 30 * time.Second, 30 * time.Second},
		// A base above the limit is never shortened.
		{3, time.Minute, 30 * time.Second, time.Minute},
	} {
		if got := platform.Backoff(tc.attempt, tc.base, tc.limit); got != tc.want {
			t.Errorf("Backoff(%d, %v, %v) = %v, want %v", tc.attempt, tc.base, tc.limit, got, tc.want)
		}
	}
}
