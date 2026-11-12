package main

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// TestReprocessMarkFollowsTheCandidateThatReplacesIt expects a marked candidate that is superseded
// before it publishes, here by activating another version, to hand its mark to the candidate that
// replaces it, so the replacement still retracts the old result and appends a reprocess fact.
func TestReprocessMarkFollowsTheCandidateThatReplacesIt(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, _, _ := reprocessWindow()
	v1 := f.saveVersion("3")
	f.activate(v1)
	transmission := f.submit("handover", firstTime)
	f.runWorker()
	f.requestReprocess(v1, from, to)
	f.runReprocess()
	if f.marked(transmission) != 1 {
		t.Fatal("the runner must have marked the candidate")
	}
	// Before the marked candidate is dispatched another version becomes active: reconciliation
	// supersedes it and admits a candidate at a new key.
	f.activate(f.saveVersion("4"))
	f.runWorker()

	if got := f.levels(transmission); !slices.Equal(got, []string{"4"}) {
		t.Fatalf("levels = %v, want the replacement's result only", got)
	}
	if published, retracted := f.results(transmission); published != 1 || retracted != 1 {
		t.Fatalf("results published=%d retracted=%d, want the old result retracted", published, retracted)
	}
	if modes := f.replayModes(transmission); !slices.Equal(modes, []string{"normal", "reprocess"}) {
		t.Fatalf("replay modes = %v, want the replacement to be a reprocess fact", modes)
	}
	if f.marked(transmission) != 0 {
		t.Fatal("no candidate may keep the mark, the donor's included")
	}
}

// readmit re-admits Processing for the Transmission like a correction does, in its own transaction.
func (f *reprocessFixture) readmit(transmission uuid.UUID) {
	f.t.Helper()
	if err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		return f.runtime.readmission().ReadmitProcessing(tx.Context(), tx, transmission)
	}); err != nil {
		f.t.Fatal(err)
	}
}

// TestReprocessMarksACandidateAdmittedBetweenTheRequestAndTheRunner expects a candidate that a
// readmission creates at the request's pinned revision for a Workflow with history, before the
// runner reached the Transmission, to be historical: it carries the request's mark, replaces the
// older result and appends a reprocess fact, instead of passing for fresh intake.
func TestReprocessMarksACandidateAdmittedBetweenTheRequestAndTheRunner(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, _, _ := reprocessWindow()
	f.activate(f.saveVersion("3"))
	transmission := f.submit("between", firstTime)
	f.runWorker()
	v2 := f.saveVersion("4")
	f.activate(v2)
	f.requestReprocess(v2, from, to)
	// A correction readmission (or the dispatch of another Workflow) reconciles the Transmission
	// before the runner gets to it and creates the v2 candidate at the pinned revision.
	f.readmit(transmission)
	f.runReprocess()
	f.runWorker()

	if got := f.levels(transmission); !slices.Equal(got, []string{"4"}) {
		t.Fatalf("levels = %v, want the replacement's result only", got)
	}
	if modes := f.replayModes(transmission); !slices.Equal(modes, []string{"normal", "reprocess"}) {
		t.Fatalf("replay modes = %v, want the re-run to be a reprocess fact", modes)
	}
	if published, retracted := f.results(transmission); published != 1 || retracted != 1 {
		t.Fatalf("results published=%d retracted=%d", published, retracted)
	}
}
