package borrowed_test

import (
	"errors"
	"strings"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestProcessingInstanceImmutableSnapshotsCASAndAudit(t *testing.T) {
	primary := newStore(t)
	store, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	allowed := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	allowed.Grants[0].Actions = []string{"plugin.configure"}
	denied := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	config, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
	if err != nil {
		t.Fatal(err)
	}
	instance := processing.InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: true,
		ExecutionPrincipalID: uuid.New(), CreatedBy: actor, Configuration: config,
		Main: processing.ProvisionedArtifact{ManifestPath: "processor/manifest.json",
			Pin: processing.ArtifactPin{PluginID: "processor", Version: "1", CapabilityID: "convert",
				Digest: strings.Repeat("a", 64)}}}
	// This repository fixture proves persistence, not provisioned artifact or Principal readiness.
	save := func(authority authz.Authority, value processing.InstanceConfiguration, expected int64, abort error) error {
		return primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			now, err := primary.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			value.CreatedAt = now
			if err := store.SaveInstance(tx.Context(), tx, authority, value, expected); err != nil {
				return err
			}
			if abort != nil {
				return abort
			}
			return nil
		})
	}
	read := func(revision int64) (processing.InstanceConfiguration, error) {
		var result processing.InstanceConfiguration
		err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			result, err = store.ReadInstance(tx.Context(), tx, instance.ID, revision)
			return err
		})
		return result, err
	}
	if err := save(denied, instance, 0, nil); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Workflow CRUD implicitly configured installed module: %v", err)
	}
	if err := save(allowed, instance, 0, nil); err != nil {
		t.Fatal(err)
	}
	first, err := read(1)
	if err != nil || !first.Enabled || first.ExecutionPrincipalID != instance.ExecutionPrincipalID ||
		first.Main != instance.Main || first.Revision != 1 {
		t.Fatalf("current instance capture: %+v err=%v", first, err)
	}
	if err := save(allowed, instance, 0, nil); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("duplicate creation replaced instance: %v", err)
	}
	abort := errors.New("instance rollback")
	instance.Revision, instance.Enabled = 2, false
	if err := save(allowed, instance, 1, abort); !errors.Is(err, abort) {
		t.Fatalf("rollback did not reach persisted participants: %v", err)
	}
	if _, err := read(1); err != nil {
		t.Fatalf("rollback advanced current pointer: %v", err)
	}
	if _, err := read(2); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("rolled-back snapshot became current: %v", err)
	}
	if err := save(allowed, instance, 1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := read(1); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("replaced revision still available for new starts: %v", err)
	}
	disabled, err := read(2)
	if err != nil || disabled.Enabled {
		t.Fatalf("disabled current instance lost: %+v err=%v", disabled, err)
	}
	if err := save(allowed, instance, 1, nil); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale configuration write succeeded: %v", err)
	}
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(primary, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{
				"ProcessingInstanceVersion": entql.FieldEQ("instance_id", instance.ID),
				"AuditRecord":               entql.FieldEQ("target_id", instance.ID),
			}})
		rows, err := client.ProcessingInstanceVersion.Query().All(scope)
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("lost immutable history or persisted aborted snapshot: %d", len(rows))
		}
		for _, row := range rows {
			if row.Revision == 1 && (!row.Configuration.Enabled ||
				row.Configuration.Main.Pin.Digest != first.Main.Pin.Digest) {
				t.Fatal("first snapshot was mutated by disable")
			}
		}
		count, err := client.AuditRecord.Query().Count(scope)
		if err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("audit not atomic with accepted revisions: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
