package borrowed_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// networkScope is an address-rules correction scope covering every POCSAG address of a Network.
func networkScope(network uuid.UUID) assignments.CorrectionScope {
	from, to := uint64(0), uint64(2097151)
	return assignments.CorrectionScope{NetworkID: &network, Protocol: protocols.POCSAG, AddressFrom: &from, AddressTo: &to}
}

// drainCorrections runs the correction runner until no scope is due and returns how many runs did
// work; every rule change records its own scope, so tests drain instead of running once.
func drainCorrections(t *testing.T, runner *assignments.CorrectionRunner) int {
	t.Helper()
	worked := 0
	for i := 0; i < 50; i++ {
		did, err := runner.RunOnce(t.Context())
		if err != nil {
			t.Fatalf("correction run failed: %v", err)
		}
		if !did {
			return worked
		}
		worked++
	}
	t.Fatal("correction scopes did not drain")
	return worked
}

// targetState is the stored resolution of one target as the tests read it.
type targetState struct {
	status   string
	unit     uuid.NullUUID
	revision int64
}

func readTarget(t *testing.T, f matcherFixture, transmission uuid.UUID) targetState {
	t.Helper()
	var state targetState
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT resolution_status,unit_id,assignment_revision
			FROM transmission_targets WHERE transmission_id=$1 AND position=0`, transmission).
			Scan(&state.status, &state.unit, &state.revision)
	}); err != nil {
		t.Fatal(err)
	}
	return state
}

// TestCorrectionScopesBySelectorAndLeavesUnrelatedTargetsAlone expects a rule change to
// enqueue one correction scope describing the affected address selector, to mark only the
// cache rows of Messages with affected targets as correcting, and the correction run to
// re-resolve only targets inside that selector; a suppression scopes one target and a Unit
// deletion scopes that Unit's targets. Targets of other addresses and Networks keep their
// assignment revision, and no dirty-capture table exists.
func TestCorrectionScopesBySelectorAndLeavesUnrelatedTargetsAlone(t *testing.T) {
	f := newMatcherFixture(t)
	gatewayID := uuid.New()
	unitA, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency, Name: "A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unitB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency, Name: "B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unitC, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agencies[1], Name: "C", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	ruleA, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unitA.ID, 10, 20, 1), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unitB.ID, 30, 40, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[1], unitC.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	// Every rule creation recorded a scope; drain them so only the later changes are observed.
	drainCorrections(t, runner)
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways (id) VALUES($1)`, gatewayID); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default) VALUES($1,$2,'n0',$3,true),($4,$2,'n1',$5,false)`,
			uuid.New(), gatewayID, f.networks[0], uuid.New(), f.networks[1])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	intake, err := ingeststore.New(f.store, f.facts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	accept := func(id string, network uuid.UUID, identifier string, observed time.Time, address uint64) ingest.Receipt {
		t.Helper()
		submission := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: observed.Format(time.RFC3339), GatewayNetworkIdentifier: identifier,
			Payload:             json.RawMessage(`{"form":"tone-only"}`),
			TransmissionTargets: []protocols.Target{{Address: protocols.NumericAddress(address), Function: &function}}}
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
			validated.ResolvedNetwork, validated.Resolution = network, "declared"
			matched, err := f.rules.MatchTargets(tx.Context(), tx, network, protocols.POCSAG, validated.ObservedAt, validated.TransmissionTargets)
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
	// Observation times hours apart keep the three Transmissions in three Messages.
	observed := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	t1 := accept("t1", f.networks[0], "n0", observed, 10)
	t2 := accept("t2", f.networks[0], "n0", observed.Add(time.Hour), 30)
	t3 := accept("t3", f.networks[1], "n1", observed.Add(2*time.Hour), 10)
	before1, before2, before3 := readTarget(t, f, t1.TransmissionID), readTarget(t, f, t2.TransmissionID), readTarget(t, f, t3.TransmissionID)
	if before1.unit.UUID != unitA.ID || before2.unit.UUID != unitB.ID || before3.unit.UUID != unitC.ID {
		t.Fatalf("intake resolutions = %+v %+v %+v", before1, before2, before3)
	}

	if _, err := f.rules.UpdateRule(t.Context(), f.proof, ruleA.ID, ruleA.Revision, ruleInput(f.networks[0], unitB.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	var scopeID uuid.UUID
	var scopeRevision int64
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var scopes int
		var network, unit uuid.NullUUID
		var protocol string
		var from, to int64
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) OVER (),id,assignment_revision,network_id,protocol,
			address_from,address_to,unit_id FROM assignment_pending_scopes WHERE state IN ('queued','running')
			ORDER BY recorded_at DESC LIMIT 1`).Scan(&scopes, &scopeID, &scopeRevision, &network, &protocol, &from, &to, &unit); err != nil {
			return err
		}
		if scopes != 1 || network.UUID != f.networks[0] || protocol != "pocsag" || from != 10 || to != 20 || unit.Valid {
			t.Fatalf("rule change scopes: count=%d network=%v protocol=%s range=%d..%d unit=%v; want one selector scope on Network 0, 10..20",
				scopes, network, protocol, from, to, unit)
		}
		statuses := map[uuid.UUID]string{}
		for _, message := range []uuid.UUID{t1.MessageID, t2.MessageID, t3.MessageID} {
			var status string
			if err := executor.QueryRowContext(tx.Context(), `SELECT correction_status FROM message_units
				WHERE message_id=$1 AND unit_id IS NOT NULL`, message).Scan(&status); err != nil {
				return err
			}
			statuses[message] = status
		}
		if statuses[t1.MessageID] != "correcting" || statuses[t2.MessageID] != "current" || statuses[t3.MessageID] != "current" {
			t.Fatalf("cache statuses after the rule change = %v; want only the affected Message correcting", statuses)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if drainCorrections(t, runner) == 0 {
		t.Fatal("selector correction did not run")
	}
	after1, after2, after3 := readTarget(t, f, t1.TransmissionID), readTarget(t, f, t2.TransmissionID), readTarget(t, f, t3.TransmissionID)
	if after1.unit.UUID != unitB.ID || after1.revision != scopeRevision {
		t.Fatalf("affected target after correction = %+v; want Unit B at revision %d", after1, scopeRevision)
	}
	if after2 != before2 || after3 != before3 {
		t.Fatalf("unrelated targets were touched: %+v -> %+v, %+v -> %+v", before2, after2, before3, after3)
	}
	status, err := f.rules.ReadCorrection(t.Context(), f.proof, scopeID)
	if err != nil || status.State != "completed" || status.Remaining != 0 {
		t.Fatalf("selector scope status = %+v err=%v; want completed with nothing remaining", status, err)
	}
	rows := readMessageUnits(t, f, t1.MessageID)
	if len(rows) != 1 || rows[0].unit.UUID != unitB.ID || rows[0].status != "current" {
		t.Fatalf("cache after correction = %+v; want one current Unit B row", rows)
	}

	suppressed, err := f.rules.CreateSuppression(t.Context(), f.proof, uuid.New(),
		assignments.CreateSuppressionInput{TransmissionID: t2.TransmissionID, TargetPosition: 0}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var transmission uuid.NullUUID
		var position *int
		var network uuid.NullUUID
		if err := executor.QueryRowContext(tx.Context(), `SELECT transmission_id,target_position,network_id FROM assignment_pending_scopes
			WHERE id=$1`, suppressed.Correction.ID).Scan(&transmission, &position, &network); err != nil {
			return err
		}
		if transmission.UUID != t2.TransmissionID || position == nil || *position != 0 || network.Valid {
			t.Fatalf("suppression scope = %v %v %v; want exactly the suppressed target", transmission, position, network)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if drainCorrections(t, runner) == 0 {
		t.Fatal("suppression correction did not run")
	}
	if rows := readMessageUnits(t, f, t2.MessageID); len(rows) != 0 {
		t.Fatalf("suppressed target still cached: %+v", rows)
	}
	if again := readTarget(t, f, t3.TransmissionID); again != before3 {
		t.Fatalf("suppression touched a target of another Network: %+v", again)
	}

	deleted, err := f.units.ReadUnit(t.Context(), f.proof, unitC.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.units.DeleteUnit(t.Context(), f.proof, unitC.ID, deleted.Revision, checkedMutation); err != nil {
		t.Fatal(err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var unit uuid.NullUUID
		if err := executor.QueryRowContext(tx.Context(), `SELECT unit_id FROM assignment_pending_scopes
			WHERE state IN ('queued','running') ORDER BY recorded_at DESC LIMIT 1`).Scan(&unit); err != nil {
			return err
		}
		if unit.UUID != unitC.ID {
			t.Fatalf("Unit deletion scope unit=%v, want %s", unit, unitC.ID)
		}
		var dirtyTables int
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM information_schema.tables
			WHERE table_name='correction_dirty_transmissions'`).Scan(&dirtyTables); err != nil {
			return err
		}
		if dirtyTables != 0 {
			t.Fatal("dirty capture table still exists")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if drainCorrections(t, runner) == 0 {
		t.Fatal("Unit deletion correction did not run")
	}
	if final := readTarget(t, f, t3.TransmissionID); final.status != "unassigned" || final.unit.Valid || final.revision <= before3.revision {
		t.Fatalf("deleted Unit's target = %+v; want unassigned at a newer revision", final)
	}
	if rows := readMessageUnits(t, f, t3.MessageID); len(rows) != 0 {
		t.Fatalf("deleted Unit still cached: %+v", rows)
	}
}
