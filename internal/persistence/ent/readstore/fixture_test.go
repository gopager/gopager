package readstore_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/readstore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/readmodel"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

func TestMain(m *testing.M) { os.Exit(testpostgres.Run(m)) }

var migratedTemplate = testpostgres.NewTemplate("readstore", func(ctx context.Context, dsn string) error {
	store, _, err := postgres.NewStore(dsn, true)
	if err != nil {
		return err
	}
	migrateErr := store.Migrate(ctx, migrations.Files, platform.MigrationApply)
	if err := store.Close(); err != nil && migrateErr == nil {
		return err
	}
	return migrateErr
})

// base is the reference instant of the fixtures.
var base = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// world is a migrated database with helpers that insert rows the way intake would leave them.
type world struct {
	t       *testing.T
	store   *postgres.Store
	reads   *readstore.Store
	gateway uuid.UUID
	serial  int
}

func newWorld(t *testing.T) *world {
	t.Helper()
	store, _, err := postgres.NewStore(migratedTemplate.Database(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reads, err := readstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, store: store, reads: reads, gateway: uuid.New()}
	w.exec(`INSERT INTO gateways (id, name) VALUES ($1, 'gw')`, w.gateway)
	return w
}

// exec runs one statement in its own transaction.
func (w *world) exec(statement string, args ...any) {
	w.t.Helper()
	err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), statement, args...)
		return err
	})
	if err != nil {
		w.t.Fatalf("%s: %v", statement, err)
	}
}

// within runs fn in a transaction.
func (w *world) within(fn func(ctx context.Context, tx platform.TransactionContext)) {
	w.t.Helper()
	err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		fn(tx.Context(), tx)
		return nil
	})
	if err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) agency(name string) uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO agencies (id, name) VALUES ($1, $2)`, id, name)
	return id
}

func (w *world) organization(name string) uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO organizations (id, name, defaults) VALUES ($1, $2, '{}')`, id, name)
	return id
}

func (w *world) unit(agency uuid.UUID, name string, organization *uuid.UUID) uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO units (id, name, agency_id, organization_id) VALUES ($1, $2, $3, $4)`, id, name, agency, organization)
	return id
}

func (w *world) collection(units ...uuid.UUID) uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO unit_collections (id, name) VALUES ($1, 'c')`, id)
	for _, unit := range units {
		w.exec(`INSERT INTO unit_collection_memberships (unit_id, collection_id) VALUES ($1, $2)`, unit, id)
	}
	return id
}

func (w *world) network(agency uuid.UUID, name, protocol string) uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, $2, $3, $4)`, id, name, protocol, agency)
	return id
}

// message inserts a Message whose canonical Payload value is value.
func (w *world) message(first time.Time, form, value string) uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO messages (id, identity_form, identity_value, first_observed_at, last_observed_at)
		VALUES ($1, $2, $3, $4, $4)`, id, form, []byte(value), first)
	return id
}

// transmissionSpec describes a Transmission.
type transmissionSpec struct {
	message    uuid.UUID
	network    uuid.UUID
	protocol   string
	accepted   time.Time
	payload    string
	incomplete bool
	unresolved bool
	status     string
	quality    string
}

func (w *world) transmission(spec transmissionSpec) uuid.UUID {
	w.serial++
	id := uuid.New()
	if spec.protocol == "" {
		spec.protocol = "pocsag"
	}
	if spec.payload == "" {
		spec.payload = `{"form":"text","value":"alarm"}`
	}
	if spec.status == "" {
		spec.status = "none"
	}
	var quality any
	if spec.quality != "" {
		quality = spec.quality
	}
	w.exec(`INSERT INTO transmissions (id, gateway_id, network_id, submission_id, submission_fingerprint, network_resolution,
		protocol, payload, observed_at, observation_offset_seconds, accepted_at, source_revision, incomplete, quality_metrics,
		message_id, processing_status, network_unresolved)
		VALUES ($1, $2, $3, $4, $5, 'declared', $6, $7::json, $8, 0, $8, 1, $9, $10::json, $11, $12, $13)`,
		id, w.gateway, spec.network, "s"+uuid.NewString(), make([]byte, 32), spec.protocol, spec.payload, spec.accepted,
		spec.incomplete, quality, spec.message, spec.status, spec.unresolved)
	return id
}

// targetSpec describes a target of a Transmission.
type targetSpec struct {
	status   string
	unit     *uuid.UUID
	address  int64
	function int
	rule     *uuid.UUID
}

func (w *world) target(transmission uuid.UUID, position int, spec targetSpec) {
	if spec.status == "" {
		spec.status = "assigned"
	}
	w.exec(`INSERT INTO transmission_targets (transmission_id, position, original, network_id, protocol, observed_at, accepted_at,
		address_number, function, resolution_status, unit_id, matched_rule_id, assignment_revision)
		SELECT id, $2, '{}'::json, network_id, protocol, observed_at, accepted_at, $3, $4, $5, $6, $7, 1
		FROM transmissions WHERE id = $1`, transmission, position, spec.address, spec.function, spec.status, spec.unit, spec.rule)
}

func (w *world) suppress(transmission uuid.UUID, position int) {
	w.exec(`INSERT INTO target_suppressions (id, transmission_id, target_position, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5)`, uuid.New(), transmission, position, base, uuid.New())
}

// appearance inserts a cache row; unit may be nil for a Unit-less row.
func (w *world) appearance(message uuid.UUID, unit *uuid.UUID, agency uuid.UUID, first, anchor time.Time) {
	w.exec(`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor) VALUES ($1, $2, $3, $4, $5)`,
		message, unit, agency, first, anchor)
}

func (w *world) correcting(message uuid.UUID) {
	w.exec(`UPDATE message_units SET correction_status = 'correcting' WHERE message_id = $1`, message)
}

// assigned creates the Transmission, its assigned target and the appearance of a Unit in one step.
func (w *world) assigned(message, network uuid.UUID, unit, agency uuid.UUID, accepted time.Time, payload string) uuid.UUID {
	id := w.transmission(transmissionSpec{message: message, network: network, accepted: accepted, payload: payload})
	w.target(id, 0, targetSpec{unit: &unit, address: 1000})
	w.exec(`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor) VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (message_id, unit_id) WHERE unit_id IS NOT NULL DO NOTHING`, message, unit, agency, accepted)
	return id
}

// principal builds an active authority with the given grants and all field groups unless the
// caller overrides them.
func principal(grants ...authz.Grant) authz.Authority {
	id := uuid.New()
	for i := range grants {
		grants[i].ID, grants[i].SubjectID, grants[i].SubjectKind = uuid.NewString(), id.String(), "principal"
	}
	return authz.Authority{PrincipalID: id.String(), Revision: 1, Active: true, Grants: grants}
}

func grant(scope authz.Scope, maxAge *int64, actions ...string) authz.Grant {
	return authz.Grant{Actions: actions, Scope: scope, MaxAgeSeconds: maxAge}
}

func scope(kind string, ids ...uuid.UUID) authz.Scope {
	s := authz.Scope{Kind: kind}
	for _, id := range ids {
		s.References = append(s.References, id.String())
	}
	return s
}

func seconds(n int64) *int64 { return &n }

// all is a caller that may read, view and see Unit-less traffic everywhere.
func all() authz.Authority {
	return principal(grant(scope("all"), nil, "message.read", "message.detail", "without-unit.read"))
}

// feed runs one feed page for the authority at now.
func (w *world) feed(authority authz.Authority, now time.Time, query readmodel.FeedQuery) readmodel.FeedResult {
	w.t.Helper()
	access, err := authz.NewMessageAccess(authority, now)
	if err != nil {
		w.t.Fatal(err)
	}
	query.Access = access
	if query.Limit == 0 && query.IDs == nil {
		query.Limit = 50
	}
	var result readmodel.FeedResult
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		var err error
		if result, err = w.reads.Feed(ctx, tx, query); err != nil {
			w.t.Fatalf("feed: %v", err)
		}
	})
	return result
}

// filtered runs a feed page with validated filters.
func (w *world) filtered(authority authz.Authority, now time.Time, filters readmodel.Filters) readmodel.FeedResult {
	w.t.Helper()
	validated, err := filters.Validate()
	if err != nil {
		w.t.Fatal(err)
	}
	return w.feed(authority, now, readmodel.FeedQuery{Filters: validated})
}

func (w *world) detail(authority authz.Authority, now time.Time, id uuid.UUID, groups ...string) (readmodel.DetailRecord, error) {
	w.t.Helper()
	access, err := authz.NewMessageAccess(authority, now)
	if err != nil {
		w.t.Fatal(err)
	}
	var record readmodel.DetailRecord
	var failure error
	w.within(func(ctx context.Context, tx platform.TransactionContext) {
		record, failure = w.reads.Detail(ctx, tx, readmodel.DetailQuery{Access: access, MessageID: id, DerivedGroups: groups})
	})
	return record, failure
}

// ids returns the Message IDs of a result in order.
func ids(result readmodel.FeedResult) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(result.Items))
	for _, item := range result.Items {
		out = append(out, item.MessageID)
	}
	return out
}

func unitNames(record readmodel.FeedRecord) []string {
	var names []string
	for _, unit := range record.Units {
		names = append(names, unit.Name)
	}
	return names
}
