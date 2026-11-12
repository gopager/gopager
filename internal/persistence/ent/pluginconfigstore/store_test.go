package pluginconfigstore_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/pluginconfigstore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

func TestMain(m *testing.M) { os.Exit(testpostgres.Run(m)) }

var migratedTemplate = testpostgres.NewTemplate("pluginconfigstore", func(ctx context.Context, dsn string) error {
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

// world is a migrated database with one instance, two accounts and one Unit.
type world struct {
	t         *testing.T
	primary   *postgres.Store
	store     *pluginconfigstore.Store
	instances *processingstore.Store
	instance  uuid.UUID
	alice     uuid.UUID
	bob       uuid.UUID
	unit      uuid.UUID
}

func newWorld(t *testing.T) *world {
	t.Helper()
	primary, _, err := postgres.NewStore(migratedTemplate.Database(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := primary.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := pluginconfigstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	instances, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, primary: primary, store: store, instances: instances, instance: uuid.New(),
		alice: uuid.New(), bob: uuid.New(), unit: uuid.New()}
	agency := uuid.New()
	w.within(func(tx platform.TransactionContext) error {
		executor, err := primary.Resolve(tx)
		if err != nil {
			return err
		}
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`INSERT INTO identity_accounts (id, kind, created_at) VALUES ($1, 'human', now()), ($2, 'human', now())`, []any{w.alice, w.bob}},
			{`INSERT INTO agencies (id, name) VALUES ($1, 'Agency')`, []any{agency}},
			{`INSERT INTO units (id, name, agency_id) VALUES ($1, 'Unit', $2)`, []any{w.unit, agency}},
		} {
			if _, err := executor.ExecContext(tx.Context(), statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	})
	w.saveInstance()
	return w
}

func (w *world) saveInstance() {
	w.t.Helper()
	actor := uuid.New()
	authority := authz.Authority{PrincipalID: actor.String(), Revision: 1, Active: true,
		Grants: []authz.Grant{{SubjectID: actor.String(), SubjectKind: "principal", Scope: authz.Scope{Kind: "all"},
			Actions: []string{"plugin.configure"}}}}
	empty := processing.EmptyConfiguration()
	value := processing.InstanceConfiguration{ID: w.instance, Revision: 1, Enabled: true, ExecutionPrincipalID: uuid.New(),
		CreatedBy: actor, Configuration: empty, Main: processing.ProvisionedArtifact{ManifestPath: "target/manifest.json",
			Pin: processing.ArtifactPin{PluginID: "target", Version: "1", CapabilityID: "deliver", Digest: strings.Repeat("d", 64)}}}
	w.within(func(tx platform.TransactionContext) error {
		now, err := w.primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		value.CreatedAt = now
		return w.instances.SaveInstance(tx.Context(), tx, authority, value, 0)
	})
}

func (w *world) within(operation func(tx platform.TransactionContext) error) {
	w.t.Helper()
	if err := w.primary.WithinTransaction(context.Background(), operation); err != nil {
		w.t.Fatalf("%v: %v", err, errors.Unwrap(err))
	}
}

func cipher(t *testing.T) *processing.SecretCipher {
	t.Helper()
	sealer, err := processing.NewSecretCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return sealer
}

func TestSecretsRoundTripAndAccountDeletion(t *testing.T) {
	w := newWorld(t)
	sealer := cipher(t)
	adminRow, err := sealer.Seal(uuid.New(), w.instance, nil, []byte("admin-token"))
	if err != nil {
		t.Fatal(err)
	}
	owned, err := sealer.Seal(uuid.New(), w.instance, &w.alice, []byte("alice-token"))
	if err != nil {
		t.Fatal(err)
	}
	w.within(func(tx platform.TransactionContext) error {
		return w.store.InsertSecrets(tx.Context(), tx, []processing.StoredSecret{adminRow, owned})
	})
	w.within(func(tx platform.TransactionContext) error {
		rows, err := w.store.ReadSecrets(tx.Context(), tx, []uuid.UUID{adminRow.ID, owned.ID, uuid.New()})
		if err != nil || len(rows) != 2 || rows[owned.ID].OwnerID == nil || *rows[owned.ID].OwnerID != w.alice ||
			rows[adminRow.ID].OwnerID != nil || rows[adminRow.ID].CreatedAt.IsZero() {
			t.Fatalf("rows = %+v, %v", rows, err)
		}
		plaintext, err := sealer.Open(rows[owned.ID])
		if err != nil || string(plaintext) != "alice-token" {
			t.Fatalf("stored row does not open: %q, %v", plaintext, err)
		}
		return nil
	})
	if err := w.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return w.store.InsertSecrets(tx.Context(), tx, []processing.StoredSecret{adminRow})
	}); err == nil {
		t.Fatal("an existing Secret row was overwritten")
	}
	w.within(func(tx platform.TransactionContext) error {
		return w.store.DeleteAccountData(tx.Context(), tx, w.alice)
	})
	w.within(func(tx platform.TransactionContext) error {
		rows, err := w.store.ReadSecrets(tx.Context(), tx, []uuid.UUID{adminRow.ID, owned.ID})
		if err != nil || len(rows) != 1 || rows[adminRow.ID].ID != adminRow.ID {
			t.Fatalf("account deletion left or removed the wrong rows: %+v, %v", rows, err)
		}
		return nil
	})
}

func TestPersonalConfigurationRevisionsAndOwners(t *testing.T) {
	w := newWorld(t)
	value := processing.EmptyConfiguration()
	save := func(owner uuid.UUID, expected int64) (int64, error) {
		var revision int64
		err := w.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			revision, err = w.store.SavePersonalConfiguration(tx.Context(), tx, w.instance, owner, expected, value)
			return err
		})
		return revision, err
	}
	if revision, err := save(w.alice, 0); err != nil || revision != 1 {
		t.Fatalf("create = %d, %v", revision, err)
	}
	if _, err := save(w.alice, 0); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("second create = %v, want revision conflict", err)
	}
	if _, err := save(w.alice, 5); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale update = %v, want revision conflict", err)
	}
	if revision, err := save(w.alice, 1); err != nil || revision != 2 {
		t.Fatalf("update = %d, %v", revision, err)
	}
	if revision, err := save(w.bob, 0); err != nil || revision != 1 {
		t.Fatalf("other owner create = %d, %v", revision, err)
	}
	w.within(func(tx platform.TransactionContext) error {
		alice, err := w.store.ReadPersonalConfiguration(tx.Context(), tx, w.instance, w.alice)
		if err != nil || alice.Revision != 2 || alice.Configuration.Kind != processing.ObjectValue {
			t.Fatalf("alice = %+v, %v", alice, err)
		}
		if _, err := w.store.ReadPersonalConfiguration(tx.Context(), tx, w.instance, uuid.New()); !errors.Is(err, authz.ErrNotFound) {
			t.Fatalf("unknown owner = %v, want not found", err)
		}
		return nil
	})
	if err := w.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return w.store.DeletePersonalConfiguration(tx.Context(), tx, w.instance, w.alice, 1)
	}); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale delete = %v", err)
	}
	w.within(func(tx platform.TransactionContext) error {
		return w.store.DeletePersonalConfiguration(tx.Context(), tx, w.instance, w.alice, 2)
	})
	w.within(func(tx platform.TransactionContext) error {
		if err := w.store.DeleteAccountData(tx.Context(), tx, w.bob); err != nil {
			return err
		}
		_, err := w.store.ReadPersonalConfiguration(tx.Context(), tx, w.instance, w.bob)
		if !errors.Is(err, authz.ErrNotFound) {
			t.Fatalf("account deletion kept personal rows: %v", err)
		}
		return nil
	})
}

func TestUnitParameters(t *testing.T) {
	w := newWorld(t)
	text, err := processing.Literal(processing.StringValue, "bell")
	if err != nil {
		t.Fatal(err)
	}
	number, err := processing.Literal(processing.IntegerValue, "2")
	if err != nil {
		t.Fatal(err)
	}
	w.within(func(tx platform.TransactionContext) error {
		exists, err := w.store.UnitExists(tx.Context(), tx, w.unit)
		missing, missingErr := w.store.UnitExists(tx.Context(), tx, uuid.New())
		if err != nil || missingErr != nil || !exists || missing {
			t.Fatalf("UnitExists = %v/%v, %v/%v", exists, err, missing, missingErr)
		}
		return w.store.ReplaceUnitParameters(tx.Context(), tx, w.instance, w.unit,
			map[string]processing.Value{"sound": text, "priority": number})
	})
	w.within(func(tx platform.TransactionContext) error {
		if err := w.store.ReplaceUnitParameters(tx.Context(), tx, w.instance, w.unit,
			map[string]processing.Value{"sound": text}); err != nil {
			return err
		}
		page, err := w.store.ListUnitParameters(tx.Context(), tx, w.instance, nil, processing.PageInput{Limit: 10})
		if err != nil || page.Total != 1 || len(page.Items) != 1 || len(page.Items[0].Values) != 1 ||
			page.Items[0].UnitID != w.unit {
			t.Fatalf("page = %+v, %v", page, err)
		}
		other := uuid.New()
		filtered, err := w.store.ListUnitParameters(tx.Context(), tx, w.instance, &other, processing.PageInput{Limit: 10})
		if err != nil || filtered.Total != 0 || len(filtered.Items) != 0 {
			t.Fatalf("filtered page = %+v, %v", filtered, err)
		}
		return nil
	})
}

func TestUnitParametersRejectUnknownUnit(t *testing.T) {
	w := newWorld(t)
	text, err := processing.Literal(processing.StringValue, "bell")
	if err != nil {
		t.Fatal(err)
	}
	err = w.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return w.store.ReplaceUnitParameters(tx.Context(), tx, w.instance, uuid.New(), map[string]processing.Value{"sound": text})
	})
	if !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("unknown Unit = %v, want not found", err)
	}
}

func TestCurrentInstancesNeedNoAuthority(t *testing.T) {
	w := newWorld(t)
	w.within(func(tx platform.TransactionContext) error {
		list, err := w.instances.ListCurrentInstances(tx.Context(), tx)
		if err != nil || len(list) != 1 || list[0].ID != w.instance || list[0].Revision != 1 {
			t.Fatalf("current instances = %+v, %v", list, err)
		}
		one, err := w.instances.ReadCurrentConfiguration(tx.Context(), tx, w.instance)
		if err != nil || one.ID != w.instance {
			t.Fatalf("current configuration = %+v, %v", one, err)
		}
		if _, err := w.instances.ReadCurrentConfiguration(tx.Context(), tx, uuid.New()); !errors.Is(err, authz.ErrNotFound) {
			t.Fatalf("unknown instance = %v, want not found", err)
		}
		return nil
	})
}
