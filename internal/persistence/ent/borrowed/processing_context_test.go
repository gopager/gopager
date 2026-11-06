package borrowed_test

import (
	"testing"

	"github.com/gopager/gopager/internal/protocols"
)

// TestProcessingContextFollowsOwnTargetsNotTheGlobalRevision expects a candidate to stay valid
// when the Assignment revision moves for a change that did not touch the Transmission's own
// targets: the processing context is derived from the targets' assignment revision (1 for a
// targetless Transmission), so the worker completes the candidate instead of superseding it.
func TestProcessingContextFollowsOwnTargetsNotTheGlobalRevision(t *testing.T) {
	t.Parallel()
	f := newProcessingSourceFixture(t)
	f.seedWorkflow(t, "any", protocolSelectorDefinition(t))
	function := uint8(1)
	withTarget := f.accept(t, "with-target", f.now, toneOnlyPayload,
		[]protocols.Target{{Address: protocols.NumericAddress(10), Function: &function}})
	targetless := f.accept(t, "targetless", f.now.Add(-3600e9), toneOnlyPayload, nil)
	// An unrelated Assignment change elsewhere in the installation.
	f.exec(t, `UPDATE assignment_control SET revision=revision+1 WHERE id=1`)
	candidates := f.dueCandidates(t)
	if len(candidates) != 2 {
		t.Fatalf("due candidates=%d want 2", len(candidates))
	}
	for _, candidate := range candidates {
		if err := f.worker.ExecuteWork(t.Context(), candidate); err != nil {
			t.Fatalf("candidate of an untouched Transmission did not complete: %v", err)
		}
	}
	for _, receipt := range []struct {
		name string
		id   string
	}{{"with-target", withTarget.TransmissionID.String()}, {"targetless", targetless.TransmissionID.String()}} {
		var count int
		f.queryRow(t, `SELECT count(*) FROM processing_candidates WHERE transmission_id=$1`, []any{receipt.id}, &count)
		if count != 1 {
			t.Fatalf("%s: candidates=%d, want the single original candidate", receipt.name, count)
		}
	}
	if status := f.transmissionStatus(t, withTarget.TransmissionID); status != "completed" {
		t.Fatalf("with-target status=%q want completed", status)
	}
	if status := f.transmissionStatus(t, targetless.TransmissionID); status != "completed" {
		t.Fatalf("targetless status=%q want completed", status)
	}
}
