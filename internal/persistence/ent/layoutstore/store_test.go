package layoutstore_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/layoutstore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/presentation"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
	"github.com/gopager/gopager/internal/units"
)

func TestMain(m *testing.M) { os.Exit(testpostgres.Run(m)) }

var migratedTemplate = testpostgres.NewTemplate("layoutstore", func(ctx context.Context, dsn string) error {
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

type fixedClock struct{}

func (fixedClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return time.Date(2026, 11, 8, 12, 0, 0, 123456789, time.UTC), nil
}

// staticResolver authenticates every proof as one prepared caller.
type staticResolver struct{ authentication identity.Authentication }

func (r staticResolver) Resolve(context.Context, platform.TransactionContext, identity.Proof) (identity.Authentication, error) {
	return r.authentication, nil
}

type noFields struct{}

func (noFields) FieldSchema(processing.FieldRef) (processing.FieldSchema, error) {
	return processing.FieldSchema{}, errors.New("no fields")
}

type world struct {
	store    *postgres.Store
	layouts  *layoutstore.Store
	manager  *presentation.Service
	reader   *presentation.Service
	resolver *presentation.Resolver
}

func serviceFor(t *testing.T, store *postgres.Store, layouts *layoutstore.Store, action string) *presentation.Service {
	t.Helper()
	receipts, err := commandstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	principal := uuid.NewString()
	authority := authz.Authority{PrincipalID: principal, Revision: 1, Active: true, Grants: []authz.Grant{{
		ID: uuid.NewString(), SubjectID: principal, SubjectKind: "principal", Actions: []string{action},
		Scope: authz.Scope{Kind: "all"}}}}
	service, err := presentation.NewService(store, staticResolver{identity.Authentication{Authority: authority,
		Kind: identity.SessionProof}}, fixedClock{}, layouts, receipts, noFields{})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func newWorld(t *testing.T) *world {
	t.Helper()
	store, _, err := postgres.NewStore(migratedTemplate.Database(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	layouts, err := layoutstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := presentation.NewResolver(layouts, map[string]processing.Layout{"text": layout("text", "default")})
	if err != nil {
		t.Fatal(err)
	}
	return &world{store: store, layouts: layouts, resolver: resolver,
		manager: serviceFor(t, store, layouts, presentation.ManageAction),
		reader:  serviceFor(t, store, layouts, presentation.SubscriptionManageOwnAction)}
}

func layout(kind, text string) processing.Layout {
	return processing.Layout{SchemaVersion: 1, OutputKind: kind, Blocks: []processing.LayoutBlock{{Kind: "text", Text: text}}}
}

func allow(identity.Authentication) error { return nil }

func TestLayoutLifecycle(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	command := uuid.New()
	input := presentation.LayoutInput{Name: "Alarm", Definition: layout("text", "v1")}
	created, err := w.manager.Create(ctx, identity.Proof{}, command, input, allow)
	if err != nil || created.Revision != 1 || created.OutputKind != "text" {
		t.Fatalf("create = %+v, %v", created, err)
	}

	replay, err := w.manager.Create(ctx, identity.Proof{}, command, input, allow)
	if err != nil || replay.ID != created.ID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	changed := input
	changed.Name = "Other"
	if _, err := w.manager.Create(ctx, identity.Proof{}, command, changed, allow); !errors.Is(err, commands.ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay: %v", err)
	}

	updated, err := w.manager.Update(ctx, identity.Proof{}, created.ID, 1,
		presentation.LayoutInput{Name: "Alarm", Definition: layout("text", "v2")}, allow)
	if err != nil || updated.Revision != 2 || updated.Definition.Blocks[0].Text != "v2" {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if _, err := w.manager.Update(ctx, identity.Proof{}, created.ID, 1, input, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	old, err := w.reader.ReadVersion(ctx, identity.Proof{}, created.ID, 1)
	if err != nil || old.Definition.Blocks[0].Text != "v1" {
		t.Fatalf("version 1 = %+v, %v", old, err)
	}
	if _, err := w.reader.ReadVersion(ctx, identity.Proof{}, created.ID, 3); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("missing version: %v", err)
	}
	read, err := w.reader.Read(ctx, identity.Proof{}, created.ID)
	if err != nil || read.Revision != 2 || !read.UpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatalf("read = %+v, %v", read, err)
	}
	if _, err := w.reader.Update(ctx, identity.Proof{}, created.ID, 2, input, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("reader update: %v", err)
	}

	page, err := w.reader.List(ctx, identity.Proof{}, units.PageInput{})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("list = %+v, %v", page, err)
	}
}

// resolve runs the resolver in a transaction.
func (w *world) resolve(request processing.LayoutRequest) (resolved processing.ResolvedLayout, err error) {
	err = w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		resolved, err = w.resolver.ResolveLayout(tx.Context(), tx, request)
		return err
	})
	return resolved, err
}

func TestResolverReadsCurrentVersionAndDefaults(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	created, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(),
		presentation.LayoutInput{Name: "A", Definition: layout("text", "v1")}, allow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.manager.Update(ctx, identity.Proof{}, created.ID, 1,
		presentation.LayoutInput{Name: "A", Definition: layout("text", "v2")}, allow); err != nil {
		t.Fatal(err)
	}
	resolved, err := w.resolve(processing.LayoutRequest{LayoutID: &created.ID, OutputKind: "text"})
	if err != nil || resolved.ID != created.ID || resolved.Version != 2 || resolved.Layout.Blocks[0].Text != "v2" {
		t.Fatalf("stored = %+v, %v", resolved, err)
	}
	if _, err := w.resolve(processing.LayoutRequest{LayoutID: &created.ID, OutputKind: "json"}); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("kind mismatch: %v", err)
	}
	missing := uuid.New()
	if _, err := w.resolve(processing.LayoutRequest{LayoutID: &missing}); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	resolved, err = w.resolve(processing.LayoutRequest{OutputKind: "text"})
	if err != nil || resolved.ID != uuid.Nil || resolved.Version != 0 || resolved.Layout.Blocks[0].Text != "default" {
		t.Fatalf("default = %+v, %v", resolved, err)
	}
}

func TestDeleteRefusesReferencedLayout(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	input := presentation.LayoutInput{Name: "A", Definition: layout("text", "v1")}
	used, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), input, allow)
	if err != nil {
		t.Fatal(err)
	}
	unused, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), input, allow)
	if err != nil {
		t.Fatal(err)
	}
	err = w.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO subscriptions (id, revision, kind, subscriber_id, created_by,
			state, event_types, layout_id, split_by_unit, created_at, updated_at)
			VALUES ($1, 1, 'global', $2, $2, 'active', ARRAY['message.accepted'], $3, false, now(), now())`,
			uuid.New(), uuid.New(), used.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.manager.Delete(ctx, identity.Proof{}, used.ID, 1, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("referenced delete: %v", err)
	}
	if _, err := w.manager.Delete(ctx, identity.Proof{}, unused.ID, 2, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	result, err := w.manager.Delete(ctx, identity.Proof{}, unused.ID, 1, allow)
	if err != nil || result.ID != unused.ID || result.Revision != 2 {
		t.Fatalf("delete = %+v, %v", result, err)
	}
	if _, err := w.reader.Read(ctx, identity.Proof{}, unused.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("read deleted: %v", err)
	}
	if _, err := w.reader.ReadVersion(ctx, identity.Proof{}, unused.ID, 1); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("versions survived delete: %v", err)
	}
}
