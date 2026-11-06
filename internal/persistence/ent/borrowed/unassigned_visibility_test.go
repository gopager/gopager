package borrowed_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// TestUnassignedTargetsAreVisibleToTheNetworkAgency expects a target that no address rule matches
// to be cached as unassigned traffic of its Network's Agency (the Unit-less row targetless traffic
// already uses), so the Agency sees it as unassigned; once a rule assigns the address and the
// correction has run, the Message carries the Unit's row and no unassigned row remains.
func TestUnassignedTargetsAreVisibleToTheNetworkAgency(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	networkID, gatewayID := f.networks[1], uuid.New()
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways (id) VALUES($1)`, gatewayID); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,priority) VALUES($1,$2,'only',$3,0)`,
			uuid.New(), gatewayID, networkID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	intake, err := ingeststore.New(f.store, f.facts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	observed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "unassigned", Protocol: protocols.POCSAG,
		ObservationTime: observed.Format(time.RFC3339), GatewayNetworkIdentifier: "only",
		Payload:             json.RawMessage(`{"form":"tone-only"}`),
		TransmissionTargets: []protocols.Target{{Address: protocols.NumericAddress(4242), Function: &function}}}
	var receipt ingest.Receipt
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		accepted, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		validated, err := submission.Validate(4096, accepted)
		if err != nil {
			return err
		}
		validated.ResolvedNetwork, validated.Resolution = networkID, "declared"
		matched, err := f.rules.MatchTargets(tx.Context(), tx, networkID, protocols.POCSAG, validated.ObservedAt, validated.TransmissionTargets)
		if err != nil {
			return err
		}
		receipt, err = intake.AcceptTransmission(tx.Context(), tx, gatewayID, validated, accepted, matched)
		return err
	}); err != nil {
		t.Fatalf("intake: %v (%s)", err, causeChain(err))
	}
	rows := readMessageUnits(t, f, receipt.MessageID)
	if len(rows) != 1 || rows[0].unit.Valid || rows[0].agency != f.agencies[1] || !rows[0].first.Equal(observed) {
		t.Fatalf("cache after an unmatched intake = %+v, want one unassigned row of the Network's Agency at %s", rows, observed)
	}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[1], Name: "Station", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(networkID, unit.ID, 4242, 4242, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	if drainCorrections(t, runner) == 0 {
		t.Fatal("correction did not run")
	}
	rows = readMessageUnits(t, f, receipt.MessageID)
	if len(rows) != 1 || rows[0].unit.UUID != unit.ID || rows[0].status != "current" {
		t.Fatalf("cache after the rule assigned the address = %+v, want only the Unit's row", rows)
	}
}
