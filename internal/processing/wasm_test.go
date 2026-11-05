package processing

import (
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

func TestWASIOutputBoundaryRemainsFailed(t *testing.T) {
	output := &boundedOutput{limit: MaxOutputBytes}
	if n, err := output.Write([]byte(strings.Repeat(" ", MaxOutputBytes))); err != nil || n != MaxOutputBytes {
		t.Fatalf("exact output boundary failed: %d, %v", n, err)
	}
	if _, err := output.Write([]byte("x")); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("output +1 accepted: %v", err)
	}
	if _, err := output.Write(nil); platform.FailureCode(err) != platform.CodeResourceLimit ||
		platform.FailureCode(output.err) != platform.CodeResourceLimit {
		t.Fatalf("overflow became successful: %v", err)
	}
}
