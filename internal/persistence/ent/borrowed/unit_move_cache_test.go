package borrowed_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// TestUnitMoveRewritesMessageUnitCache expects moving a Unit to another Agency to carry its
// message_units history along; otherwise the old Agency keeps the Unit's Messages and the new
// Agency sees none of them.
func TestUnitMoveRewritesMessageUnitCache(t *testing.T) {
	f := newMatcherFixture(t)
	networkID, gatewayID := f.networks[0], uuid.New()
	moved, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Station A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(networkID, moved.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways (id) VALUES($1)`, gatewayID); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default) VALUES($1,$2,'default',$3,true)`,
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
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "move", Protocol: protocols.POCSAG,
		ObservationTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC).Format(time.RFC3339), GatewayNetworkIdentifier: "default",
		Payload:             json.RawMessage(`{"form":"tone-only"}`),
		TransmissionTargets: []protocols.Target{{Address: protocols.NumericAddress(10), Function: &function}}}
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
	if rows := readMessageUnits(t, f, receipt.MessageID); len(rows) != 1 || rows[0].agency != f.agency {
		t.Fatalf("cache after intake = %+v, want one row of Agency A", rows)
	}
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, moved.ID, moved.Revision,
		units.UnitInput{AgencyID: f.agencies[1], Name: "Station A", CollectionIDs: []uuid.UUID{}}, checkedMutation); err != nil {
		t.Fatal(err)
	}
	rows := readMessageUnits(t, f, receipt.MessageID)
	if len(rows) != 1 || !rows[0].unit.Valid || rows[0].unit.UUID != moved.ID || rows[0].agency != f.agencies[1] {
		t.Fatalf("cache after Unit move = %+v, want the Unit's row to carry Agency B", rows)
	}
}
