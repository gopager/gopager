package readstore_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/persistence/ent/readstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

const (
	measureMessages = 100_000
	measureRuns     = 20
	measureLimit    = 50
)

// measureSchemas are the two attribute schemas that the two measured Workflows write.
func measureSchemas() []attributes.Schema {
	return []attributes.Schema{
		{Key: "incident", Subject: attributes.SubjectMessage, Fields: map[string]attributes.Field{
			"level": {Kind: processing.IntegerValue},
			"name":  {Kind: processing.StringValue},
			"city":  {Kind: processing.StringValue},
		}},
		{Key: "dispatch", Subject: attributes.SubjectMessage, Fields: map[string]attributes.Field{
			"code": {Kind: processing.StringValue},
		}},
	}
}

// compileMeasured checks the predicate JSON against the measured schemas.
func compileMeasured(t testing.TB, predicate string) *fields.Filter {
	t.Helper()
	var decoded processing.Predicate
	if err := processing.Decode([]byte(predicate), &decoded); err != nil {
		t.Fatalf("predicate %s: %v", predicate, err)
	}
	registry := fields.New(fields.Builtin(), attributes.NewProvider(measureSchemas(), 1))
	filter, err := fields.CompileFilter(decoded, registry.Declarations(fields.ScopeMessage, true))
	if err != nil {
		t.Fatalf("predicate %s: %v", predicate, err)
	}
	return filter
}

// seedMeasured fills the database with measureMessages Messages through set-based SQL. Message i
// has one Transmission with one assigned target of Unit i mod 10 (five Units per Agency), and
// one appearance. Workflow one writes incident.level and incident.name untargeted for 40 % of the
// Messages (i mod 5 in 0, 1) and republishes for every tenth. Workflow two writes for the Messages
// with i mod 5 in 1, 2: a target-scoped incident.city (20 % of all) or an untargeted dispatch.code
// (20 % of all). It returns the Agencies.
func seedMeasured(w *world) (agencyA, agencyB uuid.UUID) {
	agencyA, agencyB = w.agency("A"), w.agency("B")
	w.network(agencyA, "NA", "pocsag")
	w.network(agencyB, "NB", "pocsag")
	for n := 0; n < 10; n++ {
		agency := agencyA
		if n >= 5 {
			agency = agencyB
		}
		w.unit(agency, fmt.Sprintf("U%d", n), nil)
	}
	workflowOne, versionOne := w.workflow()
	workflowTwo, versionTwo := w.workflow()
	const prelude = `CREATE TEMP TABLE seed_unit ON COMMIT DROP AS
		SELECT u.id AS unit_id, u.agency_id, n.id AS network_id, (row_number() OVER (ORDER BY u.name) - 1)::int AS slot
		FROM units u JOIN networks n ON n.agency_id = u.agency_id`
	steps := []string{
		`INSERT INTO messages (id, identity_form, identity_value, first_observed_at, last_observed_at)
			SELECT md5('m' || i)::uuid, 'text', convert_to('alarm ' || i, 'UTF8'), {base} - i * interval '10 seconds',
				{base} - i * interval '10 seconds'
			FROM generate_series(1, {count}) i`,
		`INSERT INTO transmissions (id, gateway_id, network_id, submission_id, submission_fingerprint, network_resolution, protocol,
				payload, observed_at, observation_offset_seconds, accepted_at, source_revision, message_id, processing_status)
			SELECT md5('t' || i)::uuid, {gw}, s.network_id, 's' || i, decode(md5('f' || i) || md5('g' || i), 'hex'), 'declared', 'pocsag',
				'{"form":"text","value":"alarm"}'::json, {base} - i * interval '10 seconds', 0,
				{base} - i * interval '10 seconds', 1, md5('m' || i)::uuid, 'completed'
			FROM generate_series(1, {count}) i JOIN seed_unit s ON s.slot = i % 10`,
		`INSERT INTO transmission_targets (transmission_id, position, original, network_id, protocol, observed_at, accepted_at,
				address_number, function, resolution_status, unit_id, assignment_revision)
			SELECT md5('t' || i)::uuid, 0, '{}'::json, s.network_id, 'pocsag', {base} - i * interval '10 seconds',
				{base} - i * interval '10 seconds', 1000 + i % 10, 0, 'assigned', s.unit_id, 1
			FROM generate_series(1, {count}) i JOIN seed_unit s ON s.slot = i % 10`,
		`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor)
			SELECT md5('m' || i)::uuid, s.unit_id, s.agency_id, {base} - i * interval '10 seconds',
				{base} - i * interval '10 seconds'
			FROM generate_series(1, {count}) i JOIN seed_unit s ON s.slot = i % 10`,
		// Workflow one, first publication.
		`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision,
				configuration_revision, source_revision, context_revision, state, selected, due_at, created_at, updated_at)
			SELECT md5('c1' || i)::uuid, md5('t' || i)::uuid, {wf1}, {v1}, 1, 1, 1, 1, 'completed', true, {base}, {base}, {base}
			FROM generate_series(1, {count}) i WHERE i % 5 IN (0, 1)`,
		`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
			SELECT md5('r1' || i)::uuid, md5('c1' || i)::uuid, 1,
				jsonb_build_object('selected', true, 'fields', jsonb_build_object('incident', jsonb_build_object(
					'level', jsonb_build_object('state', 'present', 'value', jsonb_build_object('kind', 'integer', 'value', (i % 100)::text)),
					'name', jsonb_build_object('state', 'present', 'value', jsonb_build_object('kind', 'string', 'value', 'name' || (i % 1000)))))),
				'["derived","payload"]'::jsonb, {base} - i * interval '10 seconds' + interval '30 seconds',
				{base} - i * interval '10 seconds' + interval '30 seconds', md5('m' || i)::uuid
			FROM generate_series(1, {count}) i WHERE i % 5 IN (0, 1)`,
		// Workflow one republishes for every tenth Message, so that the latest result must be picked.
		`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision,
				configuration_revision, source_revision, context_revision, state, selected, due_at, created_at, updated_at)
			SELECT md5('c1b' || i)::uuid, md5('t' || i)::uuid, {wf1}, {v1}, 1, 1, 1, 2, 'completed', true, {base}, {base}, {base}
			FROM generate_series(1, {count}) i WHERE i % 10 = 0`,
		`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
			SELECT md5('r1b' || i)::uuid, md5('c1b' || i)::uuid, 1,
				jsonb_build_object('selected', true, 'fields', jsonb_build_object('incident', jsonb_build_object(
					'level', jsonb_build_object('state', 'present', 'value', jsonb_build_object('kind', 'integer', 'value', (i % 100)::text))))),
				'["derived","payload"]'::jsonb, {base} - i * interval '10 seconds' + interval '60 seconds',
				{base} - i * interval '10 seconds' + interval '60 seconds', md5('m' || i)::uuid
			FROM generate_series(1, {count}) i WHERE i % 10 = 0`,
		// Workflow two.
		`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision,
				configuration_revision, source_revision, context_revision, state, selected, due_at, created_at, updated_at)
			SELECT md5('c2' || i)::uuid, md5('t' || i)::uuid, {wf2}, {v2}, 1, 1, 1, 1, 'completed', true, {base}, {base}, {base}
			FROM generate_series(1, {count}) i WHERE i % 5 IN (1, 2)`,
		`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
			SELECT md5('r2' || i)::uuid, md5('c2' || i)::uuid, 1,
				CASE WHEN i % 5 = 1 THEN jsonb_build_object('selected', true, 'targetResults', jsonb_build_array(jsonb_build_object(
					'targetPosition', 0, 'fields', jsonb_build_object('incident', jsonb_build_object('city',
						jsonb_build_object('state', 'present', 'value', jsonb_build_object('kind', 'string', 'value', 'city' || (i % 50)))))))) 
				ELSE jsonb_build_object('selected', true, 'fields', jsonb_build_object('dispatch', jsonb_build_object('code',
					jsonb_build_object('state', 'present', 'value', jsonb_build_object('kind', 'string', 'value', 'code' || (i % 20)))))) END,
				'["derived","payload"]'::jsonb, {base} - i * interval '10 seconds' + interval '45 seconds',
				{base} - i * interval '10 seconds' + interval '45 seconds', md5('m' || i)::uuid
			FROM generate_series(1, {count}) i WHERE i % 5 IN (1, 2)`,
	}
	// The values are fixed literals and UUIDs of the fixture, never request input.
	replacer := strings.NewReplacer("{base}", "'"+base.Format(time.RFC3339)+"'::timestamptz", "{count}", fmt.Sprint(measureMessages),
		"{gw}", "'"+w.gateway.String()+"'::uuid", "{wf1}", "'"+workflowOne.String()+"'::uuid", "{v1}", "'"+versionOne.String()+"'::uuid",
		"{wf2}", "'"+workflowTwo.String()+"'::uuid", "{v2}", "'"+versionTwo.String()+"'::uuid")
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			w.t.Fatal(err)
		}
		if _, err := executor.ExecContext(ctx, prelude); err != nil {
			w.t.Fatal(err)
		}
		for i, step := range steps {
			if _, err := executor.ExecContext(ctx, replacer.Replace(step)); err != nil {
				w.t.Fatalf("seed step %d: %v", i, err)
			}
		}
	})
	return agencyA, agencyB
}

// measurement is one query of the plan, run for one caller.
type measurement struct {
	name   string
	caller string
	// core marks the queries that the index experiments repeat; the others are baseline-only to
	// keep the run within its time budget.
	core   bool
	feed   *readmodel.FeedQuery
	detail *readmodel.DetailQuery
}

// run executes the measured call once and returns its duration.
func (m measurement) run(w *world) time.Duration {
	var elapsed time.Duration
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		start := time.Now()
		var err error
		if m.detail != nil {
			_, err = w.reads.Detail(ctx, tx, *m.detail)
		} else {
			_, err = w.reads.Feed(ctx, tx, *m.feed)
		}
		elapsed = time.Since(start)
		if err != nil {
			w.t.Fatalf("%s: %v", m.name, err)
		}
	})
	return elapsed
}

// statement returns the SQL and parameters that run executes.
func (m measurement) statement() (string, []any) {
	if m.detail != nil {
		return readstore.Statement(readmodel.FeedQuery{Access: m.detail.Access, IDs: []uuid.UUID{m.detail.MessageID},
			DerivedGroups: m.detail.DerivedGroups}, true)
	}
	return readstore.Statement(*m.feed, false)
}

// explain logs EXPLAIN (ANALYZE, BUFFERS) of the statement with JIT off, as production runs it.
func (m measurement) explain(w *world) {
	statement, args := m.statement()
	w.t.Logf("SQL %s / %s:\n%s\nPARAMS: %v", m.name, m.caller, statement, args)
	var plan []string
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			w.t.Fatal(err)
		}
		if _, err := executor.ExecContext(ctx, "SET LOCAL jit = off"); err != nil {
			w.t.Fatal(err)
		}
		rows, err := executor.QueryContext(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+statement, args...)
		if err != nil {
			w.t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				w.t.Fatal(err)
			}
			plan = append(plan, line)
		}
	})
	w.t.Logf("EXPLAIN %s / %s:\n%s", m.name, m.caller, strings.Join(plan, "\n"))
}

// timings runs the measurement once to warm caches and then measureRuns times, and returns the
// median and the 90th percentile.
func (m measurement) timings(w *world) (median, p90 time.Duration) {
	m.run(w)
	runs := make([]time.Duration, measureRuns)
	for i := range runs {
		runs[i] = m.run(w)
	}
	slices.Sort(runs)
	return (runs[measureRuns/2-1] + runs[measureRuns/2]) / 2, runs[(measureRuns*9+9)/10-1]
}

// TestMeasureFeed seeds a large feed and records the cost of a feed page with and without
// attribute filters, and of a detail, to decide whether attribute filters need a projection of
// the effective values. It runs only with GOPAGER_MEASURE=1 because seeding takes a while.
func TestMeasureFeed(t *testing.T) {
	if os.Getenv("GOPAGER_MEASURE") != "1" {
		t.Skip("measurement disabled; run with GOPAGER_INTEGRATION=1 GOPAGER_MEASURE=1")
	}
	t.Parallel()
	w := newWorld(t)
	start := time.Now()
	agencyA, _ := seedMeasured(w)
	w.exec(`ANALYZE`)
	t.Logf("seeded in %s", time.Since(start).Round(time.Millisecond))
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			t.Fatal(err)
		}
		var version, settings string
		_ = executor.QueryRowContext(ctx, `SELECT version()`).Scan(&version)
		_ = executor.QueryRowContext(ctx, `SELECT 'shared_buffers=' || current_setting('shared_buffers') || ' work_mem=' ||
			current_setting('work_mem') || ' random_page_cost=' || current_setting('random_page_cost') || ' jit=' ||
			current_setting('jit')`).Scan(&settings)
		t.Logf("server: %s\nsettings: %s", version, settings)
		for _, table := range []string{"messages", "transmissions", "transmission_targets", "message_units", "processing_candidates",
			"processing_results", "units"} {
			var count int
			if err := executor.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
				t.Fatal(err)
			}
			t.Logf("rows %s = %d", table, count)
		}
		var targeted, untargeted int
		_ = executor.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE evaluation ? 'targetResults'),
			count(*) FILTER (WHERE evaluation ? 'fields') FROM processing_results`).Scan(&targeted, &untargeted)
		t.Logf("results: %d target-scoped, %d untargeted", targeted, untargeted)
	})

	now := base.Add(time.Hour)
	groups := []string{"derived", "payload", "gateway"}
	callers := map[string]authz.Authority{
		"all":    all(),
		"agency": principal(grant(scope("agency", agencyA), nil, "message.read", "message.detail")),
	}
	access := func(name string) authz.MessageAccess {
		a, err := authz.NewMessageAccess(callers[name], now)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	// Message 41 has results of both Workflows (one target-scoped) at Unit U1 of Agency A.
	var detailID uuid.UUID
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			t.Fatal(err)
		}
		if err := executor.QueryRowContext(ctx, `SELECT md5('m41')::uuid`).Scan(&detailID); err != nil {
			t.Fatal(err)
		}
	})
	predicates := []struct {
		name, predicate string
		core, agency    bool
	}{
		{"feed unfiltered", "", true, true},
		{"feed string eq (100 matches)", compare("eq", "attributes.incident.name", "string", `"name40"`), true, true},
		{"feed integer range (10 %)", logical("and", compare("gte", "attributes.incident.level", "integer", `"50"`),
			compare("lte", "attributes.incident.level", "integer", `"59"`)), true, false},
		{"feed string eq (no match, full scan)", compare("eq", "attributes.incident.name", "string", `"nobody"`), false, false},
		{"feed integer range (no match, full scan)", compare("gte", "attributes.incident.level", "integer", `"1000"`), false, false},
		{"feed target string eq", compare("eq", "attributes.incident.city", "string", `"city1"`), false, false},
	}
	var plan []measurement
	for _, caller := range []string{"all", "agency"} {
		for _, p := range predicates {
			if caller == "agency" && !p.agency {
				continue
			}
			query := readmodel.FeedQuery{Access: access(caller), Limit: measureLimit, DerivedGroups: groups}
			if p.predicate != "" {
				query.Selection = compileMeasured(t, p.predicate)
			}
			plan = append(plan, measurement{name: p.name, caller: caller, core: p.core && caller == "all", feed: &query})
		}
		plan = append(plan, measurement{name: "detail", caller: caller, core: caller == "all",
			detail: &readmodel.DetailQuery{Access: access(caller), MessageID: detailID, DerivedGroups: groups}})
	}
	for _, m := range plan {
		m.explain(w)
	}
	report := func(label string, coreOnly bool) {
		for _, m := range plan {
			if coreOnly && !m.core {
				continue
			}
			median, p90 := m.timings(w)
			t.Logf("RESULT [%s] %-42s caller=%-6s median=%8.2f ms p90=%8.2f ms", label, m.name, m.caller,
				float64(median.Microseconds())/1000, float64(p90.Microseconds())/1000)
		}
	}
	report("baseline", false)

	// Indexes inside the existing tables that might remove a sequential scan; each is measured
	// alone and dropped again.
	for _, index := range []struct{ label, name, statement string }{
		{"processing_results(message_id)", "measure_results_message", `CREATE INDEX measure_results_message ON processing_results (message_id)`},
		{"messages(first_observed_at desc, id desc)", "measure_messages_time", `CREATE INDEX measure_messages_time ON messages (first_observed_at DESC, id DESC)`},
	} {
		w.exec(index.statement)
		w.exec(`ANALYZE`)
		report("index "+index.label, true)
		w.exec(`DROP INDEX ` + index.name)
	}
}
