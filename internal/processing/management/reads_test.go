package management

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestCandidateStatusValidationKeepsOnlySafeFailureVocabulary(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	selected := true
	status := CandidateStatus{ID: uuid.New(), Revision: 1, State: "completed", Selected: &selected, UpdatedAt: now}
	if err := validateCandidateStatus(status); err != nil {
		t.Fatal(err)
	}
	code := platform.CodeResourceLimit
	status = CandidateStatus{ID: uuid.New(), Revision: 2, State: "failed", FailureCode: &code, UpdatedAt: now}
	if err := validateCandidateStatus(status); err != nil {
		t.Fatal(err)
	}
	unsafe := platform.ErrorCode("GUEST_ERROR")
	status.FailureCode = &unsafe
	if err := validateCandidateStatus(status); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("unsafe failure vocabulary accepted: %v", err)
	}
	selected = false
	status = CandidateStatus{ID: uuid.New(), Revision: 1, State: "pending", Selected: &selected, UpdatedAt: now}
	if err := validateCandidateStatus(status); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("nonterminal selection leaked into status: %v", err)
	}
}

func TestPrivateResultMetadataOmitsPrivateEvaluationAndSourceFields(t *testing.T) {
	t.Parallel()
	metadata := PrivateResultMetadata{ID: uuid.New(), Revision: 2, CompletedAt: time.Now().UTC(), CandidateID: uuid.New()}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, privateName := range []string{"evaluation", "candidateId", "transmissionId", "sourceRevision", "fields"} {
		if strings.Contains(string(encoded), privateName) {
			t.Fatalf("private field %q appeared in metadata: %s", privateName, encoded)
		}
	}
}

func TestProcessingStatusPageBoundsAndDetachesMutableFields(t *testing.T) {
	t.Parallel()
	for _, input := range []processing.PageInput{
		{Offset: -1, Limit: 1},
		{Limit: 0},
		{Limit: processing.MaxCollection + 1},
		{Offset: int(^uint(0) >> 1), Limit: 1},
	} {
		if err := validatePageInput(input); err == nil {
			t.Fatalf("invalid status page accepted: %+v", input)
		}
	}
	if err := validatePageInput(processing.PageInput{Offset: 3, Limit: processing.MaxCollection}); err != nil {
		t.Fatalf("valid bounded page rejected: %v", err)
	}
	selected := true
	code := platform.CodeResourceLimit
	status := detachCandidateStatus(CandidateStatus{Selected: &selected, FailureCode: &code})
	selected = false
	code = platform.CodeInternalError
	if !*status.Selected || *status.FailureCode != platform.CodeResourceLimit {
		t.Fatal("candidate status retained caller-owned pointers")
	}
}

func TestCandidateStatusExplainsTheFailureWithReasonAndStep(t *testing.T) {
	t.Parallel()
	step := "extract-level"
	for code, reason := range map[platform.ErrorCode]string{
		platform.CodeResourceLimit:        "resource-limit",
		platform.CodeAuthorityUnavailable: "capability-unavailable",
		platform.CodeInvalidInput:         "conversion-failed",
		platform.CodeInternalError:        "evaluation-failed",
	} {
		status := CandidateStatus{ID: uuid.New(), Revision: 2, State: "failed", FailureCode: &code, FailureStep: &step,
			UpdatedAt: time.Now().UTC()}
		if err := validateCandidateStatus(status); err != nil {
			t.Fatal(err)
		}
		explained := detachCandidateStatus(status)
		if explained.FailureReason != reason || explained.FailureStep == nil || *explained.FailureStep != step {
			t.Fatalf("%s: reason %q step %v", code, explained.FailureReason, explained.FailureStep)
		}
		encoded, err := json.Marshal(explained)
		if err != nil || !strings.Contains(string(encoded), `"failureReason":"`+reason+`"`) ||
			!strings.Contains(string(encoded), `"failureStep":"extract-level"`) {
			t.Fatalf("encoded = %s, %v", encoded, err)
		}
	}
	orphan := CandidateStatus{ID: uuid.New(), Revision: 1, State: "failed", FailureStep: &step, UpdatedAt: time.Now().UTC()}
	if err := validateCandidateStatus(orphan); err == nil {
		t.Fatal("a failing step without a failure code was accepted")
	}
}
