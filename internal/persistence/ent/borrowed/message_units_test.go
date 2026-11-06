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

// messageUnitRow is one row of the message_units cache as the tests read it.
type messageUnitRow struct {
	unit    uuid.NullUUID
	agency  uuid.UUID
	first   time.Time
	anchor  time.Time
	status  string
	message uuid.UUID
}

func readMessageUnits(t *testing.T, f matcherFixture, message uuid.UUID) []messageUnitRow {
	t.Helper()
	var rows []messageUnitRow
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		result, err := executor.QueryContext(tx.Context(), `SELECT unit_id,agency_id,first_observed_at,age_anchor,
			correction_status,message_id FROM message_units WHERE message_id=$1 ORDER BY unit_id NULLS FIRST`, message)
		if err != nil {
			return err
		}
		defer result.Close()
		for result.Next() {
			var row messageUnitRow
			if err := result.Scan(&row.unit, &row.agency, &row.first, &row.anchor, &row.status, &row.message); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return result.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestMessageUnitCacheFollowsIntakeAndCorrection expects the message_units cache to replace
// contributions and supports: intake upserts one row per Message and Unit carrying the Unit's
// Agency, the earliest observation and the age anchor; a targetless Transmission yields one
// Unit-less row per Message and Network Agency; a correction that moves an address to a Unit of
// another Agency replaces the rows in place; targets carry typed columns and the Transmission
// no longer duplicates its targets as JSON.
func TestMessageUnitCacheFollowsIntakeAndCorrection(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	networkID, gatewayID := f.networks[0], uuid.New()
	unitA, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Station A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unitB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[1], Name: "Station B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(networkID, unitA.ID, 10, 20, 1), checkedMutation)
	if err != nil {
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
			(id,gateway_id,gateway_network_identifier,network_id,priority) VALUES($1,$2,'default',$3,0)`,
			uuid.New(), gatewayID, networkID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	intake, err := ingeststore.New(f.store, f.facts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	accept := func(id string, observed time.Time, targets []protocols.Target) ingest.Receipt {
		t.Helper()
		if targets == nil {
			targets = []protocols.Target{}
		}
		submission := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: observed.Format(time.RFC3339), GatewayNetworkIdentifier: "default",
			Payload: json.RawMessage(`{"form":"tone-only"}`), TransmissionTargets: targets}
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
			t.Fatalf("intake %s: %v (%s)", id, err, causeChain(err))
		}
		return receipt
	}
	function := uint8(1)
	target := protocols.Target{Address: protocols.NumericAddress(10), Function: &function}
	observed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	later := accept("later", observed.Add(2*time.Second), []protocols.Target{target})
	earlier := accept("earlier", observed, []protocols.Target{target})
	targetless := accept("targetless", observed.Add(time.Second), nil)
	if earlier.MessageID != later.MessageID || targetless.MessageID != later.MessageID {
		t.Fatalf("intakes did not group into one Message: %s %s %s", later.MessageID, earlier.MessageID, targetless.MessageID)
	}
	rows := readMessageUnits(t, f, later.MessageID)
	if len(rows) != 2 || rows[0].unit.Valid || rows[0].agency != f.agency || !rows[0].first.Equal(observed.Add(time.Second)) ||
		!rows[1].unit.Valid || rows[1].unit.UUID != unitA.ID || rows[1].agency != f.agency ||
		!rows[1].first.Equal(observed) || !rows[1].anchor.Equal(observed) || rows[1].status != "current" {
		t.Fatalf("cache after intake = %+v, want a Unit-less row of the Network's Agency at %s and a Unit A row at %s", rows, observed.Add(time.Second), observed)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var network uuid.UUID
		var protocol string
		var observedAt time.Time
		var number *int64
		var text *string
		var fn *int16
		if err := executor.QueryRowContext(tx.Context(), `SELECT network_id,protocol,observed_at,address_number,address_text,function
			FROM transmission_targets WHERE transmission_id=$1 AND position=0`, later.TransmissionID).
			Scan(&network, &protocol, &observedAt, &number, &text, &fn); err != nil {
			return err
		}
		if network != networkID || protocol != "pocsag" || !observedAt.Equal(observed.Add(2*time.Second)) ||
			number == nil || *number != 10 || text != nil || fn == nil || *fn != 1 {
			t.Fatalf("typed target columns = %s %s %s %v %v %v", network, protocol, observedAt, number, text, fn)
		}
		var duplicates int
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM information_schema.columns
			WHERE table_name='transmissions' AND column_name='targets'`).Scan(&duplicates); err != nil {
			return err
		}
		if duplicates != 0 {
			t.Fatal("transmissions still duplicates its targets as JSON")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, rule.ID, rule.Revision, ruleInput(networkID, unitB.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	if drainCorrections(t, runner) == 0 {
		t.Fatal("correction did not run")
	}
	rows = readMessageUnits(t, f, later.MessageID)
	if len(rows) != 2 || rows[0].unit.Valid || rows[0].agency != f.agency ||
		!rows[1].unit.Valid || rows[1].unit.UUID != unitB.ID || rows[1].agency != f.agencies[1] ||
		!rows[1].first.Equal(observed) || rows[1].status != "current" {
		t.Fatalf("cache after correction = %+v, want the Unit-less row and a Unit B row of Agency B at %s", rows, observed)
	}
}
