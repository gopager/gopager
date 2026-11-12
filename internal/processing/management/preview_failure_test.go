package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestEvaluationFailureIsExplainedWithoutRawText(t *testing.T) {
	t.Parallel()
	secret := errors.New("dial tcp 10.0.0.7: secret-host refused")
	step := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(platform.CodeAuthorityUnavailable,
		errors.Join(processing.ErrCapabilityUnavailable, secret))}
	failure, ok := previewFailure(context.Background(), step)
	if !ok || failure.Code != "AUTHORITY_UNAVAILABLE" || failure.Reason != "capability-unavailable" || failure.StepID != "lookup" {
		t.Fatalf("failure = %+v, %v", failure, ok)
	}
	encoded, err := json.Marshal(PreviewResult{Failure: failure})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"failure":{"code":"AUTHORITY_UNAVAILABLE","reason":"capability-unavailable","stepId":"lookup"}}` ||
		strings.Contains(string(encoded), "secret-host") {
		t.Fatalf("encoded = %s", encoded)
	}
	limit, ok := previewFailure(context.Background(), platform.NewFailure(platform.CodeResourceLimit, errors.New("budget")))
	if !ok || limit.Code != "RESOURCE_LIMIT" || limit.Reason != "resource-limit" || limit.StepID != "" {
		t.Fatalf("limit = %+v, %v", limit, ok)
	}
	if encoded, _ = json.Marshal(PreviewResult{Failure: limit}); strings.Contains(string(encoded), "stepId") {
		t.Fatalf("a failure without a step names one: %s", encoded)
	}
}

func TestOnlyEvaluationFailuresAreAnsweredAsFailures(t *testing.T) {
	t.Parallel()
	for _, err := range []error{authz.ErrForbidden, authz.ErrNotFound, authz.ErrRevisionConflict, context.Canceled,
		platform.NewFailure(platform.CodeStorageUnavailable, errors.New("down")), errors.New("plain")} {
		if failure, ok := previewFailure(context.Background(), err); ok {
			t.Errorf("%v answered as %+v", err, failure)
		}
	}
}

func TestPreviewResultEncodesTheEvaluationUnchanged(t *testing.T) {
	t.Parallel()
	evaluation := processing.Evaluation{Selected: true}
	want, err := json.Marshal(evaluation)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(PreviewResult{Evaluation: evaluation})
	if err != nil || string(got) != string(want) {
		t.Fatalf("got %s, want %s, err %v", got, want, err)
	}
}

// TestWrappedRequestFailuresOfAStepStayErrors expects authorization, session and storage
// failures raised inside a step to keep their HTTP error instead of becoming an explanation.
func TestWrappedRequestFailuresOfAStepStayErrors(t *testing.T) {
	t.Parallel()
	for _, code := range []platform.ErrorCode{platform.CodeForbidden, platform.CodeLoginRequired, platform.CodeNotFound,
		platform.CodeStorageUnavailable, platform.CodeCommitUncertain, platform.CodeRevisionConflict, platform.CodeInternalError} {
		err := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(code, errors.New("cause"))}
		if failure, ok := previewFailure(context.Background(), err); ok {
			t.Fatalf("%s inside a step became the explanation %+v", code, failure)
		}
	}
	for _, code := range []platform.ErrorCode{platform.CodeInvalidInput, platform.CodeIncompatibleVersion} {
		err := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(code, errors.New("cause"))}
		if failure, ok := previewFailure(context.Background(), err); !ok || failure.StepID != "lookup" {
			t.Fatalf("%s inside a step = %+v, %v, want an explanation", code, failure, ok)
		}
	}
}

// TestAnExpiredCallerIsNotExplained expects a step failure caused by the caller's own expired
// deadline to stay an error, while the same failure under a live caller is explained.
func TestAnExpiredCallerIsNotExplained(t *testing.T) {
	t.Parallel()
	err := &processing.StepFailure{StepID: "lookup",
		Err: platform.NewFailure(platform.CodeInvalidInput, context.DeadlineExceeded)}
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if failure, ok := previewFailure(expired, err); ok {
		t.Fatalf("an expired caller became the explanation %+v", failure)
	}
	if _, ok := previewFailure(context.Background(), err); !ok {
		t.Fatal("a step failure under a live caller is not explained")
	}
}

// TestAFailureIsExplainedOnlyAfterTheFinalRecheck expects a failed evaluation to pass the same
// final authority and source fence as a result before it is explained: an editor who lost the
// right during the evaluation gets the error, not a 200 explanation.
func TestAFailureIsExplainedOnlyAfterTheFinalRecheck(t *testing.T) {
	t.Parallel()
	failed := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(platform.CodeInvalidInput, errors.New("cause"))}
	rechecks := 0
	revoked := func() error { rechecks++; return authz.ErrForbidden }
	if failure, err := explainAfterRecheck(context.Background(), failed, revoked); !errors.Is(err, authz.ErrForbidden) || failure != nil {
		t.Fatalf("revoked editor = %+v, %v, want FORBIDDEN", failure, err)
	}
	current := func() error { rechecks++; return nil }
	if failure, err := explainAfterRecheck(context.Background(), failed, current); err != nil || failure == nil || failure.StepID != "lookup" {
		t.Fatalf("current editor = %+v, %v, want the explanation", failure, err)
	}
	storage := platform.NewFailure(platform.CodeStorageUnavailable, errors.New("down"))
	if _, err := explainAfterRecheck(context.Background(), storage, current); !errors.Is(err, storage) {
		t.Fatalf("an unexplained error = %v, want it unchanged", err)
	}
	if rechecks != 2 {
		t.Fatalf("rechecks = %d, want one per explained failure", rechecks)
	}
}

// TestOnlyAnUnavailableCapabilityIsExplainedAsAuthorityUnavailable expects AUTHORITY_UNAVAILABLE
// inside a step to be explained only when a plugin capability caused it; an authority outage
// stays a retryable error.
func TestOnlyAnUnavailableCapabilityIsExplainedAsAuthorityUnavailable(t *testing.T) {
	t.Parallel()
	outage := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("resolver down"))}
	if failure, ok := previewFailure(context.Background(), outage); ok {
		t.Fatalf("an authority outage became the explanation %+v", failure)
	}
	capability := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(platform.CodeAuthorityUnavailable,
		errors.Join(processing.ErrCapabilityUnavailable, errors.New("plugin does not support preview")))}
	if failure, ok := previewFailure(context.Background(), capability); !ok || failure.Reason != "capability-unavailable" {
		t.Fatalf("an unavailable capability = %+v, %v, want the explanation", failure, ok)
	}
}
