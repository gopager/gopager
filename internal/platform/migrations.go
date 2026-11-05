package platform

import (
	"fmt"
	"math"
)

// NextFencingToken returns current+1. It fails with CodeResourceLimit when
// current is negative or already math.MaxInt64, so a token never wraps or
// moves backwards.
func NextFencingToken(current int64) (int64, error) {
	if current < 0 || current == math.MaxInt64 {
		return 0, NewFailure(CodeResourceLimit, fmt.Errorf("fencing token cannot be incremented"))
	}
	return current + 1, nil
}
