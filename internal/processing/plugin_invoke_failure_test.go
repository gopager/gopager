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
// plugin side before the caller's own context expired to surface as the invocation deadline, not
// as invalid input, and every other failure to stay the plugin's.
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
	started := time.Now()
	if err := invokeFailure(ctx, status.Error(codes.Unavailable, "transport closed")); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Errorf("other failure: %v, want INVALID_INPUT", err)
	}
	if time.Since(started) > time.Second {
		t.Error("an ordinary failure waited for the deadline")
	}
	if err := invokeFailure(context.Background(), status.Error(codes.DeadlineExceeded, "remote")); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Errorf("remote deadline without a local one: %v, want INVALID_INPUT", err)
	}
}
