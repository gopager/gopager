package processing

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReprocessInputValidate(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	valid := ReprocessInput{VersionID: uuid.New(), ExpectedRevision: 3, MessageTimeFrom: from, MessageTimeTo: from.Add(time.Hour)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ReprocessInput){
		"missing version":  func(i *ReprocessInput) { i.VersionID = uuid.Nil },
		"missing revision": func(i *ReprocessInput) { i.ExpectedRevision = 0 },
		"missing from":     func(i *ReprocessInput) { i.MessageTimeFrom = time.Time{} },
		"missing to":       func(i *ReprocessInput) { i.MessageTimeTo = time.Time{} },
		"empty range":      func(i *ReprocessInput) { i.MessageTimeTo = i.MessageTimeFrom },
		"reversed range":   func(i *ReprocessInput) { i.MessageTimeTo = i.MessageTimeFrom.Add(-time.Second) },
		"empty after truncating to microseconds": func(i *ReprocessInput) {
			i.MessageTimeFrom = from.Add(100 * time.Nanosecond)
			i.MessageTimeTo = from.Add(900 * time.Nanosecond)
		},
		"negative revision": func(i *ReprocessInput) { i.ExpectedRevision = -1 },
	} {
		input := valid
		mutate(&input)
		if err := input.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRetractedResultIsNotPublishable(t *testing.T) {
	t.Parallel()
	selected := true
	now := time.Now()
	item := PublicationItem{Candidate: Candidate{State: "completed", Selected: &selected}, Result: &PrivateResult{}}
	if !item.Publishable() {
		t.Fatal("a completed selected unpublished result must be publishable")
	}
	item.Result.RetractedAt = &now
	if item.Publishable() {
		t.Fatal("a retracted result must never publish again")
	}
}

func TestReplacesPublishedNeedsMarkAndCompletion(t *testing.T) {
	t.Parallel()
	mark := uuid.New()
	for _, test := range []struct {
		state  string
		marked bool
		want   bool
	}{
		{"completed", true, true},
		{"completed", false, false},
		{"failed", true, false},
		{"pending", true, false},
		{"running", true, false},
	} {
		item := PublicationItem{Candidate: Candidate{State: test.state}}
		if test.marked {
			item.ReprocessID = &mark
		}
		if got := item.ReplacesPublished(); got != test.want {
			t.Errorf("state=%s marked=%v: ReplacesPublished = %v, want %v", test.state, test.marked, got, test.want)
		}
	}
}
