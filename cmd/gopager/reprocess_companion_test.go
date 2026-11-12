package main

import (
	"slices"
	"testing"
)

// TestReprocessOfOneWorkflowMarksTheOthersReruns expects the re-run of an unrelated Workflow B on
// a Transmission under Reprocess of Workflow A to be historical too: it replaces B's older result
// and appends a reprocess fact, while fresh intake of both Workflows stays normal.
func TestReprocessOfOneWorkflowMarksTheOthersReruns(t *testing.T) {
	t.Parallel()
	a := newReprocessFixture(t)
	b := a.otherWorkflow("Other", "alert")
	from, to, firstTime, secondTime, _ := reprocessWindow()
	va, vb := a.saveVersion("3"), b.saveVersion("5")
	a.activate(va)
	b.activate(vb)
	history := a.submit("history", firstTime)
	a.runWorker()
	request := a.requestReprocess(va, from, to)
	fresh := a.submit("fresh", secondTime)
	a.runWorker()
	a.runReprocess()
	if a.request(request).Readmitted != 1 {
		t.Fatalf("request = %+v, want the historical Transmission readmitted", a.request(request))
	}
	a.runWorker()

	if modes := a.replayModesOf(history); !slices.Equal(modes, []string{"normal", "reprocess"}) {
		t.Fatalf("A replay modes = %v", modes)
	}
	if modes := b.replayModesOf(history); !slices.Equal(modes, []string{"normal", "reprocess"}) {
		t.Fatalf("B replay modes = %v, want its re-run to be a reprocess fact", modes)
	}
	if published, retracted := b.resultsOf(history); published != 1 || retracted != 1 {
		t.Fatalf("B results published=%d retracted=%d, want the older result replaced", published, retracted)
	}
	for name, view := range map[string]*reprocessFixture{"A": a, "B": b} {
		if modes := view.replayModesOf(fresh); !slices.Equal(modes, []string{"normal"}) {
			t.Fatalf("%s fresh replay modes = %v, want normal", name, modes)
		}
	}
	if a.marked(history) != 0 {
		t.Fatal("marks must clear once the replacements published")
	}
}

// TestReprocessMarksAnotherWorkflowReselectedAtDispatch expects a Workflow B that finished its
// marked re-run and is reselected again when A's candidate is dispatched, after a further
// configuration change, to inherit the mark.
func TestReprocessMarksAnotherWorkflowReselectedAtDispatch(t *testing.T) {
	t.Parallel()
	a := newReprocessFixture(t)
	b := a.otherWorkflow("Other", "alert")
	from, to, firstTime, _, _ := reprocessWindow()
	va, vb := a.saveVersion("3"), b.saveVersion("5")
	a.activate(va)
	b.activate(vb)
	history := a.submit("history", firstTime)
	a.runWorker()
	a.requestReprocess(va, from, to)
	a.runReprocess()
	// B finishes first, which clears its own mark; a configuration change then makes A's dispatch
	// reselect B from its completed state while A is still marked.
	b.runWorkerFor()
	b.updateWorkflow("Other renamed", b.agency)
	a.runWorker()

	if modes := b.replayModesOf(history); !slices.Equal(modes, []string{"normal", "reprocess", "reprocess"}) {
		t.Fatalf("B replay modes = %v, want both re-runs to be reprocess facts", modes)
	}
	if published, _ := b.resultsOf(history); published != 1 {
		t.Fatalf("B published results = %d, want 1", published)
	}
}
