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
	"github.com/gopager/gopager/internal/fields"
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

type builtinFields struct{}

func (builtinFields) Registry(context.Context, platform.TransactionContext) (*fields.Registry, error) {
	return fields.New(fields.Builtin()), nil
}

type world struct {
	agency   uuid.UUID
	store    *postgres.Store
	layouts  *layoutstore.Store
	manager  *presentation.Service
	reader   *presentation.Service
	resolver *presentation.Resolver
}

func serviceFor(t *testing.T, store *postgres.Store, layouts *layoutstore.Store, action string) *presentation.Service {
	t.Helper()
	return serviceWithScope(t, store, layouts, action, authz.Scope{Kind: "all"})
}

// serviceWithScope grants the action within scope.
func serviceWithScope(t *testing.T, store *postgres.Store, layouts *layoutstore.Store, action string, scope authz.Scope) *presentation.Service {
	t.Helper()
	receipts, err := commandstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	principal := uuid.NewString()
	authority := authz.Authority{PrincipalID: principal, Revision: 1, Active: true, Grants: []authz.Grant{{
		ID: uuid.NewString(), SubjectID: principal, SubjectKind: "principal", Actions: []string{action},
		Scope: scope}}}
	service, err := presentation.NewService(store, staticResolver{identity.Authentication{Authority: authority,
		Kind: identity.SessionProof}}, fixedClock{}, layouts, receipts, builtinFields{})
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
	return &world{store: store, layouts: layouts, resolver: resolver, agency: seedAgency(t, store),
		manager: serviceFor(t, store, layouts, presentation.ManageAction),
		reader:  serviceFor(t, store, layouts, presentation.ReadAction)}
}

// seedAgency inserts an Agency, which layouts must be assigned to.
func seedAgency(t *testing.T, store *postgres.Store) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO agencies (id, name) VALUES ($1, 'Agency')`, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func layout(kind, text string) processing.Layout {
	return processing.Layout{SchemaVersion: 1, OutputKind: kind, Blocks: []processing.LayoutBlock{{Kind: "text", Text: text}}}
}

// input returns a valid layout input in the world's Agency.
func (w *world) input(name, text string) presentation.LayoutInput {
	return presentation.LayoutInput{Name: name, Definition: layout("text", text), Agencies: []uuid.UUID{w.agency}}
}

func allow(identity.Authentication) error { return nil }

func TestLayoutLifecycle(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	command := uuid.New()
	input := w.input("Alarm", "v1")
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
		w.input("Alarm", "v2"), allow)
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
		w.input("A", "v1"), allow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.manager.Update(ctx, identity.Proof{}, created.ID, 1,
		w.input("A", "v2"), allow); err != nil {
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
	input := w.input("A", "v1")
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

// TestDeleteWaitsForAConcurrentReferenceAndConflicts proves that deleting a layout while a
// Subscription that references it is being saved waits for that save and answers a revision
// conflict instead of failing on the foreign key.
func TestDeleteWaitsForAConcurrentReferenceAndConflicts(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	created, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(),
		w.input("A", "v1"), allow)
	if err != nil {
		t.Fatal(err)
	}
	inserted, release, saved := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		saved <- w.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			executor, err := w.store.Resolve(tx)
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO subscriptions (id, revision, kind, subscriber_id,
				created_by, state, event_types, layout_id, split_by_unit, created_at, updated_at)
				VALUES ($1, 1, 'global', $2, $2, 'active', ARRAY['message.created'], $3, false, now(), now())`,
				uuid.New(), uuid.New(), created.ID); err != nil {
				return err
			}
			close(inserted)
			<-release
			return nil
		})
	}()
	<-inserted
	deleted := make(chan error, 1)
	go func() {
		_, err := w.manager.Delete(ctx, identity.Proof{}, created.ID, 1, allow)
		deleted <- err
	}()
	select {
	case err := <-deleted:
		t.Fatalf("delete did not wait for the referencing save: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("delete during a referencing save: %v", err)
	}
}

// TestAgencyBinding proves that a layout keeps its Agencies, that an unknown Agency is invalid
// input, and that visibility follows the Agency scopes of the grants.
func TestAgencyBinding(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	other := seedAgency(t, w.store)
	created, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("A", "v1"), allow)
	if err != nil || len(created.Agencies) != 1 || created.Agencies[0] != w.agency {
		t.Fatalf("create = %+v, %v", created, err)
	}
	unknown := w.input("B", "v1")
	unknown.Agencies = []uuid.UUID{uuid.New()}
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), unknown, allow); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("unknown Agency on create: %v", err)
	}
	if _, err := w.manager.Update(ctx, identity.Proof{}, created.ID, 1, unknown, allow); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("unknown Agency on update: %v", err)
	}
	moved := w.input("A", "v2")
	moved.Agencies = []uuid.UUID{other}
	updated, err := w.manager.Update(ctx, identity.Proof{}, created.ID, 1, moved, allow)
	if err != nil || len(updated.Agencies) != 1 || updated.Agencies[0] != other {
		t.Fatalf("update = %+v, %v", updated, err)
	}

	scoped := func(kind string, references ...uuid.UUID) *presentation.Service {
		refs := make([]string, len(references))
		for i, id := range references {
			refs[i] = id.String()
		}
		return serviceWithScope(t, w.store, w.layouts, presentation.ReadAction, authz.Scope{Kind: kind, References: refs})
	}
	for name, test := range map[string]struct {
		service *presentation.Service
		visible bool
	}{
		"agency of the layout":  {scoped("agency", other), true},
		"other agency":          {scoped("agency", w.agency), false},
		"unit of the Agency":    {scoped("unit", seedUnit(t, w.store, other, nil)), true},
		"unit of other Agency":  {scoped("unit", seedUnit(t, w.store, w.agency, nil)), false},
		"collection of Agency":  {scoped("unitCollection", seedCollection(t, w.store, seedUnit(t, w.store, other, nil))), true},
		"collection elsewhere":  {scoped("unitCollection", seedCollection(t, w.store, seedUnit(t, w.store, w.agency, nil))), false},
		"all":                   {w.reader, true},
		"deleted unit is blind": {scoped("unit", seedUnit(t, w.store, other, ptr(true))), false},
	} {
		page, err := test.service.List(ctx, identity.Proof{}, units.PageInput{})
		if err != nil || (page.Total == 1) != test.visible || len(page.Items) != page.Total {
			t.Errorf("%s: list = %+v, %v, want visible=%v", name, page, err, test.visible)
		}
		_, err = test.service.Read(ctx, identity.Proof{}, created.ID)
		if test.visible != (err == nil) || (err != nil && !errors.Is(err, authz.ErrNotFound)) {
			t.Errorf("%s: read: %v, want visible=%v", name, err, test.visible)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// seedUnit inserts a Unit of the Agency, deleted when deleted points to true.
func seedUnit(t *testing.T, store *postgres.Store, agency uuid.UUID, deleted *bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO units (id, name, agency_id, deleted) VALUES ($1, 'Unit', $2, $3)`,
			id, agency, deleted != nil && *deleted)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// seedCollection inserts a Unit collection with the Unit as its member.
func seedCollection(t *testing.T, store *postgres.Store, unit uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err = executor.ExecContext(tx.Context(), `INSERT INTO unit_collections (id, name) VALUES ($1, 'Collection')`, id); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO unit_collection_memberships (unit_id, collection_id) VALUES ($1, $2)`, unit, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// LockCatalog implements FieldRegistry; the fake has no catalog.
func (builtinFields) LockCatalog(context.Context, platform.TransactionContext) error { return nil }
