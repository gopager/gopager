package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestEvaluationFailureIsExplainedWithoutRawText(t *testing.T) {
	t.Parallel()
	secret := errors.New("dial tcp 10.0.0.7: secret-host refused")
	step := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(platform.CodeAuthorityUnavailable, secret)}
	failure, ok := previewFailure(step)
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
	limit, ok := previewFailure(platform.NewFailure(platform.CodeResourceLimit, errors.New("budget")))
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
		if failure, ok := previewFailure(err); ok {
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
		platform.CodeStorageUnavailable, platform.CodeCommitUncertain, platform.CodeRevisionConflict} {
		err := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(code, errors.New("cause"))}
		if failure, ok := previewFailure(err); ok {
			t.Fatalf("%s inside a step became the explanation %+v", code, failure)
		}
	}
	for _, code := range []platform.ErrorCode{platform.CodeInvalidInput, platform.CodeIncompatibleVersion,
		platform.CodeAuthorityUnavailable, platform.CodeInternalError} {
		err := &processing.StepFailure{StepID: "lookup", Err: platform.NewFailure(code, errors.New("cause"))}
		if failure, ok := previewFailure(err); !ok || failure.StepID != "lookup" {
			t.Fatalf("%s inside a step = %+v, %v, want an explanation", code, failure, ok)
		}
	}
}
