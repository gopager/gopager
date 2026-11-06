package borrowed_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// TestGroupingWindowIsInclusiveAndIndexed pins the boundary of the default 30 second window
// (exactly the window groups, one second more does not) and the index that makes the
// candidate lookup range-searchable by observation time.
func TestGroupingWindowIsInclusiveAndIndexed(t *testing.T) {
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	network, gateway := uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Window agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'window network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state) VALUES($1,'active')`, gateway); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,priority)
			VALUES($1,$2,'default',$3,0)`, uuid.New(), gateway, network)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	submit := func(id string, observed time.Time) ingest.Receipt {
		t.Helper()
		wire := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: observed.Format(time.RFC3339), GatewayNetworkIdentifier: "default",
			Payload: json.RawMessage(`{"form":"tone-only"}`), TransmissionTargets: []protocols.Target{}}
		encoded, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		var receipt ingest.Receipt
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			accepted, err := store.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			validated, err := wire.Validate(len(encoded), accepted)
			if err != nil {
				return err
			}
			validated.ResolvedNetwork, validated.Resolution = network, "declared"
			receipt, err = repository.AcceptTransmission(tx.Context(), tx, gateway, validated, accepted,
				assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{}})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	anchor := submit("anchor", base)
	atWindow := submit("at-window", base.Add(30*time.Second))
	beyond := submit("beyond", base.Add(-31*time.Second))
	if atWindow.MessageID != anchor.MessageID {
		t.Fatal("a Transmission exactly one window after the member must group")
	}
	if beyond.MessageID == anchor.MessageID {
		t.Fatal("a Transmission one second beyond the window before the member must not group")
	}
	var definition string
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT indexdef FROM pg_indexes
			WHERE tablename='messages' AND indexname='messages_identity'`).Scan(&definition)
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "identity_value, last_observed_at)") {
		t.Fatalf("messages_identity must end with last_observed_at: %s", definition)
	}
}
