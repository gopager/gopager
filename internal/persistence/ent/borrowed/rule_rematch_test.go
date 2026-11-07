package borrowed_test

import (
	"testing"

	"github.com/google/uuid"
)

// TestRuleChangeMovesInferredTransmission covers F-101: an Address-rule change on a Network of a
// multi-Network Gateway mapping queues a gateway scope with reason rule-change, which moves an
// inferred Transmission to the Network whose rules now match; a single-Network mapping queues none.
func TestRuleChangeMovesInferredTransmission(t *testing.T) {
	t.Parallel()
	f := newRematchFixture(t)
	single, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], f.unit, 300, 300, 1),
		checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.ruleGatewayScopes(t); got != 0 {
		t.Fatalf("a rule change on a single-Network mapping queued %d gateway scopes", got)
	}
	f.replace(t, mapping("n1", f.networks[0], 0), mapping("n2", f.networks[1], 1))
	drainCorrections(t, f.runner)
	receipt := f.submit(t, "inferred", "", 300)
	if got := f.state(t, receipt.TransmissionID); got.network != f.networks[0] || got.resolution != "inferred-address" {
		t.Fatalf("setup: %+v", got)
	}

	if _, err := f.rules.DeleteRule(t.Context(), f.proof, single.ID, single.Revision, checkedMutation); err != nil {
		t.Fatal(err)
	}
	drainCorrections(t, f.runner)
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[1], f.unitB, 300, 300, 1),
		checkedMutation); err != nil {
		t.Fatal(err)
	}
	if got := f.ruleGatewayScopes(t); got != 2 {
		t.Fatalf("two rule changes on a two-Network mapping queued %d gateway scopes, want 2", got)
	}
	drainCorrections(t, f.runner)
	got := f.state(t, receipt.TransmissionID)
	if got.network != f.networks[1] || got.resolution != "inferred-address" || got.unresolved ||
		got.unit == nil || *got.unit != f.unitB {
		t.Fatalf("not moved to the Network whose rule now matches: %+v", got)
	}
	if cache := f.cacheAgencies(t, receipt.MessageID); len(cache) != 1 ||
		!cache[f.agencies[1].String()+"/"+f.unitB.String()+"/current"] {
		t.Fatalf("visibility did not move to Agency B: %v", cache)
	}
	// The deletion un-assigns the target on Network 0; the creation then moves the Transmission.
	facts := f.movedFacts(t, receipt.TransmissionID)
	if len(facts) != 2 || facts[0]["previousNetworkId"] != nil ||
		facts[1]["previousNetworkId"] != f.networks[0].String() || facts[1]["networkId"] != f.networks[1].String() {
		t.Fatalf("assignment facts=%v; want an unassignment, then one move from Network 0 to Network 1", facts)
	}
}

// ruleGatewayScopes counts the gateway scopes queued for reason rule-change.
func (f *rematchFixture) ruleGatewayScopes(t *testing.T) int {
	t.Helper()
	var count int
	f.queryRow(t, `SELECT count(*) FROM assignment_pending_scopes
		WHERE reason='rule-change' AND gateway_id IS NOT NULL`, nil, &count)
	return count
}
