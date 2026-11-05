package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

func TestClassifyFailureKeepsResourceLimitTerminalAndAuthorityRetryable(t *testing.T) {
	for _, test := range []struct {
		err       error
		code      platform.ErrorCode
		retryable bool
	}{
		{platform.NewFailure(platform.CodeResourceLimit, context.DeadlineExceeded), platform.CodeResourceLimit, false},
		{platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("handshake")), platform.CodeAuthorityUnavailable, true},
		{platform.NewFailure(platform.CodeInvalidInput, errors.New("malformed")), platform.CodeInvalidInput, false},
		{platform.NewFailure(platform.CodeStorageUnavailable, errors.New("storage")), platform.CodeStorageUnavailable, true},
		{errors.New("unclassified"), platform.CodeInternalError, false},
	} {
		if code, retryable := classifyFailure(test.err); code != test.code || retryable != test.retryable {
			t.Fatalf("%v classified as %s/%v, want %s/%v", test.err, code, retryable, test.code, test.retryable)
		}
	}
}
