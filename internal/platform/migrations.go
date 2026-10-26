package platform

import (
	"fmt"
	"math"
)

func NextFencingToken(current int64) (int64, error) {
	if current < 0 || current == math.MaxInt64 {
		return 0, NewFailure(CodeResourceLimit, fmt.Errorf("fencing token cannot be incremented"))
	}
	return current + 1, nil
}
