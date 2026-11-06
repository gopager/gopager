package borrowed_test

import (
	"errors"
	"fmt"
	"strings"
)

// causeChain renders every wrapped layer of err, including joined errors, so a classified
// failure also shows its cause in a test report.
func causeChain(err error) string {
	if err == nil {
		return "<nil>"
	}
	layer := fmt.Sprintf("%T: %v", err, err)
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		parts := make([]string, 0, len(joined.Unwrap()))
		for _, inner := range joined.Unwrap() {
			parts = append(parts, causeChain(inner))
		}
		return layer + " <- [" + strings.Join(parts, " | ") + "]"
	}
	if inner := errors.Unwrap(err); inner != nil {
		return layer + " <- " + causeChain(inner)
	}
	return layer
}
