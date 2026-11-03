package processing

import (
	"errors"
	"math/rand/v2"
	"time"
)

const MaxAutomaticAttempts = 5

// NextAutomaticRetry returns a primary-clock due time or explicit terminal exhaustion.
// completedAttempts counts actual attempts; callers must enforce their current lease/fencing first.
func NextAutomaticRetry(completedAttempts int, primaryNow time.Time) (time.Time, bool, error) {
	return nextAutomaticRetry(completedAttempts, primaryNow, rand.Int64N)
}

func nextAutomaticRetry(completedAttempts int, primaryNow time.Time, sample func(int64) int64) (time.Time, bool, error) {
	if completedAttempts < 1 || completedAttempts > MaxAutomaticAttempts || primaryNow.IsZero() ||
		primaryNow.Nanosecond()%1000 != 0 || sample == nil {
		return time.Time{}, false, invalid(errors.New("invalid internal retry state or primary time"))
	}
	if completedAttempts == MaxAutomaticAttempts {
		return time.Time{}, false, nil
	}
	delays := [...]time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}
	nominal := delays[completedAttempts-1]
	spread := nominal / 5
	minimum := nominal - spread
	choices := int64(2*spread/time.Microsecond) + 1
	offset := sample(choices)
	if offset < 0 || offset >= choices {
		return time.Time{}, false, invalid(errors.New("internal retry jitter sample outside bound"))
	}
	due := primaryNow.UTC().Add(minimum + time.Duration(offset)*time.Microsecond)
	return due, true, nil
}
