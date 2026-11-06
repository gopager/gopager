package borrowed_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/credential"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// fixtureNetwork is one declared standalone Network: its Protocol and its Agency.
type fixtureNetwork struct {
	protocol protocols.Protocol
	agency   uuid.UUID
}

// matcherOwners supplies declared standalone fixture Networks, not production Intake composition.
type matcherOwners struct {
	identity *identitystore.Store
	units    *unitstore.Store
	networks map[uuid.UUID]fixtureNetwork
}

func (o matcherOwners) ReadNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	value, found := o.networks[id]
	if !found {
		return "", uuid.Nil, authz.ErrNotFound
	}
	return value.protocol, value.agency, ctx.Err()
}

func (o matcherOwners) ReadUnitContext(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (units.Context, error) {
	return o.units.ReadUnitContext(ctx, tx, id)
}

func (o matcherOwners) ScopeExists(ctx context.Context, tx platform.TransactionContext, scope authz.Scope) (bool, error) {
	switch scope.Kind {
	case "unit", "unitCollection", "agency":
		return o.units.ScopeExists(ctx, tx, scope)
	default:
		return o.identity.ScopeExists(ctx, tx, scope)
	}
}

type matcherFixture struct {
	store     *postgres.Store
	units     *units.Service
	rules     *assignments.Service
	unitStore *unitstore.Store
	ruleStore *assignmentstore.Store
	resolver  *identity.Resolver
	owners    matcherOwners
	facts     *eventstore.Store
	receipts  *commandstore.Store
	proof     identity.Proof
	principal uuid.UUID
	networks  []uuid.UUID
	// agencies[0] owns networks[0]; agencies[1] owns networks[1] and networks[2]; agency is agencies[0].
	agencies [2]uuid.UUID
	agency   uuid.UUID
}

type staleUnitSaveRepository struct {
	units.Repository
	store   *postgres.Store
	staleID uuid.UUID
}

func (r *staleUnitSaveRepository) SaveUnit(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, value units.Unit, expected int64, now time.Time,
) error {
	if value.ID == r.staleID {
		executor, err := r.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `UPDATE units SET revision=revision+1 WHERE id=$1`, value.ID); err != nil {
			return err
		}
		r.staleID = uuid.Nil
	}
	return r.Repository.SaveUnit(ctx, tx, authority, value, expected, now)
}

// newMatcherFixture composes real primary adapters and explicitly bootstrapped management rights.
func newMatcherFixture(t *testing.T) matcherFixture {
	t.Helper()
	store := newStore(t)
	repository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	ruleRepository, err := assignmentstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := commandstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	networks := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	agencies := [2]uuid.UUID{uuid.New(), uuid.New()}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO agencies(id,name) VALUES($1,'Fixture agency A'),($2,'Fixture agency B')`,
			agencies[0], agencies[1]); err != nil {
			return err
		}
		// Rule scoping resolves a Network's Agency through the networks table, so the fixture
		// Networks exist as rows and not only in the fake owner.
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO networks(id,name,protocol,agency_id) VALUES
			($1,'Fixture POCSAG A','pocsag',$4),($2,'Fixture POCSAG B','pocsag',$5),($3,'Fixture FLEX B','flex',$5)`,
			networks[0], networks[1], networks[2], agencies[0], agencies[1])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	owners := matcherOwners{identity: repository, units: unitRepository,
		networks: map[uuid.UUID]fixtureNetwork{networks[0]: {protocols.POCSAG, agencies[0]},
			networks[1]: {protocols.POCSAG, agencies[1]}, networks[2]: {protocols.FLEX, agencies[1]}}}
	resolver, err := identity.NewResolver(repository, store, owners)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := identity.Bootstrap(t.Context(), store, repository, store, facts, "Operator", "initial-password",
		identity.BootstrapPermissions{Actions: authz.Actions(), Fields: []string{"unit", "address"}})
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginService(store, resolver, repository, facts, identity.DefaultLoginRateLimits())
	if err != nil {
		t.Fatal(err)
	}
	session, err := login.AuthenticateLocal(t.Context(), "test-source", "operator", "initial-password")
	if err != nil {
		t.Fatal(err)
	}
	unitService, err := units.NewService(store, resolver, store, unitRepository, ruleRepository, receipts, facts)
	if err != nil {
		t.Fatal(err)
	}
	ruleService, err := assignments.NewService(store, resolver, store, ruleRepository, unitRepository, owners, receipts, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	return matcherFixture{store: store, units: unitService, rules: ruleService, unitStore: unitRepository,
		ruleStore: ruleRepository, resolver: resolver, owners: owners, facts: facts, receipts: receipts,
		proof: identity.Proof{Kind: identity.SessionProof, Secret: session.Secret}, principal: principal, networks: networks,
		agencies: agencies, agency: agencies[0]}
}

// checkedMutation represents successful fixture-only transport validation, not a production bypass.
func checkedMutation(identity.Authentication) error { return nil }

// ruleInput creates a POCSAG set in the explicitly declared standalone fixture Network.
func ruleInput(network, unit uuid.UUID, from, to uint64, functions ...uint16) assignments.RuleInput {
	return assignments.RuleInput{NetworkID: network, UnitID: unit, Protocol: protocols.POCSAG,
		Selector: assignments.Selector{From: &from, To: &to, Mode: "function-bit", Functions: functions}, Enabled: true}
}

// configurationCounts checks transaction participants, not only the visible domain row.
func configurationCounts(t *testing.T, f matcherFixture) [8]int64 {
	t.Helper()
	var result [8]int64
	err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM units), (SELECT count(*) FROM address_rules),
			(SELECT count(*) FROM identity_audit), (SELECT count(*) FROM domain_events),
			(SELECT count(*) FROM command_receipts), (SELECT count(*) FROM assignment_pending_scopes),
			(SELECT revision FROM assignment_control WHERE id=1), (SELECT access_revision FROM identity_control WHERE id=1)`).
			Scan(&result[0], &result[1], &result[2], &result[3], &result[4], &result[5], &result[6], &result[7])
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type failingConfigurationFacts struct{ failure error }

func (f failingConfigurationFacts) AppendConfigurationFact(context.Context, platform.TransactionContext, events.ConfigurationFact) (uuid.UUID, error) {
	return uuid.Nil, f.failure
}

func (f failingConfigurationFacts) AppendMasterDataFact(context.Context, platform.TransactionContext, events.MasterDataFact) (uuid.UUID, error) {
	return uuid.Nil, f.failure
}

func TestCurrentMatcherAtomicCreationRetryPreviewAndRollback(t *testing.T) {
	f := newMatcherFixture(t)
	command := uuid.New()
	input := units.UnitInput{AgencyID: f.agency, Name: "Station", CollectionIDs: []uuid.UUID{}}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, command, input, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	before := configurationCounts(t, f)
	retry, err := f.units.CreateUnit(t.Context(), f.proof, command, input, checkedMutation)
	if err != nil || !reflect.DeepEqual(retry, unit) || configurationCounts(t, f) != before {
		t.Fatalf("Unit retry duplicated state or facts: %v", err)
	}
	input.Name = "changed"
	if _, err := f.units.CreateUnit(t.Context(), f.proof, command, input, checkedMutation); !errors.Is(err, commands.ErrIdempotencyConflict) {
		t.Fatalf("conflicting Unit command reuse accepted: %v", err)
	}
	ruleCommand := uuid.New()
	ruleValues := ruleInput(f.networks[0], unit.ID, 10, 20, 3, 1)
	if err := f.rules.Preview(t.Context(), f.proof, uuid.Nil, 0, ruleValues, nil); err != nil || configurationCounts(t, f) != before {
		t.Fatalf("preview changed configuration: %v", err)
	}
	rule, err := f.rules.CreateRule(t.Context(), f.proof, ruleCommand, ruleValues, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	before = configurationCounts(t, f)
	ruleValues.Selector.Functions = []uint16{1, 3}
	retry, err = f.rules.CreateRule(t.Context(), f.proof, ruleCommand, ruleValues, checkedMutation)
	if err != nil || !reflect.DeepEqual(retry, rule) || configurationCounts(t, f) != before {
		t.Fatalf("set-equivalent Rule retry duplicated state: %v", err)
	}
	failure := errors.New("configuration fact unavailable")
	broken, err := assignments.NewService(f.store, f.resolver, f.store, f.ruleStore, f.unitStore, f.owners,
		f.receipts, failingConfigurationFacts{failure})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := broken.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unit.ID, 30, 40), checkedMutation); !errors.Is(err, failure) || result.ID != uuid.Nil || configurationCounts(t, f) != before {
		t.Fatalf("fact failure left partial configuration/receipt/pending scope: %v", err)
	}
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, rule.ID, 9, ruleValues, checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale rule update accepted: %v", err)
	}
}

func TestCurrentMatcherSelectionCandidatesAndDeletionInvalidation(t *testing.T) {
	f := newMatcherFixture(t)
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency, Name: "Station"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unit.ID, 10, 20, 1), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[1], unit.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	targets := []protocols.Target{{Address: protocols.NumericAddress(12), Function: &function},
		{Address: protocols.NumericAddress(12), Function: &function}, {Address: protocols.NumericAddress(99), Function: &function}}
	observed := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		result, err := f.rules.MatchTargets(tx.Context(), tx, f.networks[0], protocols.POCSAG, observed, targets)
		if err != nil {
			return err
		}
		if result.Revision != 3 || len(result.Resolutions) != 3 || result.Resolutions[1].TargetPosition != 1 ||
			*result.Resolutions[0].AddressRuleID != rule.ID || result.Resolutions[2].Status != "unassigned" ||
			result.Resolutions[2].UnitID != nil || result.UnitContext[unit.ID].ID != unit.ID {
			t.Fatal("positions, captured revision or explicit unassigned context changed")
		}
		candidates, err := f.rules.NetworksWithMatches(tx.Context(), tx, []uuid.UUID{f.networks[0], f.networks[2]}, protocols.POCSAG, observed, targets)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(candidates.Networks, []uuid.UUID{f.networks[0]}) || candidates.Revision != result.Revision {
			t.Fatalf("candidate search widened Gateway Network set: %+v", candidates)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := f.units.DeleteUnit(t.Context(), f.proof, unit.ID, unit.Revision, checkedMutation)
	if err != nil || deleted.Status != "pending" {
		t.Fatalf("Unit deletion did not retain pending correction: %v", err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		exists, err := f.unitStore.ScopeExists(tx.Context(), tx, authz.Scope{Kind: "unit", References: []string{unit.ID.String()}})
		if err != nil {
			return err
		}
		if exists {
			t.Fatal("deleted Unit still authorizes a restricting scope")
		}
		result, err := f.rules.MatchTargets(tx.Context(), tx, f.networks[0], protocols.POCSAG, observed, targets)
		if err != nil {
			return err
		}
		if result.Revision != 4 || result.Resolutions[0].Status != "unassigned" || len(result.UnitContext) != 0 {
			t.Fatal("deleted Unit remained a matching candidate")
		}
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var disabled, pending int
		if err := executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM address_rules WHERE unit_id=$1 AND NOT enabled),
			(SELECT count(*) FROM assignment_pending_scopes WHERE unit_id=$1 AND reason='unit-deleted')`, unit.ID).
			Scan(&disabled, &pending); err != nil {
			return err
		}
		if disabled != 2 || pending != 1 {
			t.Fatal("Unit marker/rule invalidation/pending correction did not commit together")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCurrentMatcherConcurrentConflictsAndCollectionDetach(t *testing.T) {
	f := newMatcherFixture(t)
	collection, err := f.units.CreateCollection(t.Context(), f.proof, uuid.New(), "Stations", checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Station", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			_, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unit.ID, 10, 20), checkedMutation)
			results <- err
		})
	}
	close(start)
	workers.Wait()
	successes, conflicts := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, assignments.ErrRuleConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent rule outcome: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent ambiguous rules both committed")
	}
	if _, err := f.units.DeleteCollection(t.Context(), f.proof, collection.ID, collection.Revision, checkedMutation); err != nil {
		t.Fatal(err)
	}
	current, err := f.units.ReadUnit(t.Context(), f.proof, unit.ID)
	if err != nil || current.Revision != 2 || len(current.CollectionIDs) != 0 {
		t.Fatalf("collection deletion did not detach and revise members: %v", err)
	}
}

// replaceManagementGrants installs exact test-only current grants under the real shared lock.
func replaceManagementGrants(t *testing.T, f matcherFixture, grants []authz.Grant) {
	t.Helper()
	err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := f.owners.identity.LockAccess(tx.Context(), tx); err != nil {
			return err
		}
		driver, err := borrowed.New(f.store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{"Grant": entql.FieldEQ("subject_id", f.principal)}, Create: map[string]bool{"Grant": true}})
		if _, err := client.Grant.Delete().Exec(scope); err != nil {
			return err
		}
		for _, value := range grants {
			if err := client.Grant.Create().SetSubjectID(f.principal).SetSubjectKind(grant.SubjectKindPrincipal).
				SetActions(value.Actions).SetScope(value.Scope).Exec(scope); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCurrentMatcherNativeFilteringAndAllUseChecks(t *testing.T) {
	f := newMatcherFixture(t)
	collection, err := f.units.CreateCollection(t.Context(), f.proof, uuid.New(), "Shared", checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency, Name: "Shared", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[1], Name: "Hidden", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ network, unit uuid.UUID }{{f.networks[0], shared.ID}, {f.networks[1], shared.ID}, {f.networks[1], hidden.ID}} {
		first := uint64(10)
		if entry.unit == hidden.ID {
			first = 30
		}
		if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(entry.network, entry.unit, first, first), checkedMutation); err != nil {
			t.Fatal(err)
		}
	}
	replaceManagementGrants(t, f, []authz.Grant{{
		Actions: []string{"unit.read", "unit.update", "unit-collection.read", "unit-collection.update", "unit-collection.delete", "address-rule.read", "address-rule.create"},
		Scope:   authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}},
	}})
	page, err := f.units.ListUnits(t.Context(), f.proof, units.PageInput{Offset: 1, Limit: 1})
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatalf("hidden Unit affected native count/page: %+v %v", page, err)
	}
	rules, err := f.rules.ListRules(t.Context(), f.proof, units.PageInput{Limit: 1})
	if err != nil || rules.Total != 1 || len(rules.Items) != 1 {
		t.Fatalf("hidden rule affected native count/page: %+v %v", rules, err)
	}
	if _, err := f.units.ReadUnit(t.Context(), f.proof, hidden.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("hidden Unit existence exposed: %v", err)
	}
	// A Unit alerted through another Agency's Network is still governed by its own Agency alone.
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, shared.ID, shared.Revision,
		units.UnitInput{AgencyID: f.agency, Name: "Shared renamed", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation); err != nil {
		t.Fatalf("own-Agency Unit rename refused because of a foreign Network use: %v", err)
	}
	before := configurationCounts(t, f)
	if _, err := f.units.DeleteCollection(t.Context(), f.proof, collection.ID, collection.Revision, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("collection deletion skipped hidden member use: %v", err)
	}
	if configurationCounts(t, f) != before {
		t.Fatal("denied all-use mutation changed state")
	}
	if err := f.rules.Preview(t.Context(), f.proof, uuid.Nil, 0, ruleInput(f.networks[0], shared.ID, 10, 10), nil); !errors.Is(err, assignments.ErrRuleConflict) {
		t.Fatalf("preview ignored current conflicting configuration: %v", err)
	}
}

// restrictingManagementKey persists a declared fixture key to exercise the real Resolver intersection.
func restrictingManagementKey(t *testing.T, f matcherFixture, restrictions *authz.Restrictions) identity.Proof {
	t.Helper()
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := f.owners.identity.LockAccess(tx.Context(), tx); err != nil {
			return err
		}
		driver, err := borrowed.New(f.store, tx)
		if err != nil {
			return err
		}
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current, Create: map[string]bool{"Credential": true}})
		return ent.NewClient(ent.Driver(driver)).Credential.Create().SetAccountID(f.principal).
			SetKind(credential.KindAccessKey).SetSecretHash(hash[:]).SetRestrictions(restrictions).SetCreatedAt(now).SetLastUsedAt(now).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
	return identity.Proof{Kind: identity.AccessKeyProof, Secret: secret}
}

func TestCurrentMatcherNativeMixedScopesAndRestrictingKeysMatchPointDecisions(t *testing.T) {
	f := newMatcherFixture(t)
	collection, err := f.units.CreateCollection(t.Context(), f.proof, uuid.New(), "Shared", checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]uuid.UUID, 3)
	ruleIDs := make([]uuid.UUID, 3)
	for i := range ids {
		memberships := []uuid.UUID{}
		if i != 1 {
			memberships = append(memberships, collection.ID)
		}
		network, agency := f.networks[1], f.agencies[1]
		if i == 0 {
			network, agency = f.networks[0], f.agencies[0]
		}
		unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: agency, Name: "Station", CollectionIDs: memberships}, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = unit.ID
		input := ruleInput(network, unit.ID, uint64(10+i*10), uint64(10+i*10))
		input.Enabled = i != 0
		rule, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), input, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		ruleIDs[i] = rule.ID
	}
	actions := []string{"unit.read", "address-rule.read", "address-rule.create"}
	replaceManagementGrants(t, f, []authz.Grant{
		{Actions: actions, Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}}},
		{Actions: actions, Scope: authz.Scope{Kind: "unit", References: []string{ids[1].String()}}},
		{Actions: actions, Scope: authz.Scope{Kind: "unitCollection", References: []string{collection.ID.String()}}},
	})
	for _, scope := range []authz.Scope{
		{Kind: "agency", References: []string{f.agencies[1].String()}},
		{Kind: "unitCollection", References: []string{collection.ID.String()}},
		{Kind: "unit", References: []string{ids[0].String()}},
	} {
		proof := restrictingManagementKey(t, f, &authz.Restrictions{Actions: actions, Scopes: []authz.Scope{scope}, Fields: []string{"unit", "address"}})
		var expectedUnits, expectedRules []uuid.UUID
		err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			authentication, err := f.resolver.Resolve(tx.Context(), tx, proof)
			if err != nil {
				return err
			}
			now, err := f.store.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			for i, id := range ids {
				unit, err := f.unitStore.ReadUnitContext(tx.Context(), tx, id)
				if err != nil {
					return err
				}
				target := authz.Target{AgencyID: unit.AgencyID, UnitID: id, UnitCollections: unit.CollectionIDs}
				if err := authz.Authorize(authentication.Authority, "unit.read", []string{"unit"}, target, now); err == nil {
					expectedUnits = append(expectedUnits, id)
				} else if !errors.Is(err, authz.ErrForbidden) {
					return err
				}
				if err := authz.Authorize(authentication.Authority, "address-rule.read", []string{"unit", "address"}, target, now); err == nil {
					expectedRules = append(expectedRules, ruleIDs[i])
				} else if !errors.Is(err, authz.ErrForbidden) {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(expectedUnits, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		page, err := f.units.ListUnits(t.Context(), proof, units.PageInput{Limit: 1})
		if err != nil || page.Total != len(expectedUnits) || len(page.Items) != 1 || page.Items[0].ID != expectedUnits[0] {
			t.Fatalf("native Unit scope differs from point decision for %s: %v", scope.Kind, err)
		}
		rules, err := f.rules.ListRules(t.Context(), proof, units.PageInput{Limit: 200})
		if err != nil || rules.Total != len(expectedRules) || len(rules.Items) != len(expectedRules) {
			t.Fatalf("native Rule scope differs from point decision for %s: %v", scope.Kind, err)
		}
		for _, rule := range rules.Items {
			if !slices.Contains(expectedRules, rule.ID) {
				t.Fatal("foreign rule escaped restricting-key intersection")
			}
		}
		if scope.Kind == "unit" {
			if err := f.rules.Preview(t.Context(), proof, uuid.Nil, 0, ruleInput(f.networks[1], ids[0], 20, 20), nil); !errors.Is(err, assignments.ErrRuleConflict) {
				t.Fatalf("hidden conflicting rule was ignored: %v", err)
			}
		}
	}
}

func TestPendingScopeSupersessionAndRebaseRespectAllScopeAndUnitIntersections(t *testing.T) {
	f := newMatcherFixture(t)
	mark := func(networks []uuid.UUID, unit *uuid.UUID) int64 {
		t.Helper()
		var revision int64
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			revision, err = f.ruleStore.MarkPending(tx.Context(), tx, networks, unit, "rule-change")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return revision
	}
	state := func(id uuid.UUID) (string, int64) {
		t.Helper()
		var got string
		var revision int64
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT state,assignment_revision
				FROM assignment_pending_scopes WHERE id=$1`, id).Scan(&got, &revision)
		}); err != nil {
			t.Fatal(err)
		}
		return got, revision
	}
	scopeID := func(revision int64) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT id FROM assignment_pending_scopes
				WHERE assignment_revision=$1`, revision).Scan(&id)
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}

	firstRevision := mark([]uuid.UUID{f.networks[0]}, nil)
	firstID := scopeID(firstRevision)
	allRevision := mark(nil, nil)
	if got, _ := state(firstID); got != "superseded" {
		t.Fatalf("all-Network scope did not supersede overlapping scope: %s", got)
	}
	allID := scopeID(allRevision)

	unitA, unitB := uuid.New(), uuid.New()
	unitARevision := mark([]uuid.UUID{f.networks[0]}, &unitA)
	unitAID := scopeID(unitARevision)
	unitBRevision := mark([]uuid.UUID{f.networks[0]}, &unitB)
	unitBID := scopeID(unitBRevision)
	if got, revision := state(allID); got != "superseded" {
		t.Fatalf("Network/Unit scope did not supersede all-scope job: %s at %d", got, revision)
	}
	if got, revision := state(unitAID); got != "queued" || revision != unitBRevision {
		t.Fatalf("disjoint Unit job was not rebased: %s at %d, want queued at %d", got, revision, unitBRevision)
	}
	if got, revision := state(unitBID); got != "queued" || revision != unitBRevision {
		t.Fatalf("new Unit job has incorrect state/revision: %s at %d", got, revision)
	}
}

func TestCorrectionLeaseExpiryReclaimsAndFencesPriorToken(t *testing.T) {
	f := newMatcherFixture(t)
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := f.ruleStore.MarkPending(tx.Context(), tx, []uuid.UUID{f.networks[0]}, nil, "rule-change")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	claim := func() (assignments.CorrectionLease, bool) {
		t.Helper()
		var lease assignments.CorrectionLease
		var claimed bool
		err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			now, err := f.store.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			lease, claimed, err = f.ruleStore.ClaimCorrection(tx.Context(), tx, now, time.Second)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return lease, claimed
	}
	first, claimed := claim()
	if !claimed || first.FencingToken != 1 {
		t.Fatalf("first correction claim=%+v claimed=%t", first, claimed)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE assignment_pending_scopes
			SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, first.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	second, claimed := claim()
	if !claimed || second.ID != first.ID || second.FencingToken != first.FencingToken+1 {
		t.Fatalf("expired correction lease was not reclaimed with a new fence: first=%+v second=%+v claimed=%t",
			first, second, claimed)
	}
	err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := f.ruleStore.CompleteCorrection(tx.Context(), tx, first)
		return err
	})
	if !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("prior lease token completed reclaimed work: %v", err)
	}
	var completed bool
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		completed, err = f.ruleStore.CompleteCorrection(tx.Context(), tx, second)
		return err
	}); err != nil || !completed {
		t.Fatalf("current lease could not complete: completed=%t err=%v", completed, err)
	}
}

func TestHistoricalCorrectionAndSuppressionRebuildSupportsWithoutChangingEvidence(t *testing.T) {
	f := newMatcherFixture(t)
	networkID, gatewayID := f.networks[0], uuid.New()
	unitA, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Station A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unitB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Station B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(),
		ruleInput(networkID, unitA.ID, 10, 20, 1), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	target := protocols.Target{Address: protocols.NumericAddress(10), Function: &function}
	original, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := json.Marshal([]protocols.Target{target})
	if err != nil {
		t.Fatal(err)
	}
	observed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	accepted := observed.Add(time.Second)
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "correction-dirty-intake",
		Protocol: protocols.POCSAG, ObservationTime: observed.Add(2 * time.Second).Format(time.RFC3339),
		GatewayNetworkIdentifier: "default", Payload: json.RawMessage(`{"form":"tone-only"}`),
		TransmissionTargets: []protocols.Target{target}}
	encodedSubmission, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	identitySubmission, err := submission.Validate(len(encodedSubmission), accepted)
	if err != nil {
		t.Fatal(err)
	}
	messageID, transmissionID := uuid.New(), uuid.New()
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var assignmentRevision int64
		if err := executor.QueryRowContext(tx.Context(), `SELECT revision FROM assignment_control WHERE id=1`).
			Scan(&assignmentRevision); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways (id) VALUES($1)`, gatewayID); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,'default',$3,true)`, uuid.New(), gatewayID, networkID); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO messages
			(id,identity_form,identity_subtype,identity_value,first_observed_at,last_observed_at)
			VALUES($1,$2,$3,$4,$5,$5)`, messageID, identitySubmission.Identity.Form,
			identitySubmission.Identity.Subtype, identitySubmission.Identity.Value, observed); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO transmissions
			(id,gateway_id,network_id,submission_id,submission_fingerprint,network_resolution,protocol,payload,targets,
			 observed_at,observation_offset_seconds,accepted_at,source_revision,message_id,processing_status)
			VALUES($1,$2,$3,$4,$5,'declared','pocsag','{}',$6,$7,0,$8,1,$9,'none')`,
			transmissionID, gatewayID, networkID, uuid.NewString(), make([]byte, 32), string(targets),
			observed, accepted, messageID); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO transmission_targets
			(transmission_id,position,original,resolution_status,unit_id,matched_rule_id,assignment_revision)
			VALUES($1,0,$2,'assigned',$3,$4,$5)`,
			transmissionID, string(original), unitA.ID, rule.ID, assignmentRevision); err != nil {
			return err
		}
		var contributionID = uuid.New()
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO message_contributions
			(id,message_id,unit_id,contribution_time,age_anchor) VALUES($1,$2,$3,$4,$4)`,
			contributionID, messageID, unitA.ID, observed); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO contribution_supports
			(id,contribution_id,transmission_id,target_position,observed_at,accepted_at)
			VALUES($1,$2,$3,0,$4,$5)`, uuid.New(), contributionID, transmissionID, observed, accepted)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, rule.ID, rule.Revision,
		ruleInput(networkID, unitB.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	intakeRepository, err := ingeststore.New(f.store, f.facts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	var lateReceipt ingest.Receipt
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		acceptedAt, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		validated, err := submission.Validate(4096, acceptedAt)
		if err != nil {
			return err
		}
		validated.ResolvedNetwork, validated.Resolution = networkID, "declared"
		matched, err := f.rules.MatchTargets(tx.Context(), tx, networkID, protocols.POCSAG,
			validated.ObservedAt, validated.TransmissionTargets)
		if err != nil {
			return err
		}
		lateReceipt, err = intakeRepository.AcceptTransmission(tx.Context(), tx, gatewayID,
			validated, acceptedAt, matched)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if lateReceipt.MessageID != messageID {
		t.Fatalf("late intake did not join the existing Message: got=%s want=%s", lateReceipt.MessageID, messageID)
	}
	var oldLease assignments.CorrectionLease
	claimed := false
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		oldLease, claimed, err = f.ruleStore.ClaimCorrection(tx.Context(), tx, now, 30*time.Second)
		return err
	})
	if err != nil || !claimed {
		t.Fatalf("r2 correction was not claimed: claimed=%t err=%v", claimed, err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var dirty int
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM correction_dirty_transmissions
			WHERE scope_id=$1 AND transmission_id=$2`, oldLease.ID, lateReceipt.TransmissionID).Scan(&dirty); err != nil {
			return err
		}
		if dirty != 1 {
			t.Fatalf("concurrent intake dirty capture count=%d, want 1", dirty)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var dirtyItem assignments.CorrectionBatchItem
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		items, err := f.ruleStore.ReadCorrectionBatch(tx.Context(), tx, oldLease, 100)
		if err != nil {
			return err
		}
		if len(items) != 1 || !items[0].Dirty || items[0].TransmissionID != lateReceipt.TransmissionID {
			t.Fatalf("correction did not select dirty intake: %+v", items)
		}
		dirtyItem = items[0]
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var staleMatch assignments.MatchResult
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		staleMatch, err = f.rules.MatchTargets(tx.Context(), tx, networkID, protocols.POCSAG,
			dirtyItem.ObservedAt, dirtyItem.Targets)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE identity_control
			SET access_revision=access_revision+1 WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := f.ruleStore.ApplyCorrectionBatch(tx.Context(), tx, oldLease,
			[]assignments.CorrectionUpdate{{Item: dirtyItem, Match: staleMatch}})
		return err
	})
	if !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("correction applied under a stale access revision: %v", err)
	}
	if _, err := f.rules.UpdateRule(t.Context(), f.proof, rule.ID, rule.Revision+1,
		ruleInput(networkID, unitA.ID, 10, 20, 1), checkedMutation); err != nil {
		t.Fatal(err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := f.ruleStore.ApplyCorrectionBatch(tx.Context(), tx, oldLease,
			[]assignments.CorrectionUpdate{{Item: dirtyItem, Match: staleMatch}})
		return err
	})
	if !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("superseded r2 worker committed derived changes: %v", err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := f.ruleStore.CompleteCorrection(tx.Context(), tx, oldLease)
		return err
	})
	if !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("superseded r2 worker was not fenced: %v", err)
	}
	runner, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, f.ruleStore, f.facts, noProcessingReadmission{})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("historical correction did not run: worked=%t err=%v", worked, err)
	}
	assertCorrected := func(wantUnit uuid.UUID, wantSupport bool) int64 {
		t.Helper()
		var revision int64
		err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			var unitID uuid.UUID
			var savedOriginal []byte
			if err := executor.QueryRowContext(tx.Context(), `SELECT unit_id,original FROM transmission_targets
				WHERE transmission_id=$1 AND position=0`, transmissionID).Scan(&unitID, &savedOriginal); err != nil {
				return err
			}
			if unitID != wantUnit || !bytes.Equal(savedOriginal, original) {
				t.Fatalf("correction changed target evidence/resolution: unit=%s", unitID)
			}
			var lateUnitID uuid.UUID
			var lateOriginal []byte
			if err := executor.QueryRowContext(tx.Context(), `SELECT unit_id,original FROM transmission_targets
				WHERE transmission_id=$1 AND position=0`, lateReceipt.TransmissionID).
				Scan(&lateUnitID, &lateOriginal); err != nil {
				return err
			}
			if lateUnitID != wantUnit || !bytes.Equal(lateOriginal, original) {
				t.Fatalf("r3 did not rebuild the dirty intake target: unit=%s", lateUnitID)
			}
			var supports int
			if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM contribution_supports
				WHERE transmission_id=$1 AND target_position=0`, transmissionID).Scan(&supports); err != nil {
				return err
			}
			if wantSupport && supports != 1 || !wantSupport && supports != 0 {
				t.Fatalf("unexpected support count %d (wantSupport=%t)", supports, wantSupport)
			}
			var lateSupports int
			if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM contribution_supports
				WHERE transmission_id=$1 AND target_position=0`, lateReceipt.TransmissionID).Scan(&lateSupports); err != nil {
				return err
			}
			if lateSupports != 1 {
				t.Fatalf("dirty-intake support count=%d, want 1", lateSupports)
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT revision FROM assignment_control WHERE id=1`).
				Scan(&revision); err != nil {
				return err
			}
			var state string
			if err := executor.QueryRowContext(tx.Context(), `SELECT state FROM assignment_pending_scopes
				WHERE assignment_revision=$1 ORDER BY recorded_at DESC LIMIT 1`, revision).Scan(&state); err != nil {
				return err
			}
			if state != "completed" {
				t.Fatalf("correction status is %q, want completed", state)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return revision
	}
	revision := assertCorrected(unitA.ID, true)
	suppressionCommand := uuid.New()
	suppressionInput := assignments.CreateSuppressionInput{TransmissionID: transmissionID, TargetPosition: 0,
		ExpectedAssignmentRevision: revision}
	suppressed, err := f.rules.CreateSuppression(t.Context(), f.proof, suppressionCommand,
		suppressionInput, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	// A replay after an uncertain commit carries the now-stale expected revision
	// for an already active suppression and must still return the stored result.
	beforeReplay := configurationCounts(t, f)
	replayed, err := f.rules.CreateSuppression(t.Context(), f.proof, suppressionCommand,
		suppressionInput, checkedMutation)
	if err != nil || replayed.Suppression.ID != suppressed.Suppression.ID ||
		replayed.Suppression.Revision != suppressed.Suppression.Revision ||
		replayed.Correction.ID != suppressed.Correction.ID || configurationCounts(t, f) != beforeReplay {
		t.Fatalf("suppression replay did not return the stored result without new state: %v", err)
	}
	changedInput := suppressionInput
	changedInput.ExpectedAssignmentRevision = beforeReplay[6]
	if _, err := f.rules.CreateSuppression(t.Context(), f.proof, suppressionCommand,
		changedInput, checkedMutation); !errors.Is(err, commands.ErrIdempotencyConflict) ||
		configurationCounts(t, f) != beforeReplay {
		t.Fatalf("changed suppression input under a reused command ID was not an idempotency conflict: %v", err)
	}
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("suppression correction did not run: worked=%t err=%v", worked, err)
	}
	revision = assertCorrected(unitA.ID, false)
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var contributions int
		var contributionTime time.Time
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*),MIN(contribution_time) FROM message_contributions
			WHERE message_id=$1 AND unit_id=$2`, messageID, unitA.ID).Scan(&contributions, &contributionTime); err != nil {
			return err
		}
		if contributions != 1 {
			t.Fatalf("suppressing one of two supports removed their shared contribution: %d", contributions)
		}
		// The surviving 12:00:02-equivalent support re-anchors the contribution time.
		if want := observed.Add(2 * time.Second); !contributionTime.Equal(want) {
			t.Fatalf("contribution time=%s after suppressing the earlier support, want %s", contributionTime, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.LiftSuppression(t.Context(), f.proof, suppressed.Suppression.ID,
		suppressed.Suppression.Revision, revision, checkedMutation); err != nil {
		t.Fatal(err)
	}
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("suppression lift correction did not run: worked=%t err=%v", worked, err)
	}
	revision = assertCorrected(unitA.ID, true)
	unassignedTarget := protocols.Target{Address: protocols.NumericAddress(99), Function: &function}
	unassignedOriginal, err := json.Marshal(unassignedTarget)
	if err != nil {
		t.Fatal(err)
	}
	unassignedTargets, err := json.Marshal([]protocols.Target{unassignedTarget})
	if err != nil {
		t.Fatal(err)
	}
	unassignedMessageID, unassignedTransmissionID := uuid.New(), uuid.New()
	unassignedFingerprint := make([]byte, 32)
	fingerprintID := uuid.New()
	copy(unassignedFingerprint, fingerprintID[:])
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var currentRevision int64
		if err := executor.QueryRowContext(tx.Context(), `SELECT revision FROM assignment_control WHERE id=1`).
			Scan(&currentRevision); err != nil {
			return err
		}
		if currentRevision != revision {
			t.Fatalf("assignment revision changed before unassigned-target case: got=%d want=%d",
				currentRevision, revision)
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO messages
			(id,identity_form,identity_value,first_observed_at,last_observed_at)
			VALUES($1,'test',$2,$3,$3)`, unassignedMessageID, []byte{2}, observed.Add(3*time.Second)); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO transmissions
			(id,gateway_id,network_id,submission_id,submission_fingerprint,network_resolution,protocol,payload,targets,
			 observed_at,observation_offset_seconds,accepted_at,source_revision,message_id,processing_status)
			VALUES($1,$2,$3,$4,$5,'declared','pocsag','{}',$6,$7,0,$8,1,$9,'none')`,
			unassignedTransmissionID, gatewayID, networkID, uuid.NewString(), unassignedFingerprint,
			string(unassignedTargets), observed.Add(3*time.Second), accepted.Add(3*time.Second), unassignedMessageID); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO transmission_targets
			(transmission_id,position,original,resolution_status,unit_id,matched_rule_id,assignment_revision)
			VALUES($1,0,$2,'unassigned',NULL,NULL,$3)`,
			unassignedTransmissionID, string(unassignedOriginal), currentRevision)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	suppressedUnassigned, err := f.rules.CreateSuppression(t.Context(), f.proof, uuid.New(),
		assignments.CreateSuppressionInput{TransmissionID: unassignedTransmissionID, TargetPosition: 0,
			ExpectedAssignmentRevision: revision}, checkedMutation)
	if err != nil {
		t.Fatalf("unassigned-target suppression failed (%T): %#v", err, err)
	}
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("unassigned-target suppression correction did not run: worked=%t err=%v", worked, err)
	}
	assertUnassigned := func() {
		t.Helper()
		if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			var resolution string
			var unitID uuid.NullUUID
			var supports int
			if err := executor.QueryRowContext(tx.Context(), `SELECT resolution_status,unit_id FROM transmission_targets
				WHERE transmission_id=$1 AND position=0`, unassignedTransmissionID).Scan(&resolution, &unitID); err != nil {
				return err
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM contribution_supports
				WHERE transmission_id=$1 AND target_position=0`, unassignedTransmissionID).Scan(&supports); err != nil {
				return err
			}
			if resolution != "unassigned" || unitID.Valid || supports != 0 {
				t.Fatalf("unassigned target acquired a Unit/support: resolution=%s unit=%+v supports=%d",
					resolution, unitID, supports)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertUnassigned()
	currentRevision := assertCorrected(unitA.ID, true)
	if _, err := f.rules.LiftSuppression(t.Context(), f.proof, suppressedUnassigned.Suppression.ID,
		suppressedUnassigned.Suppression.Revision, currentRevision, checkedMutation); err != nil {
		t.Fatal(err)
	}
	if worked, err := runner.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("unassigned-target suppression lift correction did not run: worked=%t err=%v", worked, err)
	}
	assertUnassigned()
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var transmissions, messages int
		if err := executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM transmissions WHERE id IN ($1,$3,$5)),
			(SELECT count(*) FROM messages WHERE id IN ($2,$4,$6))`,
			transmissionID, messageID, lateReceipt.TransmissionID, lateReceipt.MessageID,
			unassignedTransmissionID, unassignedMessageID).
			Scan(&transmissions, &messages); err != nil {
			return err
		}
		wantMessages := 1
		if messageID != lateReceipt.MessageID {
			wantMessages++
		}
		if unassignedMessageID != messageID && unassignedMessageID != lateReceipt.MessageID {
			wantMessages++
		}
		if transmissions != 3 || messages != wantMessages {
			t.Fatal("correction or suppression created Message membership/evidence")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCurrentMatcherCollectionScopeDeletionAndActualRevisionExhaustion(t *testing.T) {
	f := newMatcherFixture(t)
	collection, err := f.units.CreateCollection(t.Context(), f.proof, uuid.New(), "Stations", checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency, Name: "Station", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	before := configurationCounts(t, f)
	broken, err := units.NewService(f.store, f.resolver, f.store, f.unitStore, f.ruleStore, f.receipts,
		failingConfigurationFacts{errors.New("configuration fact failure")})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := broken.DeleteUnit(t.Context(), f.proof, unit.ID, 1, checkedMutation); err == nil || result.ID != uuid.Nil ||
		configurationCounts(t, f) != before {
		t.Fatal("failed Unit deletion left marker, invalidation, revision or fact")
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE units SET revision=$1 WHERE id=$2`, int64(math.MaxInt64), unit.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	before = configurationCounts(t, f)
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, math.MaxInt64, units.UnitInput{AgencyID: f.agency, Name: "Exhausted"},
		checkedMutation); platform.FailureCode(err) != platform.CodeResourceLimit || configurationCounts(t, f) != before {
		t.Fatalf("actual revision exhaustion did not fail atomically with RESOURCE_LIMIT: %v", err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE units SET revision=1 WHERE id=$1`, unit.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"unit-collection.delete"},
		Scope: authz.Scope{Kind: "unitCollection", References: []string{collection.ID.String()}}}})
	if _, err := f.units.DeleteCollection(t.Context(), f.proof, collection.ID, 1, checkedMutation); err != nil {
		t.Fatalf("deletion lost its validated self-context while detaching members: %v", err)
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		current, err := f.unitStore.ReadUnitContext(tx.Context(), tx, unit.ID)
		if err != nil {
			return err
		}
		if current.Revision != 2 || len(current.CollectionIDs) != 0 {
			t.Fatal("collection-scoped deletion did not advance/detach the Unit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type observedTransactions struct {
	store   *postgres.Store
	started chan int
}

// WithinTransaction observes the actual backend before resolving current authority/configuration.
func (o observedTransactions) WithinTransaction(ctx context.Context, operation platform.TxFunc) error {
	return o.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		executor, err := o.store.Resolve(tx)
		if err != nil {
			return err
		}
		var pid int
		if err := executor.QueryRowContext(tx.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			return err
		}
		o.started <- pid
		return operation(tx)
	})
}

// verifyCurrentMatcherSerialization observes real blocking for both ordinary edits and deletion.
func verifyCurrentMatcherSerialization(t *testing.T, deleting bool) {
	t.Helper()
	f := newMatcherFixture(t)
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency, Name: "Station"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], unit.ID, 10, 10), checkedMutation); err != nil {
		t.Fatal(err)
	}
	function := uint8(1)
	targets := []protocols.Target{{Address: protocols.NumericAddress(10), Function: &function}}
	observed := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	captured := make(chan int, 1)
	release := make(chan struct{})
	matchDone := make(chan error, 1)
	var once sync.Once
	unlock := func() { once.Do(func() { close(release) }) }
	defer unlock()
	go func() {
		matchDone <- f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			result, err := f.rules.MatchTargets(tx.Context(), tx, f.networks[0], protocols.POCSAG, observed, targets)
			if err != nil {
				captured <- 0
				return err
			}
			if result.Revision != 2 || result.Resolutions[0].Status != "assigned" {
				captured <- 0
				return errors.New("unexpected captured resolution")
			}
			executor, err := f.store.Resolve(tx)
			if err != nil {
				captured <- 0
				return err
			}
			var pid int
			if err := executor.QueryRowContext(tx.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				captured <- 0
				return err
			}
			captured <- pid
			select {
			case <-release:
				return nil
			case <-tx.Context().Done():
				return tx.Context().Err()
			}
		})
	}()
	var matcherPID int
	select {
	case matcherPID = <-captured:
	case err := <-matchDone:
		t.Fatalf("matcher transaction failed before capture: %v", err)
	}
	if matcherPID == 0 {
		t.Fatalf("matcher could not capture live configuration: %v", <-matchDone)
	}
	mutations := observedTransactions{store: f.store, started: make(chan int, 1)}
	service, err := units.NewService(mutations, f.resolver, f.store, f.unitStore, f.ruleStore, f.receipts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	deleteDone := make(chan error, 1)
	go func() {
		var err error
		if deleting {
			_, err = service.DeleteUnit(t.Context(), f.proof, unit.ID, 1, checkedMutation)
		} else {
			_, err = service.ChangeUnit(t.Context(), f.proof, unit.ID, 1, units.UnitInput{AgencyID: f.agency, Name: "Station changed"}, checkedMutation)
		}
		deleteDone <- err
	}()
	var deleterPID int
	select {
	case deleterPID = <-mutations.started:
	case err := <-deleteDone:
		t.Fatalf("deletion failed before coordinated invalidation: %v", err)
	}
	if deleting {
		waitForAuthorityLock(t, f.store, deleterPID)
		err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			var blocked bool
			if err := executor.QueryRowContext(tx.Context(), "SELECT $1::integer = ANY(pg_blocking_pids($2::integer))", matcherPID, deleterPID).Scan(&blocked); err != nil {
				return err
			}
			if !blocked {
				return errors.New("Unit deletion was not blocked by the captured configuration")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	} else {
		// An edit changes nothing the evaluation depends on, so it must not wait for it.
		select {
		case err := <-deleteDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Unit edit waited for an in-flight evaluation")
		}
	}
	unlock()
	if err := <-matchDone; err != nil {
		t.Fatal(err)
	}
	if deleting {
		if err := <-deleteDone; err != nil {
			t.Fatal(err)
		}
	}
	err = f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		result, err := f.rules.MatchTargets(tx.Context(), tx, f.networks[0], protocols.POCSAG, observed, targets)
		if err != nil {
			return err
		}
		if deleting {
			if result.Revision != 3 || result.Resolutions[0].Status != "unassigned" {
				return errors.New("a new evaluation used stale Unit authority")
			}
		} else if result.Revision != 2 || result.Resolutions[0].Status != "assigned" || result.UnitContext[unit.ID].Revision != 2 {
			return errors.New("a new evaluation used stale current Unit context")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCurrentMatcherEvaluationSerializesWithUnitDeletionNotEdits expects a Unit deletion,
// which invalidates rules, to wait for an in-flight evaluation under the shared assignment
// lock, while a Unit edit commits meanwhile; a later evaluation sees the current Unit either way.
func TestCurrentMatcherEvaluationSerializesWithUnitDeletionNotEdits(t *testing.T) {
	t.Run("edit", func(t *testing.T) { verifyCurrentMatcherSerialization(t, false) })
	t.Run("delete", func(t *testing.T) { verifyCurrentMatcherSerialization(t, true) })
}

func TestCurrentMatcherCreationReceiptsAreActorBound(t *testing.T) {
	f := newMatcherFixture(t)
	command := uuid.New()
	values := units.UnitInput{AgencyID: f.agency, Name: "Station"}
	first, err := f.units.CreateUnit(t.Context(), f.proof, command, values, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	other := f
	other.principal = seedLoginAccount(t, f.store, "Other", true)
	replaceManagementGrants(t, other, []authz.Grant{{Actions: authz.Actions(), Scope: authz.Scope{Kind: "all"}}})
	login, err := identity.NewLoginService(f.store, f.resolver, f.owners.identity, f.facts, identity.DefaultLoginRateLimits())
	if err != nil {
		t.Fatal(err)
	}
	session, err := login.AuthenticateLocal(t.Context(), "test-source", "other", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	other.proof = identity.Proof{Kind: identity.SessionProof, Secret: session.Secret}
	second, err := f.units.CreateUnit(t.Context(), other.proof, command, values, checkedMutation)
	if err != nil || second.ID == first.ID {
		t.Fatalf("another actor received the original actor's command result: %v", err)
	}
	if _, err := f.units.CreateCollection(t.Context(), f.proof, command, "Stations", checkedMutation); !errors.Is(err, commands.ErrIdempotencyConflict) {
		t.Fatalf("one actor reused a command identity for another operation: %v", err)
	}
}
