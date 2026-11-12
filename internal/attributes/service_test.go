package attributes

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/units"
)

type fakeTx struct{ ctx context.Context }

func (t fakeTx) Context() context.Context { return t.ctx }

type fakeTransactions struct{}

func (fakeTransactions) WithinTransaction(ctx context.Context, fn platform.TxFunc) error {
	return fn(fakeTx{ctx})
}

type fakeClock struct{}

func (fakeClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC), nil
}

type fakeResolver struct{ authentication identity.Authentication }

func (r fakeResolver) Resolve(context.Context, platform.TransactionContext, identity.Proof) (identity.Authentication, error) {
	return r.authentication, nil
}

type fakeReceipts struct {
	stored map[[2]uuid.UUID]commands.Receipt
}

func (r *fakeReceipts) ReadCommandReceipt(_ context.Context, _ platform.TransactionContext, actor, command uuid.UUID) (commands.Receipt, bool, error) {
	receipt, ok := r.stored[[2]uuid.UUID{actor, command}]
	return receipt, ok, nil
}

func (r *fakeReceipts) SaveCommandReceipt(_ context.Context, _ platform.TransactionContext, receipt commands.Receipt) error {
	r.stored[[2]uuid.UUID{receipt.ActorID, receipt.CommandID}] = receipt
	return nil
}

// memoryStore keeps schemas in memory; visibility is matched on Agency scopes only.
type memoryStore struct {
	schemas map[string]Schema
	facts   []events.MasterDataFact
}

// AppendMasterDataFact records the fact so tests can inspect what a service appended.
func (m *memoryStore) AppendMasterDataFact(_ context.Context, _ platform.TransactionContext, fact events.MasterDataFact) (uuid.UUID, error) {
	m.facts = append(m.facts, fact)
	return fact.ID, nil
}

func matches(filter authz.AgencyFilter, agencies []uuid.UUID) bool {
	return filter.All || slices.ContainsFunc(agencies, func(a uuid.UUID) bool { return slices.Contains(filter.Agencies, a) })
}

func (m *memoryStore) Insert(_ context.Context, _ platform.TransactionContext, schema Schema, _ uuid.UUID) error {
	m.schemas[schema.Key] = schema
	return nil
}

func (m *memoryStore) Get(_ context.Context, _ platform.TransactionContext, key string) (Schema, error) {
	schema, ok := m.schemas[key]
	if !ok {
		return Schema{}, authz.ErrNotFound
	}
	return schema, nil
}

func (m *memoryStore) Visible(_ context.Context, _ platform.TransactionContext, key string, v authz.AgencyVisibility) (bool, error) {
	schema, ok := m.schemas[key]
	return ok && slices.ContainsFunc(v.Terms, func(term authz.AgencyTerm) bool {
		return matches(term.Grant, schema.Agencies) && (term.Key == nil || matches(*term.Key, schema.Agencies))
	}), nil
}

func (m *memoryStore) List(ctx context.Context, tx platform.TransactionContext, v authz.AgencyVisibility, _ *uuid.UUID, _, _ int) ([]Schema, int, error) {
	var out []Schema
	for key, schema := range m.schemas {
		if visible, _ := m.Visible(ctx, tx, key, v); visible {
			out = append(out, schema)
		}
	}
	return out, len(out), nil
}

func (m *memoryStore) Update(_ context.Context, _ platform.TransactionContext, schema Schema, expected int64) error {
	if m.schemas[schema.Key].Revision != expected {
		return authz.ErrRevisionConflict
	}
	m.schemas[schema.Key] = schema
	return nil
}

func (m *memoryStore) Delete(_ context.Context, _ platform.TransactionContext, key string, _ int64) error {
	delete(m.schemas, key)
	return nil
}

var agencyA, agencyB = uuid.New(), uuid.New()

func serviceFor(t *testing.T, scope authz.Scope, actions ...string) (*Service, *memoryStore) {
	t.Helper()
	store := &memoryStore{schemas: map[string]Schema{}}
	principal := uuid.NewString()
	authority := authz.Authority{PrincipalID: principal, Revision: 1, Active: true, Grants: []authz.Grant{{
		ID: uuid.NewString(), SubjectID: principal, SubjectKind: "principal", Actions: actions, Scope: scope}}}
	service, err := NewService(fakeTransactions{}, fakeResolver{identity.Authentication{Authority: authority, Kind: identity.SessionProof}},
		fakeClock{}, store, &fakeReceipts{stored: map[[2]uuid.UUID]commands.Receipt{}}, store)
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func input(key string, agencies ...uuid.UUID) SchemaInput {
	return SchemaInput{Key: key, Label: "L", Subject: SubjectMessage, Agencies: agencies,
		Fields: map[string]Field{"code": {Kind: processing.StringValue}}}
}

func allow(identity.Authentication) error { return nil }

func agencyScope(ids ...uuid.UUID) authz.Scope {
	scope := authz.Scope{Kind: "agency"}
	for _, id := range ids {
		scope.References = append(scope.References, id.String())
	}
	return scope
}

func TestManageNeedsEveryAgency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service, store := serviceFor(t, agencyScope(agencyA), ManageAction)
	if _, err := service.Create(ctx, identity.Proof{}, uuid.New(), input("alarm", agencyA, agencyB), allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("create in a foreign Agency: %v", err)
	}
	created, err := service.Create(ctx, identity.Proof{}, uuid.New(), input("alarm", agencyA), allow)
	if err != nil || created.Revision != 1 {
		t.Fatalf("create = %+v, %v", created, err)
	}
	if _, err := service.Update(ctx, identity.Proof{}, "alarm", 1, input("alarm", agencyA, agencyB), allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("update adding a foreign Agency: %v", err)
	}
	shared := store.schemas["alarm"]
	shared.Agencies = []uuid.UUID{agencyA, agencyB}
	store.schemas["alarm"] = shared
	if _, err := service.Update(ctx, identity.Proof{}, "alarm", 1, input("alarm", agencyA), allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("update of a shared schema: %v", err)
	}
	if _, err := service.Delete(ctx, identity.Proof{}, "alarm", 1, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("delete of a shared schema: %v", err)
	}
}

func TestInvisibleSchemaIsNotFoundAndReadersCannotWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reader, store := serviceFor(t, agencyScope(agencyA), "workflow.read")
	store.schemas["mine"] = Schema{Key: "mine", Revision: 1, Agencies: []uuid.UUID{agencyA}}
	store.schemas["theirs"] = Schema{Key: "theirs", Revision: 1, Agencies: []uuid.UUID{agencyB}}
	if _, err := reader.Read(ctx, identity.Proof{}, "mine"); err != nil {
		t.Fatalf("visible read: %v", err)
	}
	if _, err := reader.Read(ctx, identity.Proof{}, "theirs"); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("invisible read: %v", err)
	}
	if _, err := reader.Delete(ctx, identity.Proof{}, "theirs", 1, allow); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("invisible delete: %v", err)
	}
	if _, err := reader.Delete(ctx, identity.Proof{}, "mine", 1, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("reader delete: %v", err)
	}
	if page, err := reader.List(ctx, identity.Proof{}, nil, units.PageInput{}); err != nil || page.Total != 1 {
		t.Fatalf("list = %+v, %v", page, err)
	}
	stranger, _ := serviceFor(t, authz.Scope{Kind: "all"}, "audit.read")
	if _, err := stranger.List(ctx, identity.Proof{}, nil, units.PageInput{}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("unrelated right: %v", err)
	}
}

func TestInputIsValidatedBeforeAnything(t *testing.T) {
	t.Parallel()
	service, _ := serviceFor(t, authz.Scope{Kind: "all"}, ManageAction)
	for name, in := range map[string]SchemaInput{
		"bad key": input("Bad Key", agencyA), "no agency": input("alarm"), "duplicate": input("alarm", agencyA, agencyA),
	} {
		if _, err := service.Create(context.Background(), identity.Proof{}, uuid.New(), in, allow); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := service.Create(context.Background(), identity.Proof{}, uuid.Nil, input("alarm", agencyA), allow); !errors.Is(err, authz.ErrInvalidInput) {
		t.Errorf("nil command: %v", err)
	}
}
