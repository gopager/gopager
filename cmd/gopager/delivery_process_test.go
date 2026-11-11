package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

// TestDeliveryProcessWithoutTargetProviderIsVisiblyUnavailable starts the real command with the
// api and worker roles and no Delivery target provider: a Subscription naming an enabled
// instance is rejected as unavailable, and the worker's fan-out consumer consumes a deliverable
// fact without creating a Delivery.
func TestDeliveryProcessWithoutTargetProviderIsVisiblyUnavailable(t *testing.T) {
	t.Parallel()
	if os.Getenv("GOPAGER_TEST_DATABASE_URL") == "" {
		t.Skip("GOPAGER_TEST_DATABASE_URL is not set; command/PostgreSQL process evidence not run")
	}
	dsn := testpostgres.EmptyDatabase(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	stop := startCommand(t, "api,worker", dsn, address, "--event-poll-interval=100ms")
	defer stop()

	store, _, err := postgres.NewStore(dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	fixture := &queryFixture{t: t, store: store, origin: "http://" + address, client: http.DefaultClient}
	bearer, _ := fixture.caller([]string{"subscription.manage-own", "message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	instance := processInstance(t, store)

	body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "commandId": uuid.New(), "kind": "personal",
		"state": "active", "eventTypes": []string{"message.created"}, "targets": []any{target(instance)}})
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, fixture.origin+"/api/v1/subscriptions",
		strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", bearer)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var rejected struct {
		Code       string         `json:"code"`
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Code != "AUTHORITY_UNAVAILABLE" || rejected.Parameters["status"] != "unavailable" ||
		rejected.Parameters["instanceId"] != instance.String() {
		t.Fatalf("status %d, body %+v", response.StatusCode, rejected)
	}

	fact := uuid.New()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	envelope, _ := json.Marshal(map[string]any{"schemaVersion": 1, "domainEventId": fact, "type": "message.created",
		"producer":     map[string]string{"domain": "grouping", "version": "test1.0"},
		"aggregateRef": map[string]any{"type": "message", "id": uuid.New()}, "aggregateRevision": "1",
		"occurredAt": now, "recordedAt": now, "data": map[string]any{"messageId": uuid.New(), "messageTime": now},
		"classification": "operational", "replayMode": "normal"})
	fixture.exec(`INSERT INTO domain_events (id, envelope) VALUES ($1, $2::jsonb)`, fact, string(envelope))
	deadline := time.Now().Add(15 * time.Second)
	for {
		var consumed bool
		var deliveries int
		if err := store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT EXISTS (SELECT 1 FROM consumer_offsets o
				JOIN domain_events e ON e.id = $2 WHERE o.consumer = $1 AND (o.last_tx_id, o.last_seq) >= (e.tx_id, e.seq))`,
				delivery.FanOutConsumerName, fact).Scan(&consumed); err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM deliveries`).Scan(&deliveries)
		}); err != nil {
			t.Fatal(err)
		}
		if consumed {
			if deliveries != 0 {
				t.Fatalf("%d Deliveries without any Subscription", deliveries)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the worker's Delivery fan-out did not consume the fact")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// processInstance stores an enabled processing instance through the instance store.
func processInstance(t *testing.T, store *postgres.Store) uuid.UUID {
	t.Helper()
	instances, err := processingstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	authority := authz.Authority{PrincipalID: actor.String(), Revision: 1, Active: true,
		Grants: []authz.Grant{{SubjectID: actor.String(), SubjectKind: "principal", Scope: authz.Scope{Kind: "all"},
			Actions: []string{"plugin.configure"}}}}
	config, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
	if err != nil {
		t.Fatal(err)
	}
	value := processing.InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: true, ExecutionPrincipalID: uuid.New(),
		CreatedBy: actor, Configuration: config, Main: processing.ProvisionedArtifact{ManifestPath: "target/manifest.json",
			Pin: processing.ArtifactPin{PluginID: "target", Version: "1", CapabilityID: "deliver", Digest: strings.Repeat("c", 64)}}}
	if err := store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		value.CreatedAt = now
		return instances.SaveInstance(tx.Context(), tx, authority, value, 0)
	}); err != nil {
		t.Fatal(err)
	}
	return value.ID
}
