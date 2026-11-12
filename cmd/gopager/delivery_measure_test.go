package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Measurement volumes: one event is handled against measureSubscribers x measurePerSubscriber
// active Subscriptions, over measureMessages seeded Messages with published attributes.
const (
	measureSubscribers   = 5
	measurePerSubscriber = 4
	measureMessages      = 300
	measureEvents        = 50
	measureCompileRounds = 200
)

var measureCities = []string{"Aachen", "Bonn", "Cologne"}

// measureEnabled reports whether the opt-in measurement was requested. The run is slow and its
// numbers depend on the machine, so it never runs as part of the regular suites.
func measureEnabled() bool {
	return os.Getenv("GOPAGER_INTEGRATION") == "1" && os.Getenv("GOPAGER_MEASURE") == "1"
}

// withTx runs the callback in a transaction and fails the test on an error.
func (f *deliveryFixture) withTx(callback func(tx platform.TransactionContext) error) {
	f.t.Helper()
	if err := f.store.WithinTransaction(context.Background(), callback); err != nil {
		f.t.Fatal(err)
	}
}

// measureSeed is the seeded world: the Subscriptions and the Messages the events refer to.
type measureSeed struct {
	subscriptions []delivery.Subscription
	messages      []uuid.UUID
}

// seedMeasurement creates the subscribers, Layouts, Subscriptions and Messages with results. The
// filters mix level thresholds, cities and conjunctions so that a share of the Subscriptions
// matches each event and renders its Layout.
func (f *deliveryFixture) seedMeasurement() measureSeed {
	f.t.Helper()
	instance := f.instance(true)
	f.attributeSchema("incident", incidentFields)
	workflow := f.resultWorkflow()
	manager := f.layoutManager()
	var layouts []uuid.UUID
	for range 4 {
		layouts = append(layouts, f.layoutCreated(manager, layoutBody("text", []any{
			textBlock("Alarm"),
			map[string]any{"kind": "field", "field": attributeRef("level"), "label": "Level"},
			map[string]any{"kind": "field", "field": attributeRef("city"), "label": "City"},
			map[string]any{"kind": "field", "field": map[string]any{"namespace": "message", "path": []string{"text"}}, "label": "Text"},
		})).ID)
	}
	var seed measureSeed
	for subscriber := range measureSubscribers {
		bearer, _ := f.caller([]string{"subscription.manage-own", "message.read", "delivery.read"}, authz.Scope{Kind: "all"},
			authz.MessageFieldGroups)
		for n := range measurePerSubscriber {
			i := subscriber*measurePerSubscriber + n
			level, city := levelAtLeast(fmt.Sprint(1+i%3)), cityIs(measureCities[i%len(measureCities)])
			filter := level
			switch i % 3 {
			case 1:
				filter = city
			case 2:
				filter = map[string]any{"op": "or", "args": []any{level, city}}
			}
			seed.subscriptions = append(seed.subscriptions, f.subCreated(bearer, subInput(instance, map[string]any{
				"filter": filter, "layoutId": layouts[i%len(layouts)], "splitByUnit": i%5 == 0})))
		}
	}
	units := []uuid.UUID{f.unit("Engine 1", nil), f.unit("Engine 2", nil), f.unit("Ladder 1", nil)}
	for i := range measureMessages {
		message := f.message(f.minuteAgo(), fmt.Sprintf("ALARM %d", i), units[i%len(units)], units[(i+1)%len(units)])
		f.publishResult(workflow, message, untargetedResult(
			attributeEntry("level", "integer", fmt.Sprintf(`"%d"`, 1+i%5)),
			attributeEntry("city", "string", `"`+measureCities[i%len(measureCities)]+`"`)), `["derived","payload"]`)
		seed.messages = append(seed.messages, message)
	}
	return seed
}

// eventRecord records a message.created fact for the Message and returns it as the runner would
// hand it to the consumer.
func (f *deliveryFixture) eventRecord(message uuid.UUID) events.Record {
	f.t.Helper()
	id := f.messageFact("message.created", message, nil)
	envelope := f.subStrings(`SELECT envelope::text FROM domain_events WHERE id = $1`, id)
	return events.Record{ID: id, Type: "message.created", Envelope: json.RawMessage(envelope[0])}
}

// statementStats says whether pg_stat_statements is loaded into the server, so that statements can be counted.
type statementStats struct{ available bool }

// statementCount is the number of executions of one normalized statement.
type statementCount struct {
	query string
	calls int64
}

func (f *deliveryFixture) enableStatementStats() statementStats {
	f.t.Helper()
	err := f.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err = executor.ExecContext(tx.Context(), `CREATE EXTENSION IF NOT EXISTS pg_stat_statements`); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `SELECT pg_stat_statements_reset(0, (SELECT oid FROM pg_database
			WHERE datname = current_database()), 0)`)
		return err
	})
	if err != nil {
		f.t.Logf("statement counting unavailable (pg_stat_statements): %v", err)
		return statementStats{}
	}
	return statementStats{available: true}
}

func (f *deliveryFixture) resetStatements() {
	f.t.Helper()
	f.exec(`SELECT pg_stat_statements_reset(0, (SELECT oid FROM pg_database WHERE datname = current_database()), 0)`)
}

// statements returns the statements run in the current database since the last reset, without
// the measurement's own queries.
func (f *deliveryFixture) statements() []statementCount {
	f.t.Helper()
	var out []statementCount
	f.withTx(func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), `SELECT query, calls FROM pg_stat_statements
			WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
			AND query NOT LIKE '%pg_stat_statements%'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item statementCount
			if err := rows.Scan(&item.query, &item.calls); err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out
}

// statementKind classifies a statement by its leading keyword; transaction control is grouped.
func statementKind(query string) string {
	word := strings.ToUpper(strings.Fields(strings.TrimSpace(query))[0])
	switch word {
	case "BEGIN", "COMMIT", "ROLLBACK", "START", "SAVEPOINT", "RELEASE":
		return "TRANSACTION CONTROL"
	case "WITH":
		return "SELECT (WITH)"
	}
	return word
}

// percentile returns the p-quantile (0..1) of the sorted durations by nearest rank.
func percentile(sorted []time.Duration, p float64) time.Duration {
	return sorted[min(len(sorted)-1, int(float64(len(sorted))*p))]
}

func summarize(label string, samples []time.Duration) time.Duration {
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	var sum time.Duration
	for _, sample := range sorted {
		sum += sample
	}
	median := percentile(sorted, 0.5)
	fmt.Printf("%-44s n=%d median=%v p90=%v mean=%v min=%v max=%v\n", label, len(sorted), median, percentile(sorted, 0.9),
		sum/time.Duration(len(sorted)), sorted[0], sorted[len(sorted)-1])
	return median
}

// TestMeasureFanOutPerEvent measures what one message.created fact costs in Delivery fan-out
// against 20 attribute Subscriptions of 5 subscribers: wall time, statements, the share of
// compiling filters and Layouts, and the time a projection shared per subscriber authority would
// save. It prints its findings and asserts nothing; run it with GOPAGER_INTEGRATION=1 GOPAGER_MEASURE=1.
func TestMeasureFanOutPerEvent(t *testing.T) {
	t.Parallel()
	if !measureEnabled() {
		t.Skip("fan-out measurement disabled; run with GOPAGER_INTEGRATION=1 GOPAGER_MEASURE=1")
	}
	f := newDeliveryFixture(t, delivery.AdmissionLimits{})
	seed := f.seedMeasurement()
	runtime, err := composeDelivery(f.core)
	if err != nil {
		t.Fatal(err)
	}
	stats := f.enableStatementStats()
	rng := rand.New(rand.NewPCG(1, 2))
	picks := make([]uuid.UUID, measureEvents)
	for i := range picks {
		picks[i] = seed.messages[rng.IntN(len(seed.messages))]
	}

	// Warm up caches and pools with events that are not measured.
	for _, message := range picks[:5] {
		record := f.eventRecord(message)
		f.withTx(func(tx platform.TransactionContext) error { return runtime.fanOut.Handle(tx.Context(), tx, record) })
	}

	var handle []time.Duration
	kinds := map[string]int64{}
	queries := map[string]int64{}
	for _, message := range picks {
		record := f.eventRecord(message)
		if stats.available {
			f.resetStatements()
		}
		var elapsed time.Duration
		f.withTx(func(tx platform.TransactionContext) error {
			start := time.Now()
			err := runtime.fanOut.Handle(tx.Context(), tx, record)
			elapsed = time.Since(start)
			return err
		})
		handle = append(handle, elapsed)
		if stats.available {
			for _, item := range f.statements() {
				kinds[statementKind(item.query)] += item.calls
				queries[item.query] += item.calls
			}
		}
	}
	created := f.subStrings(`SELECT count(*)::text FROM deliveries`)
	fmt.Printf("\n== seeded: %d subscribers x %d Subscriptions, %d Messages with results, %s Deliveries created in total\n",
		measureSubscribers, measurePerSubscriber, measureMessages, created[0])
	median := summarize("FanOut.Handle per event (inside tx)", handle)

	if stats.available {
		fmt.Printf("\n== statements per event (mean over %d events, includes transaction control)\n", measureEvents)
		var total int64
		for _, kind := range sortedKeys(kinds) {
			fmt.Printf("  %-22s %8.1f\n", kind, float64(kinds[kind])/measureEvents)
			total += kinds[kind]
		}
		fmt.Printf("  %-22s %8.1f\n", "TOTAL", float64(total)/measureEvents)
		fmt.Printf("\n== distinct statements per event (calls/event)\n")
		for _, query := range sortedKeys(queries) {
			fmt.Printf("  %7.1f  %s\n", float64(queries[query])/measureEvents, oneLine(query, 150))
		}
	}

	measureCompile(t, f, runtime, seed, median)
	measureProjection(t, f, runtime, seed, picks, median)
}

func sortedKeys(counts map[string]int64) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return counts[keys[i]] > counts[keys[j]] || counts[keys[i]] == counts[keys[j]] && keys[i] < keys[j]
	})
	return keys
}

func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

// measureCompile times CompileFilter and CompileLayout per Subscription in isolation, against
// the registry of the same transaction, and relates one event's compilations to the fan-out time.
func measureCompile(t *testing.T, f *deliveryFixture, runtime *deliveryRuntime, seed measureSeed, fanOut time.Duration) {
	t.Helper()
	var filterTime, layoutTime, registryTime, resolveTime time.Duration
	f.withTx(func(tx platform.TransactionContext) error {
		ctx := tx.Context()
		start := time.Now()
		registry, err := runtime.deps.Fields.Registry(ctx, tx)
		registryTime = time.Since(start)
		if err != nil {
			return err
		}
		declarations := registry.Declarations(fields.ScopeDelivery, true)
		for _, subscription := range seed.subscriptions {
			start = time.Now()
			resolved, err := runtime.deps.Layouts.ResolveLayout(ctx, tx, processing.LayoutRequest{LayoutID: subscription.LayoutID})
			resolveTime += time.Since(start)
			if err != nil {
				return err
			}
			start = time.Now()
			for range measureCompileRounds {
				if _, err := fields.CompileFilter(*subscription.Filter, declarations); err != nil {
					return err
				}
			}
			filterTime += time.Since(start) / measureCompileRounds
			start = time.Now()
			for range measureCompileRounds {
				if _, err := processing.CompileLayout(ctx, resolved.Layout, declarations); err != nil {
					return err
				}
			}
			layoutTime += time.Since(start) / measureCompileRounds
		}
		return nil
	})
	rendered := f.subStrings(`SELECT count(*)::text FROM deliveries WHERE state <> 'failed'`)[0]
	withDeliveries := f.subStrings(`SELECT count(DISTINCT domain_event_id)::text FROM deliveries`)[0]
	fmt.Printf("\n== compile cost per event over the %d Subscriptions (isolated, mean of %d rounds)\n", len(seed.subscriptions), measureCompileRounds)
	fmt.Printf("  CompileFilter, all Subscriptions:        %v\n", filterTime)
	fmt.Printf("  CompileLayout, all Subscriptions (health): %v\n", layoutTime)
	fmt.Printf("  Registry load (one per Subscription in fan-out, SQL-backed): %v each\n", registryTime)
	fmt.Printf("  ResolveLayout, all Subscriptions (SQL-backed): %v\n", resolveTime)
	fmt.Printf("  Deliveries with a rendering: %s over %s events with Deliveries; the render path compiles the Layout again per rendered Delivery\n", rendered, withDeliveries)
	health := filterTime + layoutTime
	fmt.Printf("  health compile (filter+layout): %v = %.1f %% of the %v fan-out median\n", health, 100*float64(health)/float64(fanOut), fanOut)
}

// measureProjection times ProjectForDelivery per call and compares 20 calls (one per
// Subscription) with 5 calls (one per subscriber authority) for the same events.
func measureProjection(t *testing.T, f *deliveryFixture, runtime *deliveryRuntime, seed measureSeed, picks []uuid.UUID, fanOut time.Duration) {
	t.Helper()
	var subscribers []uuid.UUID
	for _, subscription := range seed.subscriptions {
		if !slices.Contains(subscribers, subscription.SubscriberID) {
			subscribers = append(subscribers, subscription.SubscriberID)
		}
	}
	var call, twenty, five, resolve20, resolve5 []time.Duration
	f.withTx(func(tx platform.TransactionContext) error {
		ctx := tx.Context()
		now, err := f.store.Now(ctx, tx)
		if err != nil {
			return err
		}
		authorities := map[uuid.UUID]authz.Authority{}
		for _, subscriber := range subscribers {
			if authorities[subscriber], err = f.core.resolver.ResolvePrincipalAuthority(ctx, tx, subscriber); err != nil {
				return err
			}
		}
		project := func(subscriber, message uuid.UUID) (time.Duration, error) {
			start := time.Now()
			_, _, err := runtime.deps.Projector.ProjectForDelivery(ctx, tx, authorities[subscriber], now, message)
			return time.Since(start), err
		}
		for _, message := range picks {
			var elapsed time.Duration
			for _, subscription := range seed.subscriptions {
				took, err := project(subscription.SubscriberID, message)
				if err != nil {
					return err
				}
				call, elapsed = append(call, took), elapsed+took
			}
			twenty = append(twenty, elapsed)
			elapsed = 0
			for _, subscriber := range subscribers {
				took, err := project(subscriber, message)
				if err != nil {
					return err
				}
				elapsed += took
			}
			five = append(five, elapsed)
		}
		for range picks {
			start := time.Now()
			for _, subscription := range seed.subscriptions {
				if _, err := f.core.resolver.ResolvePrincipalAuthority(ctx, tx, subscription.SubscriberID); err != nil {
					return err
				}
			}
			resolve20 = append(resolve20, time.Since(start))
			start = time.Now()
			for _, subscriber := range subscribers {
				if _, err := f.core.resolver.ResolvePrincipalAuthority(ctx, tx, subscriber); err != nil {
					return err
				}
			}
			resolve5 = append(resolve5, time.Since(start))
		}
		return nil
	})
	fmt.Printf("\n== projection (ProjectForDelivery), %d calls per event today, %d distinct authorities\n", len(seed.subscriptions), len(subscribers))
	summarize("one ProjectForDelivery call", call)
	m20 := summarize("20 calls per event", twenty)
	m5 := summarize("5 calls per event (shared per authority)", five)
	saved := m20 - m5
	fmt.Printf("  saving per event: %v = %.1f %% of the %v fan-out median\n", saved, 100*float64(saved)/float64(fanOut), fanOut)
	r20 := summarize("20 ResolvePrincipalAuthority per event", resolve20)
	r5 := summarize("5 ResolvePrincipalAuthority per event", resolve5)
	fmt.Printf("  supplemental saving if authority resolution were shared too: %v = %.1f %%\n", r20-r5, 100*float64(r20-r5)/float64(fanOut))
}
