package borrowed_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// storedResolution is what intake persisted for one Transmission.
type storedResolution struct {
	network    uuid.UUID
	resolution string
	ambiguity  []map[string]any
}

func readStoredResolution(t *testing.T, f matcherFixture, transmission uuid.UUID) storedResolution {
	t.Helper()
	var result storedResolution
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT network_id,network_resolution
			FROM transmissions WHERE id=$1`, transmission).Scan(&result.network, &result.resolution); err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), `SELECT envelope->'data' FROM domain_events
			WHERE envelope->>'type'='operation.network-ambiguous' AND envelope->'data'->>'transmissionId'=$1
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
			result.ambiguity = append(result.ambiguity, data)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

// TestIntakeResolvesOverlappingNetworksByPriorityAndRecordsAmbiguity runs the real matcher against
// two Networks of one Gateway whose rules both match the target: intake must store
// address-priority, choose the higher-priority Network and append operation.network-ambiguous.
// A target matched by no rule falls back to the highest-priority Network (priority) without the
// ambiguity fact, and a target matched by only one Network's rules selects that Network
// (inferred-address) even when the other Network has the higher priority.
func TestIntakeResolvesOverlappingNetworksByPriorityAndRecordsAmbiguity(t *testing.T) {
	f := newMatcherFixture(t)
	gateway := uuid.New()
	secret, secretHash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	unitA, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[0], Name: "Overlap A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unitB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[1], Name: "Overlap B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	// Both Networks match RIC 10..20; only network 0 matches RIC 30..40.
	for _, rule := range []struct {
		network, unit uuid.UUID
		from, to      uint64
	}{{f.networks[0], unitA.ID, 10, 20}, {f.networks[1], unitB.ID, 10, 20}, {f.networks[0], unitA.ID, 30, 40}} {
		if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(),
			ruleInput(rule.network, rule.unit, rule.from, rule.to, 1), checkedMutation); err != nil {
			t.Fatal(err)
		}
	}
	// Network 1 has the higher priority (0) of the two.
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state) VALUES($1,'active')`, gateway); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,priority) VALUES($1,$2,'a',$3,1),($4,$2,'b',$5,0)`,
			uuid.New(), gateway, f.networks[0], uuid.New(), f.networks[1]); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_credentials
			(id,gateway_id,secret_hash,pending,created_at) VALUES($1,$2,$3,false,$4)`,
			uuid.New(), gateway, secretHash[:], time.Now().UTC())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	intake, err := ingeststore.New(f.store, f.facts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	service, err := ingest.NewService(f.store, f.store, intake, f.rules)
	if err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	submit := func(id string, address uint64, declared string) ingest.Receipt {
		t.Helper()
		wire := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), GatewayNetworkIdentifier: declared,
			Payload:             json.RawMessage(`{"form":"tone-only"}`),
			TransmissionTargets: []protocols.Target{{Address: protocols.NumericAddress(address), Function: &function}}}
		encoded, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := service.Submit(t.Context(), secret, wire, len(encoded))
		if err != nil {
			t.Fatalf("submit %s: %v (%s)", id, err, causeChain(err))
		}
		return receipt
	}

	overlap := readStoredResolution(t, f, submit("overlap", 15, "").TransmissionID)
	if overlap.resolution != "address-priority" || overlap.network != f.networks[1] {
		t.Fatalf("overlap stored network=%s resolution=%s, want the higher-priority matching Network %s address-priority",
			overlap.network, overlap.resolution, f.networks[1])
	}
	if len(overlap.ambiguity) != 1 || overlap.ambiguity[0]["networkId"] != f.networks[1].String() ||
		overlap.ambiguity[0]["resolution"] != "address-priority" {
		t.Fatalf("ambiguity facts = %v, want one naming the chosen Network and resolution", overlap.ambiguity)
	}

	onlyA := readStoredResolution(t, f, submit("only-a", 35, "").TransmissionID)
	if onlyA.resolution != "inferred-address" || onlyA.network != f.networks[0] || len(onlyA.ambiguity) != 0 {
		t.Fatalf("single address match stored network=%s resolution=%s facts=%v, want %s inferred-address without fact",
			onlyA.network, onlyA.resolution, onlyA.ambiguity, f.networks[0])
	}

	none := readStoredResolution(t, f, submit("none", 99, "").TransmissionID)
	if none.resolution != "priority" || none.network != f.networks[1] || len(none.ambiguity) != 0 {
		t.Fatalf("no match stored network=%s resolution=%s facts=%v, want %s priority without fact",
			none.network, none.resolution, none.ambiguity, f.networks[1])
	}

	declared := readStoredResolution(t, f, submit("declared", 15, "a").TransmissionID)
	if declared.resolution != "declared" || declared.network != f.networks[0] || len(declared.ambiguity) != 0 {
		t.Fatalf("declared stored network=%s resolution=%s facts=%v", declared.network, declared.resolution, declared.ambiguity)
	}
}

// TestGatewayNetworkPriorityIsUniquePerGatewayInTheDatabase pins the schema guard behind the
// administration API: two Networks of one Gateway cannot share a priority, and a negative
// priority is rejected.
func TestGatewayNetworkPriorityIsUniquePerGatewayInTheDatabase(t *testing.T) {
	f := newMatcherFixture(t)
	gateway := uuid.New()
	exec := func(query string, args ...any) error {
		return f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			_, err = executor.ExecContext(tx.Context(), query, args...)
			return err
		})
	}
	if err := exec(`INSERT INTO gateways(id,state) VALUES($1,'active')`, gateway); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
		VALUES($1,$2,$3,$4,$5)`
	if err := exec(insert, uuid.New(), gateway, "a", f.networks[0], 0); err != nil {
		t.Fatal(err)
	}
	if err := exec(insert, uuid.New(), gateway, "b", f.networks[1], 0); err == nil {
		t.Fatal("a second Network with the same priority was accepted")
	}
	if err := exec(insert, uuid.New(), gateway, "b", f.networks[1], -1); err == nil {
		t.Fatal("a negative priority was accepted")
	}
	if err := exec(insert, uuid.New(), gateway, "b", f.networks[1], 1); err != nil {
		t.Fatalf("distinct priority rejected: %v", err)
	}
}
