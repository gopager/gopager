package borrowed_test

import (
	"errors"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/platform"
)

func TestIdentityFactCommitAndAppendFailureAreAtomic(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	appender, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	id, accountID := uuid.New(), uuid.New()
	var saved platform.TransactionContext
	var fact events.IdentityFact
	write := func(tx platform.TransactionContext, name string, target uuid.UUID, invalid bool) error {
		saved = tx
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		now, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		ctx := accountContext(tx.Context(), driver, entql.FieldNotNil("id"), true)
		if err := createAccount(ctx, client, target, name, now); err != nil {
			return err
		}
		fact = events.IdentityFact{
			ID: id, Type: events.ConfigurationChanged,
			Aggregate: events.EntityRef{Type: "account", ID: target}, Revision: 1,
			OccurredAt: now, ChangeKind: events.AccountCreated,
		}
		if invalid {
			fact.ChangeKind = "not-a-classified-change"
		}
		_, err = appender.AppendIdentityFact(tx.Context(), tx, fact)
		return err
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return write(tx, "committed", accountID, false)
	}); err != nil {
		t.Fatal(err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return write(tx, "must-rollback", uuid.New(), true)
	})
	if !errors.Is(err, events.ErrInvalidFact) {
		t.Fatalf("invalid fact did not reject its state transaction: %v", err)
	}
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return write(tx, "duplicate-must-rollback", uuid.New(), false)
	})
	if platform.FailureCode(err) != platform.CodeRevisionConflict {
		t.Fatalf("duplicate fact did not abort: %v", err)
	}
	if _, err := appender.AppendIdentityFact(t.Context(), saved, fact); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("append accepted expired capability: %v", err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var accounts, facts int
		if err := executor.QueryRowContext(tx.Context(),
			`SELECT (SELECT count(*) FROM identity_accounts), (SELECT count(*) FROM domain_events)`).
			Scan(&accounts, &facts); err != nil {
			return err
		}
		if accounts != 1 || facts != 1 {
			t.Fatalf("partial/duplicate transaction: accounts=%d facts=%d", accounts, facts)
		}
		var entity, version, classification, revision string
		if err := executor.QueryRowContext(tx.Context(),
			`SELECT envelope->'aggregateRef'->>'id', envelope->>'schemaVersion',
			        envelope->>'classification', envelope->>'aggregateRevision'
			 FROM domain_events WHERE id = $1`, id).
			Scan(&entity, &version, &classification, &revision); err != nil {
			return err
		}
		if entity != accountID.String() || version != "1" || classification != "authorization" || revision != "1" {
			t.Fatalf("incorrect persisted fact metadata: %q %q %q %q", entity, version, classification, revision)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
