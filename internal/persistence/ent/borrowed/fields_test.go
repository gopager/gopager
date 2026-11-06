package borrowed_test

import (
	"errors"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/fieldpermission"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
)

func TestGlobalFieldGroupMembershipRevocationAndAtomicFacts(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	group, membership := uuid.New(), uuid.New()
	facts, err := eventstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("rollback global permission fixture")
	before := configurationCounts(t, f)
	mutate := func(remove bool, rollback bool) error {
		return f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			if _, err := f.owners.identity.LockAccess(tx.Context(), tx); err != nil {
				return err
			}
			driver, err := borrowed.New(f.store, tx)
			if err != nil {
				return err
			}
			client := ent.NewClient(ent.Driver(driver))
			scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
				Create:     map[string]bool{"UserGroup": true, "Membership": true, "FieldPermission": true},
				Predicates: map[string]entql.P{"Membership": entql.FieldEQ("id", membership)}})
			if remove {
				if err := client.Membership.DeleteOneID(membership).Exec(scope); err != nil {
					return err
				}
			} else {
				if err := client.UserGroup.Create().SetID(group).SetName("global-fields").Exec(scope); err != nil {
					return err
				}
				if err := client.Membership.Create().SetID(membership).SetAccountID(f.principal).SetGroupID(group).Exec(scope); err != nil {
					return err
				}
				if err := client.FieldPermission.Create().SetSubjectID(group).SetSubjectKind(fieldpermission.SubjectKindUserGroup).
					SetFieldNames([]string{"payload"}).Exec(scope); err != nil {
					return err
				}
			}
			now, err := f.store.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			if err := f.owners.identity.RecordConfigurationChange(tx.Context(), tx, f.principal, f.principal, "grant.manage", now); err != nil {
				return err
			}
			revision, err := f.owners.identity.LockAccess(tx.Context(), tx)
			if err != nil {
				return err
			}
			if _, err := facts.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{ID: uuid.New(),
				Type: events.PrincipalAccessChanged, Aggregate: events.EntityRef{Type: "account", ID: f.principal},
				Revision: revision, OccurredAt: now, ChangeKind: events.MembershipChanged}); err != nil {
				return err
			}
			if rollback {
				return failure
			}
			return nil
		})
	}
	checkFields := func(payload bool) {
		t.Helper()
		err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			authentication, err := f.resolver.Resolve(tx.Context(), tx, f.proof)
			if err != nil {
				return err
			}
			if err := authz.AuthorizeFields(authentication.Authority, []string{"unit", "address"}); err != nil {
				return err
			}
			execution, err := f.resolver.ResolvePrincipalAuthority(tx.Context(), tx, f.principal)
			if err != nil {
				return err
			}
			if execution.PrincipalID != authentication.Authority.PrincipalID || execution.Key != nil ||
				len(execution.Grants) != len(authentication.Authority.Grants) ||
				(authz.AuthorizeFields(execution, []string{"payload"}) == nil) != payload {
				t.Fatal("internal current-Principal resolution diverged from stored group authority")
			}
			err = authz.AuthorizeFields(authentication.Authority, []string{"payload"})
			if (err == nil) != payload || (err != nil && !errors.Is(err, authz.ErrForbidden)) {
				t.Fatalf("current membership field resolution differs: %v, want payload=%v", err, payload)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	checkFields(false)
	if err := mutate(false, true); !errors.Is(err, failure) || configurationCounts(t, f) != before {
		t.Fatalf("failed global field transaction changed revision/audit/fact: %v", err)
	}
	checkFields(false)
	if err := mutate(false, false); err != nil {
		t.Fatal(err)
	}
	checkFields(true)
	after := configurationCounts(t, f)
	if after[2] != before[2]+1 || after[3] != before[3]+1 || after[7] != before[7]+1 {
		t.Fatalf("global group change missed atomic authority/audit/fact: %v -> %v", before, after)
	}
	if err := mutate(true, true); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	checkFields(true)
	if err := mutate(true, false); err != nil {
		t.Fatal(err)
	}
	checkFields(false)
}
