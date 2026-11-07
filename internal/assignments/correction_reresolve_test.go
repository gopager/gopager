package assignments

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// reresolveFixture resolves every item to resolution and records the Network it was matched on.
type reresolveFixture struct {
	resolution NetworkResolution
	resolved   int
	matchedOn  uuid.UUID
}

func (f *reresolveFixture) ResolveNetwork(context.Context, platform.TransactionContext, CorrectionBatchItem) (NetworkResolution, error) {
	f.resolved++
	return f.resolution, nil
}

func (f *reresolveFixture) MatchTargets(_ context.Context, _ platform.TransactionContext, network uuid.UUID,
	_ protocols.Protocol, _ time.Time, targets []protocols.Target) (MatchResult, error) {
	f.matchedOn = network
	return MatchResult{Revision: 7, Resolutions: make([]Resolution, len(targets))}, nil
}

func TestReresolveMatchesOnTheResolvedNetworkOnlyForGatewayScopes(t *testing.T) {
	t.Parallel()
	stored, moved := uuid.New(), uuid.New()
	item := CorrectionBatchItem{TransmissionID: uuid.New(), NetworkID: stored, Protocol: protocols.POCSAG}
	gateway := CorrectionLease{AssignmentRev: 7, Scope: GatewayScope(uuid.New(), protocols.POCSAG)}
	tx := runnerTransaction{ctx: t.Context()}
	for name, test := range map[string]struct {
		lease      CorrectionLease
		resolution NetworkResolution
		want       uuid.UUID
		resolves   int
	}{
		"gateway resolved":   {gateway, NetworkResolution{NetworkID: moved, Method: "priority", Resolved: true}, moved, 1},
		"gateway unresolved": {gateway, NetworkResolution{}, stored, 1},
		"rule scope":         {CorrectionLease{AssignmentRev: 7, Scope: UnitScope(uuid.New())}, NetworkResolution{}, stored, 0},
	} {
		f := &reresolveFixture{resolution: test.resolution}
		runner := &CorrectionRunner{matcher: f, networks: f}
		update, err := runner.reresolve(tx, test.lease, item)
		if err != nil || f.matchedOn != test.want || f.resolved != test.resolves ||
			(test.resolves == 1) != (update.Network != nil) {
			t.Fatalf("%s: update=%+v matchedOn=%v err=%v", name, update, f.matchedOn, err)
		}
	}
}

func TestCorrectionFactsNameThePreviousNetworkOfAMove(t *testing.T) {
	t.Parallel()
	from, to, message := uuid.New(), uuid.New(), uuid.New()
	changed, targetless := uuid.New(), uuid.New()
	unit := uuid.New()
	facts := correctionFacts([]CorrectionChange{{TransmissionID: changed, MessageID: message, NetworkID: to,
		TargetPosition: 0, MessageRev: 2, Before: events.IntakeResolution{Status: "unassigned"},
		After: events.IntakeResolution{Status: "assigned", UnitID: &unit}}},
		[]NetworkMove{{TransmissionID: changed, MessageID: message, From: from, To: to, MessageRev: 2},
			{TransmissionID: targetless, MessageID: message, From: from, To: to, MessageRev: 2}}, time.Now())
	if len(facts) != 2 || facts[0].TransmissionID != changed || len(facts[0].Targets) != 1 ||
		facts[1].TransmissionID != targetless || len(facts[1].Targets) != 0 {
		t.Fatalf("facts=%+v", facts)
	}
	for _, fact := range facts {
		if fact.NetworkID != to || fact.PreviousNetworkID == nil || *fact.PreviousNetworkID != from || fact.Revision != 2 {
			t.Fatalf("move not recorded: %+v", fact)
		}
	}
}
