package attributestore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/attributestore"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
	"github.com/gopager/gopager/internal/units"
)

func TestMain(m *testing.M) { os.Exit(testpostgres.Run(m)) }

var migratedTemplate = testpostgres.NewTemplate("attributestore", func(ctx context.Context, dsn string) error {
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

type world struct {
	store   *postgres.Store
	schemas *attributestore.Store
	agency  uuid.UUID
	manager *attributes.Service
}

func (w *world) service(t *testing.T, scope authz.Scope, actions ...string) *attributes.Service {
	t.Helper()
	receipts, err := commandstore.New(w.store)
	if err != nil {
		t.Fatal(err)
	}
	principal := uuid.NewString()
	authority := authz.Authority{PrincipalID: principal, Revision: 1, Active: true, Grants: []authz.Grant{{
		ID: uuid.NewString(), SubjectID: principal, SubjectKind: "principal", Actions: actions, Scope: scope}}}
	facts, err := eventstore.New(w.store)
	if err != nil {
		t.Fatal(err)
	}
	service, err := attributes.NewService(w.store, staticResolver{identity.Authentication{Authority: authority,
		Kind: identity.SessionProof}}, fixedClock{}, w.schemas, receipts, facts)
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
	schemas, err := attributestore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{store: store, schemas: schemas}
	w.agency = w.exec(t, func(id uuid.UUID) (string, []any) {
		return `INSERT INTO agencies (id, name) VALUES ($1, 'Agency')`, []any{id}
	})
	w.manager = w.service(t, authz.Scope{Kind: "all"}, attributes.ManageAction)
	return w
}

// exec runs one statement built for a fresh ID in its own transaction and returns the ID.
func (w *world) exec(t *testing.T, build func(id uuid.UUID) (string, []any)) uuid.UUID {
	t.Helper()
	id := uuid.New()
	query, args := build(id)
	w.run(t, query, args...)
	return id
}

func (w *world) run(t *testing.T, query string, args ...any) {
	t.Helper()
	err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), query, args...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (w *world) input(key string, agencies ...uuid.UUID) attributes.SchemaInput {
	if len(agencies) == 0 {
		agencies = []uuid.UUID{w.agency}
	}
	return attributes.SchemaInput{Key: key, Label: "Label", Subject: attributes.SubjectMessage, Agencies: agencies,
		Fields: map[string]attributes.Field{"code": {Kind: processing.StringValue}}}
}

func allow(identity.Authentication) error { return nil }

func (w *world) catalogRevision(t *testing.T) int64 {
	t.Helper()
	var revision int64
	err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		_, r, err := w.schemas.Catalog(tx.Context(), tx)
		revision = r
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestSchemaLifecycleAndCatalogRevision(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	if w.catalogRevision(t) != 1 {
		t.Fatal("initial catalog revision is not 1")
	}
	command := uuid.New()
	created, err := w.manager.Create(ctx, identity.Proof{}, command, w.input("alarm"), allow)
	if err != nil || created.Revision != 1 || len(created.Agencies) != 1 {
		t.Fatalf("create = %+v, %v", created, err)
	}
	replay, err := w.manager.Create(ctx, identity.Proof{}, command, w.input("alarm"), allow)
	if err != nil || replay.Key != "alarm" {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	if _, err := w.manager.Create(ctx, identity.Proof{}, command, w.input("other"), allow); !errors.Is(err, commands.ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay: %v", err)
	}
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("alarm"), allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("duplicate key: %v", err)
	}
	if got := w.catalogRevision(t); got != 2 {
		t.Fatalf("catalog revision after create = %d", got)
	}

	next := w.input("alarm")
	next.Label = "Renamed"
	next.Fields = map[string]attributes.Field{"code": {Kind: processing.StringValue}, "extra": {Kind: processing.StringValue}}
	updated, err := w.manager.Update(ctx, identity.Proof{}, "alarm", 1, next, allow)
	if err != nil || updated.Revision != 2 || updated.Label != "Renamed" {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if _, err := w.manager.Update(ctx, identity.Proof{}, "alarm", 1, next, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	removing := w.input("alarm")
	if _, err := w.manager.Update(ctx, identity.Proof{}, "alarm", 2, removing, allow); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("incompatible update: %v", err)
	}
	if got := w.catalogRevision(t); got != 3 {
		t.Fatalf("catalog revision after update = %d (failed changes must not count)", got)
	}
	read, err := w.manager.Read(ctx, identity.Proof{}, "alarm")
	if err != nil || read.Revision != 2 || !read.UpdatedAt.Equal(updated.UpdatedAt) || len(read.Fields) != 2 {
		t.Fatalf("read = %+v, %v", read, err)
	}

	if _, err := w.manager.Delete(ctx, identity.Proof{}, "alarm", 1, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	result, err := w.manager.Delete(ctx, identity.Proof{}, "alarm", 2, allow)
	if err != nil || result.Key != "alarm" || result.Revision != 3 {
		t.Fatalf("delete = %+v, %v", result, err)
	}
	if got := w.catalogRevision(t); got != 4 {
		t.Fatalf("catalog revision after delete = %d", got)
	}
	if _, err := w.manager.Read(ctx, identity.Proof{}, "alarm"); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("read deleted: %v", err)
	}
}

func TestUnknownAgencyIsInvalidInput(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("alarm", uuid.New()), allow); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("create: %v", err)
	}
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("alarm"), allow); err != nil {
		t.Fatal(err)
	}
	if _, err := w.manager.Update(ctx, identity.Proof{}, "alarm", 1, w.input("alarm", uuid.New()), allow); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("update: %v", err)
	}
}

// TestDeleteRefusesReferencedSchema proves that each kind of reference blocks the delete and
// that an unreferenced schema is deleted.
func TestDeleteRefusesReferencedSchema(t *testing.T) {
	t.Parallel()
	reference := `{"namespace":"attributes","path":["%s","code"]}`
	unitReference := `{"namespace":"unit","path":["attributes","%s","code"]}`
	for name, insert := range map[string]func(w *world, key string){
		"active Workflow version": func(w *world, key string) {
			workflow, version := uuid.New(), uuid.New()
			w.run(t, `INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', now(), now())`, workflow)
			w.run(t, `INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by) VALUES ($1, $2, $3::jsonb, now(), $4)`,
				version, workflow, `{"outputs":{"`+key+`":{}}}`, uuid.New())
			w.run(t, `UPDATE processing_workflows SET active_version_id = $2 WHERE id = $1`, workflow, version)
		},
		"Layout version": func(w *world, key string) {
			layout := uuid.New()
			w.run(t, `INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at) VALUES ($1, 1, 'l', 'text', $2, now(), now())`, layout, uuid.New())
			w.run(t, `INSERT INTO layout_definition_versions (layout_id, version, definition, created_by, created_at) VALUES ($1, 1, $2::jsonb, $3, now())`,
				layout, `{"blocks":[{"kind":"field","field":`+fmt.Sprintf(reference, key)+`}]}`, uuid.New())
		},
		"Subscription filter": func(w *world, key string) {
			w.run(t, `INSERT INTO subscriptions (id, revision, kind, subscriber_id, created_by, state, event_types, filter, split_by_unit, created_at, updated_at)
				VALUES ($1, 1, 'global', $2, $2, 'active', ARRAY['message.created'], $3::jsonb, false, now(), now())`,
				uuid.New(), uuid.New(), `{"all":[{"field":`+fmt.Sprintf(reference, key)+`}]}`)
		},
		"Subscription target input binding": func(w *world, key string) {
			instance, version, subscription := uuid.New(), uuid.New(), uuid.New()
			err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
				executor, err := w.store.Resolve(tx)
				if err != nil {
					return err
				}
				for _, statement := range []struct {
					query string
					args  []any
				}{
					{`INSERT INTO processing_instances (id, revision) VALUES ($1, 1)`, []any{instance}},
					{`INSERT INTO processing_instance_versions (id, instance_id, revision, configuration) VALUES ($1, $2, 1, jsonb_build_object('id', $3::text, 'revision', '1'))`, []any{version, instance, instance.String()}},
					{`INSERT INTO subscriptions (id, revision, kind, subscriber_id, created_by, state, event_types, split_by_unit, created_at, updated_at)
						VALUES ($1, 1, 'global', $2, $2, 'active', ARRAY['message.created'], false, now(), now())`, []any{subscription, uuid.New()}},
					{`INSERT INTO subscription_targets (subscription_id, position, instance_id, capability_kind, instance_revision, inputs, input_bindings)
						VALUES ($1, 0, $2, 'delivery', 1, '{}', $3::jsonb)`,
						[]any{subscription, instance, `{"recipient":{"namespace":"unit","path":["attributes","` + key + `","code"]}}`}},
				} {
					if _, err := executor.ExecContext(tx.Context(), statement.query, statement.args...); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		},
		"saved view": func(w *world, key string) {
			w.run(t, `INSERT INTO saved_views (id, ownership, name, definition, created_at, updated_at) VALUES ($1, 'global', 'v', $2::jsonb, now(), now())`,
				uuid.New(), `{"filters":[{"field":`+fmt.Sprintf(reference, key)+`}]}`)
		},
		"Workflow reading a target Unit attribute": func(w *world, key string) {
			workflow, version := uuid.New(), uuid.New()
			w.run(t, `INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', now(), now())`, workflow)
			w.run(t, `INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by) VALUES ($1, $2, $3::jsonb, now(), $4)`,
				version, workflow, `{"steps":[{"input":{"namespace":"target","path":["unit","attributes","`+key+`","code"]}}]}`, uuid.New())
			w.run(t, `UPDATE processing_workflows SET active_version_id = $2 WHERE id = $1`, workflow, version)
		},
		"Layout reading a Unit attribute": func(w *world, key string) {
			layout := uuid.New()
			w.run(t, `INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at) VALUES ($1, 1, 'l', 'text', $2, now(), now())`, layout, uuid.New())
			w.run(t, `INSERT INTO layout_definition_versions (layout_id, version, definition, created_by, created_at) VALUES ($1, 1, $2::jsonb, $3, now())`,
				layout, `{"blocks":[{"kind":"field","field":`+fmt.Sprintf(unitReference, key)+`}]}`, uuid.New())
		},
		"Subscription filter on a Unit attribute": func(w *world, key string) {
			w.run(t, `INSERT INTO subscriptions (id, revision, kind, subscriber_id, created_by, state, event_types, filter, split_by_unit, created_at, updated_at)
				VALUES ($1, 1, 'global', $2, $2, 'active', ARRAY['message.created'], $3::jsonb, false, now(), now())`,
				uuid.New(), uuid.New(), `{"all":[{"field":`+fmt.Sprintf(unitReference, key)+`}]}`)
		},
		"saved view on a Unit attribute": func(w *world, key string) {
			w.run(t, `INSERT INTO saved_views (id, ownership, name, definition, created_at, updated_at) VALUES ($1, 'global', 'v', $2::jsonb, now(), now())`,
				uuid.New(), `{"filters":[{"field":`+fmt.Sprintf(unitReference, key)+`}]}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			ctx := context.Background()
			if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("alarm"), allow); err != nil {
				t.Fatal(err)
			}
			if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("other"), allow); err != nil {
				t.Fatal(err)
			}
			insert(w, "alarm")
			if _, err := w.manager.Delete(ctx, identity.Proof{}, "alarm", 1, allow); !errors.Is(err, authz.ErrRevisionConflict) {
				t.Fatalf("referenced delete: %v", err)
			}
			if _, err := w.manager.Delete(ctx, identity.Proof{}, "other", 1, allow); err != nil {
				t.Fatalf("unreferenced delete: %v", err)
			}
		})
	}
}

// TestVisibilityFollowsScopeKinds proves which rights show a schema and how their scopes map to
// the schema's Agencies.
func TestVisibilityFollowsScopeKinds(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	other := w.exec(t, func(id uuid.UUID) (string, []any) {
		return `INSERT INTO agencies (id, name) VALUES ($1, 'Other')`, []any{id}
	})
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("alarm"), allow); err != nil {
		t.Fatal(err)
	}
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("elsewhere", other), allow); err != nil {
		t.Fatal(err)
	}
	unit := func(agency uuid.UUID, deleted bool) uuid.UUID {
		return w.exec(t, func(id uuid.UUID) (string, []any) {
			return `INSERT INTO units (id, name, agency_id, deleted) VALUES ($1, 'U', $2, $3)`, []any{id, agency, deleted}
		})
	}
	collection := func(member uuid.UUID) uuid.UUID {
		id := w.exec(t, func(id uuid.UUID) (string, []any) {
			return `INSERT INTO unit_collections (id, name) VALUES ($1, 'C')`, []any{id}
		})
		w.run(t, `INSERT INTO unit_collection_memberships (unit_id, collection_id) VALUES ($1, $2)`, member, id)
		return id
	}
	ref := func(kind string, id uuid.UUID) authz.Scope {
		return authz.Scope{Kind: kind, References: []string{id.String()}}
	}
	for name, test := range map[string]struct {
		scope   authz.Scope
		action  string
		visible []string
	}{
		"all":                   {authz.Scope{Kind: "all"}, "message.read", []string{"alarm", "elsewhere"}},
		"agency":                {ref("agency", w.agency), "workflow.read", []string{"alarm"}},
		"unit":                  {ref("unit", unit(other, false)), "unit.update", []string{"elsewhere"}},
		"deleted unit":          {ref("unit", unit(other, true)), "unit.update", nil},
		"unit collection":       {ref("unitCollection", collection(unit(w.agency, false))), "message.read", []string{"alarm"}},
		"account sees all":      {ref("account", uuid.New()), "subscription.manage-own", []string{"alarm", "elsewhere"}},
		"manage in one agency":  {ref("agency", other), attributes.ManageAction, []string{"elsewhere"}},
		"presentation (agency)": {ref("agency", w.agency), "presentation.manage", []string{"alarm"}},
	} {
		service := w.service(t, test.scope, test.action)
		page, err := service.List(ctx, identity.Proof{}, nil, units.PageInput{})
		if test.visible == nil {
			if !errors.Is(err, authz.ErrForbidden) && (err != nil || page.Total != 0) {
				t.Errorf("%s: list = %+v, %v", name, page, err)
			}
			continue
		}
		if err != nil || page.Total != len(test.visible) || len(page.Items) != len(test.visible) {
			t.Errorf("%s: list = %+v, %v, want %v", name, page, err, test.visible)
			continue
		}
		for i, key := range test.visible {
			if page.Items[i].Key != key {
				t.Errorf("%s: item %d = %s, want %s", name, i, page.Items[i].Key, key)
			}
		}
	}
	scoped := w.service(t, ref("agency", w.agency), "workflow.read")
	if _, err := scoped.Read(ctx, identity.Proof{}, "elsewhere"); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("invisible read: %v", err)
	}
	if _, err := scoped.Delete(ctx, identity.Proof{}, "elsewhere", 1, allow); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("invisible delete: %v", err)
	}
	if _, err := scoped.Delete(ctx, identity.Proof{}, "alarm", 1, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("delete without manage: %v", err)
	}
	filtered, err := w.manager.List(ctx, identity.Proof{}, &other, units.PageInput{})
	if err != nil || filtered.Total != 1 || filtered.Items[0].Key != "elsewhere" {
		t.Fatalf("agency filter = %+v, %v", filtered, err)
	}
}

func TestCatalogReturnsEverySchema(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	for _, key := range []string{"b-schema", "a-schema"} {
		if _, err := w.manager.Create(context.Background(), identity.Proof{}, uuid.New(), w.input(key), allow); err != nil {
			t.Fatal(err)
		}
	}
	err := w.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		schemas, revision, err := w.schemas.Catalog(tx.Context(), tx)
		if err != nil || revision != 3 || len(schemas) != 2 || schemas[0].Key != "a-schema" {
			t.Fatalf("catalog = %+v, %d, %v", schemas, revision, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
