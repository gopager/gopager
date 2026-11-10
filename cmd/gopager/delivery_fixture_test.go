package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// controlledTargets is a Delivery target provider whose descriptors and outcomes a test controls.
// It records every invocation.
type controlledTargets struct {
	mu          sync.Mutex
	unavailable map[uuid.UUID]bool
	descriptor  func(instance processing.InstanceConfiguration) delivery.TargetDescriptor
	// outcome answers each invocation; nil accepts.
	outcome func(invocation delivery.AttemptInvocation) (delivery.AttemptResult, error)
	// wait, when set, runs first with the call's context; a non-nil error is the call's error.
	wait        func(ctx context.Context, invocation delivery.AttemptInvocation) error
	invocations []delivery.AttemptInvocation
}

func newControlledTargets() *controlledTargets {
	return &controlledTargets{unavailable: map[uuid.UUID]bool{}}
}

// Describe implements delivery.Targets with a text target that requires a string recipient.
func (c *controlledTargets) Describe(_ context.Context, instance processing.InstanceConfiguration) (delivery.TargetDescriptor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unavailable[instance.ID] {
		return delivery.TargetDescriptor{}, delivery.ErrTargetUnavailable
	}
	if c.descriptor != nil {
		return c.descriptor(instance), nil
	}
	return delivery.TargetDescriptor{Version: delivery.DescriptorVersion, InstanceID: instance.ID,
		InstanceRevision: instance.Revision, CapabilityKind: delivery.CapabilityDelivery, ConfigurationAuthority: "admin",
		OutputKinds:  []string{processing.LayoutText, processing.LayoutMarkdown, processing.LayoutJSON},
		Inputs:       []delivery.InputField{{Name: "recipient", Kind: processing.StringValue, Required: true, MaxBytes: 256}},
		Dependencies: []delivery.TargetDependency{{Kind: "network", Description: "controlled test provider"}}}, nil
}

// Attempt implements delivery.Targets.
func (c *controlledTargets) Attempt(ctx context.Context, invocation delivery.AttemptInvocation) (delivery.AttemptResult, error) {
	c.mu.Lock()
	c.invocations = append(c.invocations, invocation)
	outcome, wait := c.outcome, c.wait
	c.mu.Unlock()
	if wait != nil {
		if err := wait(ctx, invocation); err != nil {
			return delivery.AttemptResult{}, err
		}
	}
	if outcome == nil {
		return delivery.AttemptResult{Status: delivery.StatusAccepted, Code: "ok", Receipt: json.RawMessage(`{"id":"r-1"}`)}, nil
	}
	return outcome(invocation)
}

func (c *controlledTargets) calls() []delivery.AttemptInvocation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]delivery.AttemptInvocation(nil), c.invocations...)
}

func (c *controlledTargets) setOutcome(outcome func(delivery.AttemptInvocation) (delivery.AttemptResult, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outcome = outcome
}

func (c *controlledTargets) setWait(wait func(context.Context, delivery.AttemptInvocation) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wait = wait
}

func (c *controlledTargets) setUnavailable(instance uuid.UUID, unavailable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unavailable[instance] = unavailable
}

// deliveryFixture is a composed API and worker side with a controlled target provider, seeded
// master data, and helpers to record facts and drive fan-out and attempts.
type deliveryFixture struct {
	*queryFixture
	core    *core
	targets *controlledTargets
	runner  *delivery.Runner
	admin   authz.Authority
}

func newDeliveryFixture(t *testing.T, limits delivery.AdmissionLimits) *deliveryFixture {
	t.Helper()
	store := newManagementStore(t)
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	targets := newControlledTargets()
	shared := mustComposeCore(t, store)
	shared.deliveryConfig = deliveryConfig{targets: targets, limits: limits}
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	runner, err := composeDeliveryRunner(shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	query := &queryFixture{t: t, store: store, origin: origin, client: backend.Client(), gateway: uuid.New(),
		network: uuid.New(), agency: seedAgency(t, store)}
	query.exec(`INSERT INTO gateways (id, name) VALUES ($1, 'gw')`, query.gateway)
	query.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, 'Net', 'pocsag', $2)`, query.network, query.agency)
	query.exec(`INSERT INTO gateway_networks (id, gateway_id, gateway_network_identifier, network_id, priority)
		VALUES ($1, $2, 'net', $3, 0)`, uuid.New(), query.gateway, query.network)
	admin := uuid.New()
	return &deliveryFixture{queryFixture: query, core: shared, targets: targets, runner: runner,
		admin: authz.Authority{PrincipalID: admin.String(), Revision: 1, Active: true,
			Grants: []authz.Grant{{SubjectID: admin.String(), SubjectKind: "principal", Scope: authz.Scope{Kind: "all"},
				Actions: []string{"plugin.configure"}}}}}
}

// instance stores a processing instance that the controlled provider serves.
func (f *deliveryFixture) instance(enabled bool) uuid.UUID {
	f.t.Helper()
	config, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
	if err != nil {
		f.t.Fatal(err)
	}
	actor, _ := uuid.Parse(f.admin.PrincipalID)
	value := processing.InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: enabled, ExecutionPrincipalID: uuid.New(),
		CreatedBy: actor, Configuration: config, Main: processing.ProvisionedArtifact{ManifestPath: "target/manifest.json",
			Pin: processing.ArtifactPin{PluginID: "target", Version: "1", CapabilityID: "deliver", Digest: strings.Repeat("b", 64)}}}
	f.saveInstance(value, 0)
	return value.ID
}

// saveInstance stores a revision of an instance.
func (f *deliveryFixture) saveInstance(value processing.InstanceConfiguration, expected int64) {
	f.t.Helper()
	err := f.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		value.CreatedAt = now
		return f.core.processing.SaveInstance(tx.Context(), tx, f.admin, value, expected)
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

// now returns the primary clock.
func (f *deliveryFixture) now() time.Time {
	f.t.Helper()
	var now time.Time
	if err := f.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		var err error
		now, err = f.store.Now(tx.Context(), tx)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return now
}

// fact records a fact envelope of the given type and data, recorded at the primary clock unless
// recordedAt is set, in normal replay mode unless replay is set.
type factOptions struct {
	recordedAt *time.Time
	replay     string
	aggregate  string
}

func (f *deliveryFixture) fact(eventType string, data map[string]any, options factOptions) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	recorded := f.now()
	if options.recordedAt != nil {
		recorded = *options.recordedAt
	}
	replay := options.replay
	if replay == "" {
		replay = events.ReplayNormal
	}
	aggregate := options.aggregate
	if aggregate == "" {
		aggregate = "message"
	}
	entry, ok := events.LookupType(eventType)
	if !ok {
		f.t.Fatalf("unknown type %s", eventType)
	}
	envelope, err := json.Marshal(map[string]any{"schemaVersion": 1, "domainEventId": id, "type": eventType,
		"producer":     map[string]string{"domain": entry.Producer, "version": "test1.0"},
		"aggregateRef": map[string]any{"type": aggregate, "id": uuid.New()}, "aggregateRevision": "1",
		"occurredAt": recorded.Format(time.RFC3339Nano), "recordedAt": recorded.Format(time.RFC3339Nano),
		"data": data, "classification": "operational", "replayMode": replay})
	if err != nil {
		f.t.Fatal(err)
	}
	f.exec(`INSERT INTO domain_events (id, envelope) VALUES ($1, $2::jsonb)`, id, string(envelope))
	return id
}

// producedFact records the envelope a producer builds at the primary clock, exactly as encoded.
func (f *deliveryFixture) producedFact(build func(now time.Time) (any, uuid.UUID, error)) uuid.UUID {
	f.t.Helper()
	envelope, id, err := build(f.now())
	if err != nil {
		f.t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		f.t.Fatal(err)
	}
	f.exec(`INSERT INTO domain_events (id, envelope) VALUES ($1, $2::jsonb)`, id, string(encoded))
	return id
}

// messageFact records a Message fact for the Message.
func (f *deliveryFixture) messageFact(eventType string, message uuid.UUID, extra map[string]any) uuid.UUID {
	data := map[string]any{"messageId": message, "messageTime": f.now().Format(time.RFC3339Nano)}
	for key, value := range extra {
		data[key] = value
	}
	return f.fact(eventType, data, factOptions{})
}

// fanOut runs the Delivery fan-out consumer on the real event runner until it has consumed
// every committed fact.
func (f *deliveryFixture) fanOut() {
	f.t.Helper()
	runtime, err := composeDelivery(f.core)
	if err != nil {
		f.t.Fatal(err)
	}
	wakes := &manualWakes{signal: make(chan struct{}, 1)}
	runner, err := events.NewRunner(runtime.fanOut, f.core.facts, f.store, wakes, slogDiscard(), 20*time.Millisecond)
	if err != nil {
		f.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	deadline := time.Now().Add(120 * time.Second)
	for !f.fanOutCaughtUp() {
		if time.Now().After(deadline) {
			cancel()
			f.t.Fatalf("fan-out did not catch up: %v", <-done)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		f.t.Fatal(err)
	}
	if runner.Blocked() {
		f.t.Fatal("fan-out blocked")
	}
}

// fanOutCaughtUp reports whether the fan-out offset is at or after every stored fact. It reads
// the table directly instead of the gap-safe log, which hides facts while any transaction of the
// shared test cluster is still running.
func (f *deliveryFixture) fanOutCaughtUp() bool {
	var behind int
	err := f.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events e WHERE NOT EXISTS (
			SELECT 1 FROM consumer_offsets o WHERE o.consumer = $1 AND (e.tx_id, e.seq) <= (o.last_tx_id, o.last_seq))`,
			delivery.FanOutConsumerName).Scan(&behind)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return behind == 0
}

// attempt runs the Delivery runner until no due Delivery is left.
func (f *deliveryFixture) attempt() {
	f.t.Helper()
	for range 20 {
		more, err := f.runner.Tick(context.Background())
		if err != nil {
			f.t.Fatal(err)
		}
		if !more {
			return
		}
	}
}

// manualWakes is a WakeSubscriber that never signals; the runner drains at start.
type manualWakes struct{ signal chan struct{} }

func (m *manualWakes) Subscribe() (<-chan struct{}, func()) { return m.signal, func() {} }

// subscriber seeds an account allowed to manage its own Subscriptions and read its Deliveries,
// with message.read on all and the given field groups.
func (f *deliveryFixture) subscriber(fields ...string) (string, uuid.UUID) {
	return f.caller([]string{"subscription.manage-own", "message.read", "delivery.read", "delivery.retry",
		"delivery.redeliver"}, authz.Scope{Kind: "all"}, fields)
}

// subscribe creates a personal Subscription and returns its ID.
func (f *deliveryFixture) subscribe(bearer string, body map[string]any) uuid.UUID {
	f.t.Helper()
	body["schemaVersion"], body["commandId"] = 1, uuid.New()
	if _, ok := body["kind"]; !ok {
		body["kind"] = "personal"
	}
	if _, ok := body["state"]; !ok {
		body["state"] = "active"
	}
	encoded, _ := json.Marshal(body)
	response := managementRequest(f.t, f.client, f.origin, "POST", "/api/v1/subscriptions", string(encoded), "", bearer, 201)
	var created delivery.Subscription
	if err := json.Unmarshal(response.Data, &created); err != nil {
		f.t.Fatal(err)
	}
	return created.ID
}

// target returns a Subscription target body for an instance with a recipient input.
func target(instance uuid.UUID) map[string]any {
	return map[string]any{"instanceId": instance, "inputs": map[string]any{
		"recipient": map[string]any{"kind": "string", "value": "user-1"}}}
}

// deliveries lists the caller's Deliveries.
func (f *deliveryFixture) deliveries(bearer, query string) []delivery.Delivery {
	f.t.Helper()
	response := managementRequest(f.t, f.client, f.origin, "GET", "/api/v1/deliveries"+query, "", "", bearer, 200)
	var page struct {
		Items []delivery.Delivery `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal(response.Data, &page); err != nil {
		f.t.Fatal(err)
	}
	return page.Items
}

// slogDiscard returns a logger that drops everything.
func slogDiscard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
