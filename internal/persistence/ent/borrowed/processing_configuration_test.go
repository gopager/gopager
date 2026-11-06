package borrowed_test

import (
	"errors"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/jackc/pgx/v5/pgconn"
)

type processingDeclarationsFixture struct{}

func (processingDeclarationsFixture) FieldSchema(ref processing.FieldRef) (processing.FieldSchema, error) {
	if ref.Namespace != "source" || len(ref.Path) != 1 || ref.Path[0] != "flag" {
		return processing.FieldSchema{}, authz.ErrInvalidInput
	}
	return processing.FieldSchema{Kind: processing.BooleanValue}, nil
}

// FieldGroups declares the fixture field in a group the bootstrap author may read, so save-time
// field validation can classify the definition.
func (processingDeclarationsFixture) FieldGroups(processing.FieldRef) ([]string, error) {
	return []string{"unit"}, nil
}

func preparedProcessingDefinition(t *testing.T) processing.Definition {
	t.Helper()
	flag := processing.FieldRef{Namespace: "source", Path: []string{"flag"}}
	value, err := processing.Literal(processing.BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	definition := processing.Definition{SchemaVersion: 1, Selector: processing.Predicate{
		Op: "eq", Field: &flag, Value: &value}, OutputSchema: map[string]processing.FieldSchema{},
		Outputs: map[string]processing.Assignment{}}
	if _, err := processing.CompileWorkflow(t.Context(), definition, processingDeclarationsFixture{}, nil); err != nil {
		t.Fatal(err)
	}
	return definition
}

func processingAuthorityFixture(actor uuid.UUID, scopes ...authz.Scope) authz.Authority {
	authority := authz.Authority{PrincipalID: actor.String(), Revision: 1, Active: true}
	for _, scope := range scopes {
		authority.Grants = append(authority.Grants, authz.Grant{SubjectID: actor.String(), SubjectKind: "principal",
			Scope: scope, Actions: []string{"rule.read", "rule.create", "rule.update", "rule.activate", "rule.delete"}})
	}
	return authority
}

func TestProcessingConfigurationNativeUnionBeforeCountPagingAndVersionLookup(t *testing.T) {
	primary := newStore(t)
	store, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	actor, network1, network2 := uuid.New(), uuid.New(), uuid.New()
	admin := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	scopes := []authz.Scope{{Kind: "all"}, {Kind: "network", References: []string{network1.String()}},
		{Kind: "network", References: []string{network2.String()}},
		{Kind: "network", References: []string{network1.String(), network2.String()}}}
	configs := make([]processing.WorkflowConfiguration, len(scopes))
	versions := make([]processing.Version, len(scopes))
	definition := preparedProcessingDefinition(t)
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		for i, scope := range scopes {
			configs[i] = processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1,
				Name: uuid.NewString(), Scope: scope}
			if err := store.SaveWorkflow(tx.Context(), tx, admin, "rule.create", configs[i], 0, now); err != nil {
				return err
			}
			versions[i] = processing.Version{ID: uuid.New(), WorkflowID: configs[i].ID,
				Definition: definition, CreatedBy: actor, CreatedAt: now}
			if err := store.SaveVersion(tx.Context(), tx, versions[i]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	check := func(authority authz.Authority, total int, permitted []uuid.UUID) {
		t.Helper()
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			now, err := primary.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			page, err := store.ListWorkflows(tx.Context(), tx, authority, processing.PageInput{Limit: 1}, now)
			if err != nil {
				return err
			}
			if page.Total != total || len(page.Items) != 1 {
				t.Fatalf("scope count/page leaked: total=%d items=%d want=%d", page.Total, len(page.Items), total)
			}
			for i, value := range configs {
				visible := false
				for _, id := range permitted {
					visible = visible || value.ID == id
				}
				_, readErr := store.ReadWorkflow(tx.Context(), tx, authority, "rule.read", value.ID, now)
				_, versionErr := store.ReadVersion(tx.Context(), tx, authority, value.ID, versions[i].ID, now)
				if visible && (readErr != nil || versionErr != nil) {
					t.Fatalf("permitted configuration hidden: %v %v", readErr, versionErr)
				}
				if !visible && (!errors.Is(readErr, authz.ErrNotFound) || !errors.Is(versionErr, authz.ErrNotFound)) {
					t.Fatalf("hidden Workflow/version became visible: %v %v", readErr, versionErr)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	networkOnly := processingAuthorityFixture(actor, scopes[1])
	check(networkOnly, 1, []uuid.UUID{configs[1].ID})
	union := processingAuthorityFixture(actor, scopes[1], scopes[2])
	check(union, 3, []uuid.UUID{configs[1].ID, configs[2].ID, configs[3].ID})
	union.Key = &authz.Restrictions{Actions: []string{"rule.read"}, Scopes: []authz.Scope{scopes[1]}}
	check(union, 1, []uuid.UUID{configs[1].ID})
}

func TestProcessingConfigurationSaveActivationCASAndAllParticipantsRollback(t *testing.T) {
	primary := newStore(t)
	store, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := commandstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	actor, commandID := uuid.New(), uuid.New()
	authority := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	config := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1, Name: "saved but inactive",
		Scope: authz.Scope{Kind: "all"}}
	definition := preparedProcessingDefinition(t)
	savedVersion := processing.Version{ID: uuid.New(), WorkflowID: config.ID, Definition: definition, CreatedBy: actor}
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		savedVersion.CreatedAt = now
		if err := store.SaveWorkflow(tx.Context(), tx, authority, "rule.create", config, 0, now); err != nil {
			return err
		}
		return store.SaveVersion(tx.Context(), tx, savedVersion)
	}); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("abort complete Processing change")
	activate := func(tx platform.TransactionContext) error {
		if _, err := store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		current, err := store.ReadWorkflow(tx.Context(), tx, authority, "rule.activate", config.ID, now)
		if err != nil {
			return err
		}
		if current.ActiveVersionID != nil {
			t.Fatal("saving an immutable version activated it")
		}
		current.Revision = 2
		current.ActiveVersionID, current.Enabled = &savedVersion.ID, true
		if err := store.SaveWorkflow(tx.Context(), tx, authority, "rule.activate", current, 1, now); err != nil {
			return err
		}
		if err := store.RecordChange(tx.Context(), tx, actor, current.ID, "rule.activate", now); err != nil {
			return err
		}
		fingerprint, err := commands.Fingerprint(current)
		if err != nil {
			return err
		}
		if err := receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor,
			CommandID: commandID, Operation: "processing-fixture", Fingerprint: fingerprint,
			Result: commands.Result{ID: current.ID, Revision: current.Revision}, RecordedAt: now}); err != nil {
			return err
		}
		if _, err := facts.AppendProcessingConfiguration(tx.Context(), tx, events.ProcessingConfigurationFact{
			ID: uuid.New(), WorkflowID: current.ID, Revision: current.Revision,
			OccurredAt: now, ChangeKind: "activated", CausationID: &commandID}); err != nil {
			return err
		}
		return nil
	}
	err = primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if err := activate(tx); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("atomic fixture did not propagate failure: %v", err)
	}
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		current, err := store.ReadWorkflow(tx.Context(), tx, authority, "rule.read", config.ID, now)
		if err != nil {
			return err
		}
		if current.Revision != 1 || current.Enabled || current.ActiveVersionID != nil {
			t.Fatalf("aborted activation escaped rollback: %+v", current)
		}
		executor, err := primary.Resolve(tx)
		if err != nil {
			return err
		}
		var revision, shared int64
		var audit, facts, receipts int
		if err := executor.QueryRowContext(tx.Context(),
			`SELECT (SELECT revision FROM processing_control WHERE id=1),
			        (SELECT access_revision FROM identity_control WHERE id=1),
			        (SELECT count(*) FROM identity_audit), (SELECT count(*) FROM domain_events),
			        (SELECT count(*) FROM command_receipts)`).
			Scan(&revision, &shared, &audit, &facts, &receipts); err != nil {
			return err
		}
		if revision != 1 || shared != 1 || audit != 0 || facts != 0 || receipts != 0 {
			t.Fatalf("partial config transaction: own/shared=%d/%d audit/facts/receipts=%d/%d/%d",
				revision, shared, audit, facts, receipts)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := primary.WithinTransaction(t.Context(), activate); err != nil {
		t.Fatal(err)
	}
	err = primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		stale := config
		stale.Revision, stale.Name = 2, "stale overwrite"
		return store.SaveWorkflow(tx.Context(), tx, authority, "rule.update", stale, 1, now)
	})
	if !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale metadata write was not rejected: %v", err)
	}
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		current, err := store.ReadWorkflow(tx.Context(), tx, authority, "rule.read", config.ID, now)
		if err != nil {
			return err
		}
		if current.Revision != 2 || !current.Enabled || current.ActiveVersionID == nil ||
			*current.ActiveVersionID != savedVersion.ID || current.Name != config.Name {
			t.Fatalf("committed activation lost or overwritten: %+v", current)
		}
		receipt, found, err := receipts.ReadCommandReceipt(tx.Context(), tx, actor, commandID)
		if err != nil {
			return err
		}
		if !found || receipt.Result.ID != current.ID || receipt.Result.Revision != 2 {
			t.Fatalf("committed activation receipt missing: %+v found=%t", receipt, found)
		}
		executor, err := primary.Resolve(tx)
		if err != nil {
			return err
		}
		var revision, shared int64
		var audit, factCount, receiptCount int
		if err := executor.QueryRowContext(tx.Context(),
			`SELECT (SELECT revision FROM processing_control WHERE id=1),
			        (SELECT access_revision FROM identity_control WHERE id=1),
			        (SELECT count(*) FROM identity_audit), (SELECT count(*) FROM domain_events),
			        (SELECT count(*) FROM command_receipts)`).
			Scan(&revision, &shared, &audit, &factCount, &receiptCount); err != nil {
			return err
		}
		if revision != 2 || shared != 2 || audit != 1 || factCount != 1 || receiptCount != 1 {
			t.Fatalf("committed participant counts wrong: own/shared=%d/%d audit/facts/receipts=%d/%d/%d",
				revision, shared, audit, factCount, receiptCount)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessingSchemaDeferredVersionOwnershipAndPrivacy(t *testing.T) {
	primary := newStore(t)
	store, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	authority := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	first, second, foreignVersion := uuid.New(), uuid.New(), uuid.New()
	definition := preparedProcessingDefinition(t)
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		for _, id := range []uuid.UUID{first, second} {
			if err := store.SaveWorkflow(tx.Context(), tx, authority, "rule.create",
				processing.WorkflowConfiguration{ID: id, Revision: 1, Name: id.String(), Scope: authz.Scope{Kind: "all"}}, 0, now); err != nil {
				return err
			}
		}
		return store.SaveVersion(tx.Context(), tx, processing.Version{ID: foreignVersion, WorkflowID: second,
			CreatedBy: actor, CreatedAt: now, Definition: definition})
	}); err != nil {
		t.Fatal(err)
	}
	err = primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(primary, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		if _, err := client.Workflow.Query().Count(tx.Context()); err == nil {
			t.Fatal("unguarded new schema query was accepted")
		}
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{"Workflow": entql.FieldEQ("id", first)}})
		return client.Workflow.UpdateOneID(first).SetActiveVersionID(foreignVersion).Exec(scope)
	})
	var databaseError *pgconn.PgError
	if err == nil || !errors.As(err, &databaseError) || databaseError.Code != "23503" ||
		platform.FailureCode(err) == platform.CodeCommitUncertain {
		t.Fatalf("foreign active version escaped deferred owner FK or became uncertain: %v", err)
	}
}
