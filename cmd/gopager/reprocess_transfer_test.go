package main

import (
	"slices"
	"testing"
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
