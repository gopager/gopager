package processing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestInvokeFailureReportsAnEarlyRemoteDeadlineAsTheDeadline expects a deadline reported by the
// plugin side just before the caller's own context expired to surface as the invocation deadline,
// not as invalid input, and every failure well before the deadline to stay the plugin's.
func TestInvokeFailureReportsAnEarlyRemoteDeadlineAsTheDeadline(t *testing.T) {
	t.Parallel()
	for _, code := range []codes.Code{codes.DeadlineExceeded, codes.Canceled} {
		ctx, cancel := context.WithTimeoutCause(context.Background(), 50*time.Millisecond, errInvocationDeadline)
		err := invokeFailure(ctx, status.Error(code, "remote deadline"))
		cancel()
		if platform.FailureCode(err) != platform.CodeResourceLimit || !errors.Is(err, errInvocationDeadline) {
			t.Errorf("%s before the local deadline: %v, want the invocation deadline", code, err)
		}
	}
	ctx, cancel := context.WithTimeoutCause(context.Background(), time.Minute, errInvocationDeadline)
	defer cancel()
	// Well before the caller's deadline, every failure is the plugin's and returns at once, so the
	// process slot is not held until the deadline.
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.Canceled} {
		started := time.Now()
		if err := invokeFailure(ctx, status.Error(code, "plugin")); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s well before the deadline: %v, want INVALID_INPUT", code, err)
		}
		if time.Since(started) > time.Second {
			t.Errorf("%s well before the deadline waited for it", code)
		}
	}
	if err := invokeFailure(context.Background(), status.Error(codes.DeadlineExceeded, "remote")); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Errorf("remote deadline without a local one: %v, want INVALID_INPUT", err)
	}
}
