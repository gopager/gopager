package borrowed_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// requireFactsAtMessageTime expects exactly want facts of one type that name the Transmission (or
// other JSON key) and the Message, each with a messageTime equal to the Message's stored
// first_observed_at.
func requireFactsAtMessageTime(t *testing.T, store *postgres.Store, factType, key, value string,
	message uuid.UUID, want int,
) {
	t.Helper()
	var total, matching int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*),
			count(*) FILTER (WHERE (e.envelope->'data'->>'messageTime')::timestamptz = m.first_observed_at)
			FROM domain_events e, messages m
			WHERE m.id=$1 AND e.envelope->>'type'=$2 AND e.envelope->'data'->>$3=$4
			AND e.envelope->'data'->>'messageId'=$1::text`, message, factType, key, value).Scan(&total, &matching)
	}); err != nil {
		t.Fatal(err)
	}
	if total != want || matching != want {
		t.Fatalf("%s facts of Message %s: %d, at the Message time: %d, want %d", factType, message, total, matching, want)
	}
}

// messageFactData returns the data payload of the one fact of a type for a Transmission.
func messageFactData(t *testing.T, store *postgres.Store, factType string, transmission uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT envelope->'data' FROM domain_events
			WHERE envelope->>'type'=$1 AND envelope->'data'->>'transmissionId'=$2`,
			factType, transmission.String()).Scan(&raw)
	}); err != nil {
		t.Fatalf("%s fact of %s: %v", factType, transmission, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

// requireMessageTimes expects the fact's messageTime and, when previous is the zero time, no
// previousMessageTime.
func requireMessageTimes(t *testing.T, name string, data map[string]any, current, previous time.Time) {
	t.Helper()
	if data["messageTime"] != current.UTC().Format(time.RFC3339) {
		t.Fatalf("%s messageTime=%v, want %s", name, data["messageTime"], current.UTC().Format(time.RFC3339))
	}
	if previous.IsZero() {
		if _, set := data["previousMessageTime"]; set {
			t.Fatalf("%s carries previousMessageTime=%v without moving the Message time", name, data["previousMessageTime"])
		}
		return
	}
	if data["previousMessageTime"] != previous.UTC().Format(time.RFC3339) {
		t.Fatalf("%s previousMessageTime=%v, want %s", name, data["previousMessageTime"], previous.UTC().Format(time.RFC3339))
	}
}

// TestGroupingFactsCarryMessageTime covers a created Message, a join that keeps the Message time,
// a late join that moves it earlier, and a merge: the facts carry the Message time after the
// change and the latest time the replaced Messages had before it.
func TestGroupingFactsCarryMessageTime(t *testing.T) {
	t.Parallel()
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
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'message time network','pocsag',a.id FROM a`, network); err != nil {
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

	created := submit("created", base)
	requireMessageTimes(t, "message.created", messageFactData(t, store, "message.created", created.TransmissionID), base, time.Time{})

	later := submit("later", base.Add(10*time.Second))
	requireMessageTimes(t, "non-late join", messageFactData(t, store, "message.transmission-joined", later.TransmissionID), base, time.Time{})

	earlier := submit("earlier", base.Add(-10*time.Second))
	requireMessageTimes(t, "late join", messageFactData(t, store, "message.transmission-joined", earlier.TransmissionID),
		base.Add(-10*time.Second), base)

	far := submit("far", base.Add(60*time.Second))
	if far.MessageID == created.MessageID {
		t.Fatal("transmission beyond the grouping window joined the first Message")
	}
	requireMessageTimes(t, "second message.created", messageFactData(t, store, "message.created", far.TransmissionID),
		base.Add(60*time.Second), time.Time{})

	bridge := submit("bridge", base.Add(35*time.Second))
	oldest, latest := base.Add(-10*time.Second), base.Add(60*time.Second)
	requireMessageTimes(t, "merge", messageFactData(t, store, "message.merged", bridge.TransmissionID), oldest, latest)
	requireMessageTimes(t, "join of the merge", messageFactData(t, store, "message.transmission-joined", bridge.TransmissionID), oldest, latest)
}
