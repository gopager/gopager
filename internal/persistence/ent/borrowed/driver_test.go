package borrowed_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
	"github.com/gopager/gopager/internal/persistence/ent/privacy"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

func TestMain(m *testing.M) {
	code := testpostgres.Run(m)
	pluginfixture.Cleanup()
	os.Exit(code)
}

func newStore(t *testing.T) *postgres.Store {
	t.Helper()
	return newStoreWithMigrations(t, migrations.Files)
}

// migratedTemplate is the database with the release migrations applied, built once per process.
var migratedTemplate = testpostgres.NewTemplate("migrated", func(ctx context.Context, dsn string) error {
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

// storeDSNs maps each fixture store to the DSN of its private database.
var storeDSNs sync.Map

// storeDSN returns the DSN of the database behind a fixture store, for independent observers.
func storeDSN(t testing.TB, store *postgres.Store) string {
	t.Helper()
	dsn, ok := storeDSNs.Load(store)
	if !ok {
		t.Fatal("store was not created by newStoreWithMigrations")
	}
	return dsn.(string)
}

// newStoreWithMigrations opens a store on an isolated database with a chosen release collection
// applied. The release migrations are cloned from a pre-migrated template.
func newStoreWithMigrations(t *testing.T, files fs.FS) *postgres.Store {
	t.Helper()
	migrated := files == migrations.Files
	var dsn string
	if migrated {
		dsn = migratedTemplate.Database(t)
	} else {
		dsn = testpostgres.EmptyDatabase(t)
	}
	store, _, err := postgres.NewStore(dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		storeDSNs.Delete(store)
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	storeDSNs.Store(store, dsn)
	if migrated {
		return store
	}
	if err := store.Migrate(t.Context(), files, platform.MigrationApply); err != nil {
		t.Fatal(err)
	}
	return store
}

func accountContext(ctx context.Context, driver *borrowed.Driver, predicate entql.P, create bool) context.Context {
	return guard.Bind(ctx, guard.Access{
		Predicates: map[string]entql.P{"Account": predicate},
		Create:     map[string]bool{"Account": create}, Current: driver.Current,
	})
}

func createAccount(ctx context.Context, client *ent.Client, id uuid.UUID, name string, now time.Time) error {
	return client.Account.Create().SetID(id).SetKind(account.KindHuman).
		SetUsername(name).SetUsernameKey(name).SetCreatedAt(now).Exec(ctx)
}

func TestBorrowedDriverUsesPlatformRollbackAndExpires(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	var client *ent.Client
	var savedContext context.Context
	abort := errors.New("abort account and audit transaction")
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client = ent.NewClient(ent.Driver(driver))
		savedContext = accountContext(tx.Context(), driver, entql.FieldNotNil("id"), true)
		if err := createAccount(savedContext, client, uuid.New(), "rollback", time.Now().UTC()); err != nil {
			return err
		}
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO identity_audit
			(id, actor_id, target_id, action, outcome, recorded_at)
			VALUES ($1, $2, $3, 'test.rollback', 'accepted', clock_timestamp())`,
			uuid.New(), uuid.New(), uuid.New()); err != nil {
			return err
		}
		appender, err := eventstore.New(store)
		if err != nil {
			return err
		}
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		if _, err := appender.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
			ID: uuid.New(), Type: events.PrincipalAccessChanged,
			Aggregate: events.EntityRef{Type: "account", ID: uuid.New()}, Revision: 1,
			OccurredAt: now, ChangeKind: events.GrantsChanged,
		}); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("rollback cause = %v", err)
	}
	if _, err := client.Account.Query().Count(savedContext); !errors.Is(err, privacy.Deny) {
		t.Fatalf("retained client accepted expired authority: %v", err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var accounts, audit, facts int
		if err := executor.QueryRowContext(tx.Context(),
			`SELECT (SELECT count(*) FROM identity_accounts), (SELECT count(*) FROM identity_audit), (SELECT count(*) FROM domain_events)`).
			Scan(&accounts, &audit, &facts); err != nil {
			return err
		}
		if accounts != 0 || audit != 0 || facts != 0 {
			t.Fatalf("partial rollback: accounts=%d audit=%d facts=%d", accounts, audit, facts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativeAgeBoundaryAndIndependentGrantUnion(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	atBoundary, older := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 26, 12, 0, 0, 123456000, time.UTC)
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := accountContext(tx.Context(), driver, entql.FieldNotNil("id"), true)
		if err := createAccount(ctx, client, atBoundary, "boundary", now.Add(-time.Hour)); err != nil {
			return err
		}
		if err := createAccount(ctx, client, older, "older", now.Add(-time.Hour-time.Microsecond)); err != nil {
			return err
		}
		age := int64(3600)
		principal := uuid.NewString()
		authority := authz.Authority{PrincipalID: principal, Active: true, Revision: 1,
			Grants: []authz.Grant{{
				SubjectID: principal, SubjectKind: "principal", Actions: []string{"account.read"},
				Scope: authz.Scope{Kind: "all"}, MaxAgeSeconds: &age,
			}}, FieldPermissions: []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
				SubjectID: principal, SubjectKind: "principal", Fields: []string{"payload"}}}}
		check := func(expected int) error {
			plan, err := permissions.ReadScope(authority, "account.read", []string{"payload"},
				permissions.Attributes{Account: "id", AgeAnchor: "created_at"}, now)
			if err != nil {
				return err
			}
			count, err := client.Account.Query().Count(accountContext(tx.Context(), driver, plan.Predicate, false))
			if err != nil || count != expected {
				t.Fatalf("inclusive age/union count = %d, want %d: %v", count, expected, err)
			}
			ids, err := client.Account.Query().IDs(accountContext(tx.Context(), driver, plan.Predicate, false))
			if err != nil {
				return err
			}
			for _, candidate := range []struct {
				id     uuid.UUID
				anchor time.Time
			}{{atBoundary, now.Add(-time.Hour)}, {older, now.Add(-time.Hour - time.Microsecond)}} {
				pointErr := authz.Authorize(authority, "account.read", []string{"payload"},
					authz.Target{AccountID: candidate.id, AgeAnchor: candidate.anchor}, now)
				if pointErr != nil && !errors.Is(pointErr, authz.ErrForbidden) {
					return pointErr
				}
				if (pointErr == nil) != slices.Contains(ids, candidate.id) {
					t.Fatal("point decision differs from the native Ent query result")
				}
			}
			return nil
		}
		if err := check(1); err != nil {
			return err
		}
		// Unlimited eligibility is independent of global field permissions.
		authority.Grants = append(authority.Grants, authz.Grant{
			SubjectID: principal, SubjectKind: "principal", Actions: []string{"account.read"},
			Scope: authz.Scope{Kind: "all"},
		})
		if err := check(2); err != nil {
			return err
		}
		authority.Key = &authz.Restrictions{Actions: []string{"account.read"},
			Scopes: []authz.Scope{{Kind: "all"}}, Fields: []string{"payload"}, MaxAgeSeconds: &age}
		return check(1)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativePrivacyFiltersCountPageAndMutations(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	visible, hidden := uuid.New(), uuid.New()
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		if _, err := client.Account.Query().Count(tx.Context()); !errors.Is(err, privacy.Deny) {
			t.Fatalf("missing context allowed: %v", err)
		}
		ctx := accountContext(tx.Context(), driver, entql.FieldNotNil("id"), true)
		for id, name := range map[uuid.UUID]string{visible: "z-visible", hidden: "a-hidden"} {
			if err := createAccount(ctx, client, id, name, time.Now().UTC()); err != nil {
				return err
			}
		}
		scoped := accountContext(tx.Context(), driver, entql.FieldEQ("id", visible), false)
		count, err := client.Account.Query().Count(scoped)
		if err != nil || count != 1 {
			t.Fatalf("count leaked hidden account: %d %v", count, err)
		}
		page, err := client.Account.Query().Order(ent.Asc(account.FieldUsername)).Limit(1).All(scoped)
		if err != nil || len(page) != 1 || page[0].ID != visible {
			t.Fatalf("paging was not filtered before LIMIT: %v %v", page, err)
		}
		updated, err := client.Account.Update().SetDisplayName("changed").Save(scoped)
		if err != nil || updated != 1 {
			t.Fatalf("mutation scope = %d %v", updated, err)
		}
		if err := createAccount(scoped, client, uuid.New(), "forbidden", time.Now()); !errors.Is(err, privacy.Deny) {
			t.Fatalf("create permission was inferred from read scope: %v", err)
		}
		if _, err := client.Account.Query().Count(privacy.DecisionContext(scoped, privacy.Allow)); err == nil {
			t.Fatal("borrowed driver accepted privacy Allow bypass")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativeReadScopeKeepsGlobalFieldsSeparateFromScopedDataAndAge(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	first, second := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 26, 12, 0, 0, 123456000, time.UTC)
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := accountContext(tx.Context(), driver, entql.FieldNotNil("id"), true)
		if err := createAccount(ctx, client, first, "boundary", now.Add(-time.Hour)); err != nil {
			return err
		}
		if err := createAccount(ctx, client, second, "other", now.Add(-time.Second)); err != nil {
			return err
		}
		age := int64(3600)
		principal := uuid.New().String()
		authority := authz.Authority{PrincipalID: principal, Active: true, Revision: 1,
			Grants: []authz.Grant{
				{SubjectID: principal, SubjectKind: "principal", Actions: []string{"account.read"},
					Scope: authz.Scope{Kind: "all"}, MaxAgeSeconds: &age},
				{SubjectID: principal, SubjectKind: "principal", Actions: []string{"account.read"},
					Scope: authz.Scope{Kind: "account", References: []string{first.String()}}},
			}, FieldPermissions: []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
				SubjectID: principal, SubjectKind: "principal", Fields: []string{"payload"}}}}
		plan, err := permissions.ReadScope(authority, "account.read", []string{"payload"},
			permissions.Attributes{Account: "id", AgeAnchor: "created_at"}, now)
		if err != nil {
			return err
		}
		scoped := accountContext(tx.Context(), driver, plan.Predicate, false)
		ids, err := client.Account.Query().IDs(scoped)
		if err != nil || len(ids) != 2 || !slices.Contains(ids, first) || !slices.Contains(ids, second) {
			t.Fatalf("global fields were restricted to one data grant: %v %v", ids, err)
		}
		authority.Key = &authz.Restrictions{Actions: []string{"account.read"},
			Scopes: []authz.Scope{{Kind: "account", References: []string{second.String()}}}, Fields: []string{"payload"}}
		plan, err = permissions.ReadScope(authority, "account.read", []string{"payload"},
			permissions.Attributes{Account: "id", AgeAnchor: "created_at"}, now)
		if err != nil {
			return err
		}
		count, err := client.Account.Query().Count(accountContext(tx.Context(), driver, plan.Predicate, false))
		if err != nil || count != 1 {
			t.Fatalf("key widened owner scope: %d %v", count, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
