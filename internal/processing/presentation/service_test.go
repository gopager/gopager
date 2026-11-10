package presentation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
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

// fakeResolver authenticates every proof as the configured authentication.
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

// memoryStore keeps layouts in memory.
type memoryStore struct {
	records  map[uuid.UUID]Record
	versions map[uuid.UUID][]VersionRecord
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: map[uuid.UUID]Record{}, versions: map[uuid.UUID][]VersionRecord{}}
}

func (m *memoryStore) Insert(_ context.Context, _ platform.TransactionContext, record Record) error {
	m.records[record.ID] = record
	m.versions[record.ID] = []VersionRecord{{LayoutID: record.ID, Version: 1, Definition: record.Definition}}
	return nil
}

func (m *memoryStore) Get(_ context.Context, _ platform.TransactionContext, id uuid.UUID) (Record, error) {
	record, ok := m.records[id]
	if !ok {
		return Record{}, authz.ErrNotFound
	}
	return record, nil
}

func (m *memoryStore) List(context.Context, platform.TransactionContext, int, int) ([]Record, int, error) {
	var out []Record
	for _, record := range m.records {
		out = append(out, record)
	}
	return out, len(out), nil
}

func (m *memoryStore) Version(_ context.Context, _ platform.TransactionContext, id uuid.UUID, version int64) (VersionRecord, error) {
	for _, v := range m.versions[id] {
		if v.Version == version {
			return v, nil
		}
	}
	return VersionRecord{}, authz.ErrNotFound
}

func (m *memoryStore) Update(_ context.Context, _ platform.TransactionContext, id uuid.UUID, expected int64, name string,
	definition []byte, _ uuid.UUID, now time.Time) error {
	record := m.records[id]
	if record.Revision != expected {
		return authz.ErrRevisionConflict
	}
	record.Name, record.Definition, record.Revision, record.UpdatedAt = name, definition, expected+1, now
	m.records[id] = record
	m.versions[id] = append(m.versions[id], VersionRecord{LayoutID: id, Version: expected + 1, Definition: definition})
	return nil
}

func (m *memoryStore) Delete(_ context.Context, _ platform.TransactionContext, id uuid.UUID, expected int64) error {
	if m.records[id].Revision != expected {
		return authz.ErrRevisionConflict
	}
	delete(m.records, id)
	return nil
}

type failingFields struct{}

func (failingFields) FieldSchema(processing.FieldRef) (processing.FieldSchema, error) {
	return processing.FieldSchema{}, errors.New("unknown field")
}

func textLayout(kind string) processing.Layout {
	return processing.Layout{SchemaVersion: 1, OutputKind: kind,
		Blocks: []processing.LayoutBlock{{Kind: "text", Text: "hello"}}}
}

func serviceFor(t *testing.T, kind identity.ProofKind, actions ...string) (*Service, *memoryStore) {
	t.Helper()
	store := newMemoryStore()
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true}
	if len(actions) > 0 {
		authority.Grants = []authz.Grant{{ID: uuid.NewString(), SubjectID: authority.PrincipalID, SubjectKind: "principal",
			Actions: actions, Scope: authz.Scope{Kind: "all"}}}
	}
	service, err := NewService(fakeTransactions{}, fakeResolver{identity.Authentication{Authority: authority, Kind: kind}},
		fakeClock{}, store, &fakeReceipts{stored: map[[2]uuid.UUID]commands.Receipt{}}, failingFields{})
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func allow(identity.Authentication) error { return nil }

func TestRightsMatrix(t *testing.T) {
	t.Parallel()
	input := LayoutInput{Name: "n", Definition: textLayout("text")}
	for _, test := range []struct {
		name    string
		kind    identity.ProofKind
		actions []string
		read    bool
		mutate  error
	}{
		{"manager", identity.SessionProof, []string{ManageAction}, true, nil},
		{"own subscriber", identity.SessionProof, []string{SubscriptionManageOwnAction}, true, authz.ErrForbidden},
		{"global subscriber", identity.SessionProof, []string{SubscriptionManageGlobalAction}, true, authz.ErrForbidden},
		{"nothing", identity.SessionProof, nil, false, authz.ErrForbidden},
		{"guest manager", identity.AnonymousProof, []string{ManageAction}, true, identity.ErrLoginRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, _ := serviceFor(t, test.kind, test.actions...)
			_, err := service.Create(context.Background(), identity.Proof{}, uuid.New(), input, allow)
			if !errors.Is(err, test.mutate) && (test.mutate != nil || err != nil) {
				t.Fatalf("create: %v, want %v", err, test.mutate)
			}
			_, err = service.List(context.Background(), identity.Proof{}, units.PageInput{})
			if (err == nil) != test.read || (err != nil && !errors.Is(err, authz.ErrForbidden)) {
				t.Fatalf("list: %v, want readable=%v", err, test.read)
			}
			_, err = service.Read(context.Background(), identity.Proof{}, uuid.New())
			if test.read != errors.Is(err, authz.ErrNotFound) {
				t.Fatalf("read: %v, want readable=%v", err, test.read)
			}
		})
	}
}

func TestCreateValidatesNameAndStructure(t *testing.T) {
	t.Parallel()
	service, _ := serviceFor(t, identity.SessionProof, ManageAction)
	for name, input := range map[string]LayoutInput{
		"empty name": {Name: "", Definition: textLayout("text")},
		"blank name": {Name: " \t", Definition: textLayout("text")},
		"long name":  {Name: strings.Repeat("a", 257), Definition: textLayout("text")},
		"bad utf8":   {Name: "\xff", Definition: textLayout("text")},
		"bad kind":   {Name: "n", Definition: textLayout("html")},
		"no blocks":  {Name: "n", Definition: processing.Layout{SchemaVersion: 1, OutputKind: "text"}},
		"unknown field": {Name: "n", Definition: processing.Layout{SchemaVersion: 1, OutputKind: "text", Blocks: []processing.LayoutBlock{
			{Kind: "field", Field: &processing.FieldRef{Namespace: "x", Path: []string{"y"}}}}}},
	} {
		_, err := service.Create(context.Background(), identity.Proof{}, uuid.New(), input, allow)
		var failure *platform.Failure
		if !errors.As(err, &failure) && !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := service.Create(context.Background(), identity.Proof{}, uuid.New(),
		LayoutInput{Name: strings.Repeat("a", 256), Definition: textLayout("text")}, allow); err != nil {
		t.Fatalf("256-byte name: %v", err)
	}
}

func TestOutputKindIsFixedAtCreation(t *testing.T) {
	t.Parallel()
	service, _ := serviceFor(t, identity.SessionProof, ManageAction)
	created, err := service.Create(context.Background(), identity.Proof{}, uuid.New(),
		LayoutInput{Name: "n", Definition: textLayout("text")}, allow)
	if err != nil || created.OutputKind != "text" || created.Revision != 1 {
		t.Fatalf("create = %+v, %v", created, err)
	}
	_, err = service.Update(context.Background(), identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "n", Definition: textLayout("markdown")}, allow)
	if !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("kind change: %v", err)
	}
	updated, err := service.Update(context.Background(), identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "renamed", Definition: textLayout("text")}, allow)
	if err != nil || updated.Revision != 2 || updated.Name != "renamed" {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if _, err := service.Update(context.Background(), identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "x", Definition: textLayout("text")}, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale update: %v", err)
	}
}

func TestResolver(t *testing.T) {
	t.Parallel()
	service, store := serviceFor(t, identity.SessionProof, ManageAction)
	created, err := service.Create(context.Background(), identity.Proof{}, uuid.New(),
		LayoutInput{Name: "n", Definition: textLayout("markdown")}, allow)
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]processing.Layout{"text": textLayout("text")}
	resolver, err := NewResolver(store, defaults)
	if err != nil {
		t.Fatal(err)
	}
	tx := fakeTx{context.Background()}
	resolved, err := resolver.ResolveLayout(context.Background(), tx, processing.LayoutRequest{OutputKind: "text"})
	if err != nil || resolved.ID != uuid.Nil || resolved.Version != 0 {
		t.Fatalf("default = %+v, %v", resolved, err)
	}
	if _, err := resolver.ResolveLayout(context.Background(), tx, processing.LayoutRequest{OutputKind: "json"}); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("unknown default: %v", err)
	}
	resolved, err = resolver.ResolveLayout(context.Background(), tx, processing.LayoutRequest{LayoutID: &created.ID})
	if err != nil || resolved.ID != created.ID || resolved.Version != 1 {
		t.Fatalf("stored = %+v, %v", resolved, err)
	}
	if _, err := resolver.ResolveLayout(context.Background(), tx, processing.LayoutRequest{LayoutID: &created.ID, OutputKind: "text"}); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("kind mismatch: %v", err)
	}
	missing := uuid.New()
	if _, err := resolver.ResolveLayout(context.Background(), tx, processing.LayoutRequest{LayoutID: &missing}); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
