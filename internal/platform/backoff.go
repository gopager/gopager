package platform

import "time"

// Backoff returns the delay before retry number attempt (1 for the first retry; smaller values
// count as 1): base doubled for every further attempt, capped at limit. A base above limit is
// used as it is, so a configured interval is never shortened.
func Backoff(attempt int, base, limit time.Duration) time.Duration {
	delay := base
	for i := 1; i < attempt && delay < limit; i++ {
		delay *= 2
	}
	return min(delay, max(base, limit))
}
