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

// assignmentFacts returns the data payloads of the assignment.changed facts of one Transmission in
// log order.
func assignmentFacts(t *testing.T, f matcherFixture, transmission uuid.UUID) []map[string]any {
	t.Helper()
	var facts []map[string]any
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), `SELECT envelope->'data' FROM domain_events
			WHERE envelope->>'type'='assignment.changed' AND envelope->'data'->>'transmissionId'=$1
			ORDER BY seq`, transmission.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var data map[string]any
			if err := json.Unmarshal(raw, &data); err != nil {
				return err
			}
			facts = append(facts, data)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return facts
}

// TestAssignmentChangedIsOneFactPerTransmissionAtIntakeAndCorrection expects a two-target
// Transmission to emit exactly one assignment.changed fact at intake (reason created, both
// targets in position order) and exactly one more when a rule change re-resolves both targets
// (reason corrected, both targets with before and after).
func TestAssignmentChangedIsOneFactPerTransmissionAtIntakeAndCorrection(t *testing.T) {
	f := newMatcherFixture(t)
	gatewayID := uuid.New()
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency, Name: "A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
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
			(id,gateway_id,gateway_network_identifier,network_id,is_default) VALUES($1,$2,'n0',$3,true)`,
			uuid.New(), gatewayID, f.networks[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	intake, err := ingeststore.New(f.store, f.facts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "two-targets", Protocol: protocols.POCSAG,
		ObservationTime: time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC).Format(time.RFC3339), GatewayNetworkIdentifier: "n0",
		Payload: json.RawMessage(`{"form":"tone-only"}`),
		TransmissionTargets: []protocols.Target{
			{Address: protocols.NumericAddress(12), Function: &function},
			{Address: protocols.NumericAddress(13), Function: &function}}}
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
		validated.ResolvedNetwork, validated.Resolution = f.networks[0], "declared"
		matched, err := f.rules.MatchTargets(tx.Context(), tx, f.networks[0], protocols.POCSAG, validated.ObservedAt, validated.TransmissionTargets)
		if err != nil {
			return err
		}
		receipt, err = intake.AcceptTransmission(tx.Context(), tx, gatewayID, validated, accepted, matched)
		return err
	}); err != nil {
		t.Fatalf("intake: %v (%s)", err, causeChain(err))
	}
	created := assignmentFacts(t, f, receipt.TransmissionID)
	if len(created) != 1 || created[0]["reason"] != "created" {
		t.Fatalf("intake facts = %v, want exactly one created fact", created)
	}
	if targets, _ := created[0]["targets"].([]any); len(targets) != 2 ||
		targets[0].(map[string]any)["position"] != float64(0) || targets[1].(map[string]any)["position"] != float64(1) ||
		targets[0].(map[string]any)["status"] != "unassigned" {
		t.Fatalf("intake targets = %v", created[0]["targets"])
	}

	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unit.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	drainCorrections(t, runner)
	all := assignmentFacts(t, f, receipt.TransmissionID)
	if len(all) != 2 || all[1]["reason"] != "corrected" {
		t.Fatalf("facts after correction = %v, want created plus one corrected fact", all)
	}
	targets, _ := all[1]["targets"].([]any)
	if len(targets) != 2 {
		t.Fatalf("corrected targets = %v, want both positions", all[1]["targets"])
	}
	first := targets[0].(map[string]any)
	if first["before"].(map[string]any)["status"] != "unassigned" || first["after"].(map[string]any)["status"] != "assigned" {
		t.Fatalf("first corrected target = %v", first)
	}
}
