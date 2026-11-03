package borrowed_test

import (
	"context"
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
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/credential"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// matcherOwners supplies declared standalone fixture Networks, not production DOM-01 composition.
type matcherOwners struct {
	identity *identitystore.Store
	units    *unitstore.Store
	networks map[uuid.UUID]protocols.Protocol
}

func (o matcherOwners) ReadNetworkProtocol(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, error) {
	value, found := o.networks[id]
	if !found {
		return "", authz.ErrNotFound
	}
	return value, ctx.Err()
}

func (o matcherOwners) ScopeExists(ctx context.Context, tx platform.TransactionContext, scope authz.Scope) (bool, error) {
	switch scope.Kind {
	case "unit", "unitCollection":
		return o.units.ScopeExists(ctx, tx, scope)
	case "network":
		for _, reference := range scope.References {
			id, err := uuid.Parse(reference)
			if err != nil {
				return false, err
			}
			if _, found := o.networks[id]; !found {
				return false, nil
			}
		}
		return true, ctx.Err()
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
	owners := matcherOwners{identity: repository, units: unitRepository,
		networks: map[uuid.UUID]protocols.Protocol{networks[0]: protocols.POCSAG, networks[1]: protocols.POCSAG, networks[2]: protocols.FLEX}}
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
	session, err := login.AuthenticateLocal(t.Context(), "operator", "initial-password")
	if err != nil {
		t.Fatal(err)
	}
	unitService, err := units.NewService(store, resolver, store, unitRepository, ruleRepository, receipts, facts)
	if err != nil {
		t.Fatal(err)
	}
	ruleService, err := assignments.NewService(store, resolver, store, ruleRepository, unitRepository, owners, receipts, facts)
	if err != nil {
		t.Fatal(err)
	}
	return matcherFixture{store: store, units: unitService, rules: ruleService, unitStore: unitRepository,
		ruleStore: ruleRepository, resolver: resolver, owners: owners, facts: facts, receipts: receipts,
		proof: identity.Proof{Kind: identity.SessionProof, Secret: session.Secret}, principal: principal, networks: networks}
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

func TestCurrentMatcherAtomicCreationRetryPreviewAndRollback(t *testing.T) {
	f := newMatcherFixture(t)
	command := uuid.New()
	input := units.UnitInput{Name: "Station", CollectionIDs: []uuid.UUID{}}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, command, input, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	before := configurationCounts(t, f)
	retry, err := f.units.CreateUnit(t.Context(), f.proof, command, input, checkedMutation)
	if err != nil || retry != unit || configurationCounts(t, f) != before {
		t.Fatalf("Unit retry duplicated state or facts: %v", err)
	}
	input.Name = "changed"
	if _, err := f.units.CreateUnit(t.Context(), f.proof, command, input, checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("conflicting Unit command reuse accepted: %v", err)
	}
	ruleCommand := uuid.New()
	ruleValues := ruleInput(f.networks[0], unit.ID, 10, 20, 3, 1)
	if err := f.rules.Preview(t.Context(), f.proof, uuid.Nil, 0, ruleValues); err != nil || configurationCounts(t, f) != before {
		t.Fatalf("preview changed configuration: %v", err)
	}
	rule, err := f.rules.CreateRule(t.Context(), f.proof, ruleCommand, ruleValues, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	before = configurationCounts(t, f)
	ruleValues.Selector.Functions = []uint16{1, 3}
	retry, err = f.rules.CreateRule(t.Context(), f.proof, ruleCommand, ruleValues, checkedMutation)
	if err != nil || retry != rule || configurationCounts(t, f) != before {
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
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{Name: "Station"}, checkedMutation)
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
		units.UnitInput{Name: "Station", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation)
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
	shared, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{Name: "Shared", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{Name: "Hidden"}, checkedMutation)
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
		Scope:   authz.Scope{Kind: "network", References: []string{f.networks[0].String()}},
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
	before := configurationCounts(t, f)
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, shared.ID, shared.Revision,
		units.UnitInput{Name: "unauthorized shared rename", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("shared hidden Network use permitted rename: %v", err)
	}
	if _, err := f.units.DeleteCollection(t.Context(), f.proof, collection.ID, collection.Revision, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("collection deletion skipped hidden member use: %v", err)
	}
	if configurationCounts(t, f) != before {
		t.Fatal("denied all-use mutation changed state")
	}
	if err := f.rules.Preview(t.Context(), f.proof, uuid.Nil, 0, ruleInput(f.networks[0], shared.ID, 10, 10)); !errors.Is(err, assignments.ErrRuleConflict) {
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
		unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{Name: "Station", CollectionIDs: memberships}, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = unit.ID
		network := f.networks[1]
		if i == 0 {
			network = f.networks[0]
		}
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
		{Actions: actions, Scope: authz.Scope{Kind: "network", References: []string{f.networks[0].String()}}},
		{Actions: actions, Scope: authz.Scope{Kind: "unit", References: []string{ids[1].String()}}},
		{Actions: actions, Scope: authz.Scope{Kind: "unitCollection", References: []string{collection.ID.String()}}},
	})
	for _, scope := range []authz.Scope{
		{Kind: "network", References: []string{f.networks[1].String()}},
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
				networks, err := f.ruleStore.NetworksForUnit(tx.Context(), tx, id)
				if err != nil {
					return err
				}
				target := authz.Target{NetworkID: networks[0], UnitID: id, UnitCollections: unit.CollectionIDs}
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
			if err := f.rules.Preview(t.Context(), proof, uuid.Nil, 0, ruleInput(f.networks[1], ids[0], 20, 20)); !errors.Is(err, assignments.ErrRuleConflict) {
				t.Fatalf("hidden conflicting rule was ignored: %v", err)
			}
		}
	}
}

func TestCurrentMatcherCollectionScopeDeletionAndActualRevisionExhaustion(t *testing.T) {
	f := newMatcherFixture(t)
	collection, err := f.units.CreateCollection(t.Context(), f.proof, uuid.New(), "Stations", checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{Name: "Station", CollectionIDs: []uuid.UUID{collection.ID}}, checkedMutation)
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
	if _, err := f.units.ChangeUnit(t.Context(), f.proof, unit.ID, math.MaxInt64, units.UnitInput{Name: "Exhausted"},
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
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{Name: "Station"}, checkedMutation)
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
			_, err = service.ChangeUnit(t.Context(), f.proof, unit.ID, 1, units.UnitInput{Name: "Station changed"}, checkedMutation)
		}
		deleteDone <- err
	}()
	var deleterPID int
	select {
	case deleterPID = <-mutations.started:
	case err := <-deleteDone:
		t.Fatalf("deletion failed before coordinated invalidation: %v", err)
	}
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
	unlock()
	if err := <-matchDone; err != nil {
		t.Fatal(err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatal(err)
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

func TestCurrentMatcherEvaluationSerializesWithUnitChanges(t *testing.T) {
	t.Run("edit", func(t *testing.T) { verifyCurrentMatcherSerialization(t, false) })
	t.Run("delete", func(t *testing.T) { verifyCurrentMatcherSerialization(t, true) })
}

func TestCurrentMatcherCreationReceiptsAreActorBound(t *testing.T) {
	f := newMatcherFixture(t)
	command := uuid.New()
	values := units.UnitInput{Name: "Station"}
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
	session, err := login.AuthenticateLocal(t.Context(), "other", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	other.proof = identity.Proof{Kind: identity.SessionProof, Secret: session.Secret}
	second, err := f.units.CreateUnit(t.Context(), other.proof, command, values, checkedMutation)
	if err != nil || second.ID == first.ID {
		t.Fatalf("another actor received the original actor's command result: %v", err)
	}
	if _, err := f.units.CreateCollection(t.Context(), f.proof, command, "Stations", checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("one actor reused a command identity for another operation: %v", err)
	}
}
