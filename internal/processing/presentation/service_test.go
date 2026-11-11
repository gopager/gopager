package presentation

import (
	"context"
	"errors"
	"slices"
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

// matches reports whether the filter selects one of the Agencies; Unit-based references are
// out of reach of the memory store, so tests use Agency scopes.
func matches(filter authz.AgencyFilter, agencies []uuid.UUID) bool {
	return filter.All || slices.ContainsFunc(agencies, func(a uuid.UUID) bool { return slices.Contains(filter.Agencies, a) })
}

func (m *memoryStore) Visible(_ context.Context, _ platform.TransactionContext, id uuid.UUID, visibility authz.AgencyVisibility) (bool, error) {
	record, ok := m.records[id]
	if !ok {
		return false, nil
	}
	return slices.ContainsFunc(visibility.Terms, func(term authz.AgencyTerm) bool {
		return matches(term.Grant, record.Agencies) && (term.Key == nil || matches(*term.Key, record.Agencies))
	}), nil
}

func (m *memoryStore) List(ctx context.Context, tx platform.TransactionContext, visibility authz.AgencyVisibility, _, _ int) ([]Record, int, error) {
	var out []Record
	for id, record := range m.records {
		if visible, _ := m.Visible(ctx, tx, id, visibility); visible {
			out = append(out, record)
		}
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
	agencies []uuid.UUID, definition []byte, _ uuid.UUID, now time.Time) error {
	record := m.records[id]
	if record.Revision != expected {
		return authz.ErrRevisionConflict
	}
	record.Name, record.Agencies, record.Definition, record.Revision, record.UpdatedAt = name, agencies, definition, expected+1, now
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

// testAgency is the Agency the fixtures put layouts in.
var testAgency = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")

// agencies returns the Agency list of a valid input.
func agencies() []uuid.UUID { return []uuid.UUID{testAgency} }

func serviceFor(t *testing.T, kind identity.ProofKind, actions ...string) (*Service, *memoryStore) {
	t.Helper()
	return serviceWithScope(t, kind, authz.Scope{Kind: "all"}, actions...)
}

// serviceWithScope grants the actions within scope.
func serviceWithScope(t *testing.T, kind identity.ProofKind, scope authz.Scope, actions ...string) (*Service, *memoryStore) {
	t.Helper()
	store := newMemoryStore()
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true}
	if len(actions) > 0 {
		authority.Grants = []authz.Grant{{ID: uuid.NewString(), SubjectID: authority.PrincipalID, SubjectKind: "principal",
			Actions: actions, Scope: scope}}
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
	input := LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: agencies()}
	for _, test := range []struct {
		name    string
		kind    identity.ProofKind
		actions []string
		read    bool
		mutate  error
	}{
		{"manager", identity.SessionProof, []string{ManageAction}, true, nil},
		{"message reader", identity.SessionProof, []string{ReadAction}, true, authz.ErrForbidden},
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
		"empty name": {Name: "", Definition: textLayout("text"), Agencies: agencies()},
		"blank name": {Name: " \t", Definition: textLayout("text"), Agencies: agencies()},
		"long name":  {Name: strings.Repeat("a", 257), Definition: textLayout("text"), Agencies: agencies()},
		"bad utf8":   {Name: "\xff", Definition: textLayout("text"), Agencies: agencies()},
		"bad kind":   {Name: "n", Definition: textLayout("html")},
		"no blocks":  {Name: "n", Definition: processing.Layout{SchemaVersion: 1, OutputKind: "text"}, Agencies: agencies()},
		"unknown field": {Name: "n", Definition: processing.Layout{SchemaVersion: 1, OutputKind: "text", Blocks: []processing.LayoutBlock{
			{Kind: "field", Field: &processing.FieldRef{Namespace: "x", Path: []string{"y"}}}}}, Agencies: agencies()},
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
		LayoutInput{Name: strings.Repeat("a", 256), Definition: textLayout("text"), Agencies: agencies()}, allow); err != nil {
		t.Fatalf("256-byte name: %v", err)
	}
}

func TestOutputKindIsFixedAtCreation(t *testing.T) {
	t.Parallel()
	service, _ := serviceFor(t, identity.SessionProof, ManageAction)
	created, err := service.Create(context.Background(), identity.Proof{}, uuid.New(),
		LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: agencies()}, allow)
	if err != nil || created.OutputKind != "text" || created.Revision != 1 {
		t.Fatalf("create = %+v, %v", created, err)
	}
	_, err = service.Update(context.Background(), identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "n", Definition: textLayout("markdown"), Agencies: agencies()}, allow)
	if !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("kind change: %v", err)
	}
	updated, err := service.Update(context.Background(), identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "renamed", Definition: textLayout("text"), Agencies: agencies()}, allow)
	if err != nil || updated.Revision != 2 || updated.Name != "renamed" {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if _, err := service.Update(context.Background(), identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "x", Definition: textLayout("text"), Agencies: agencies()}, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale update: %v", err)
	}
}

func TestResolver(t *testing.T) {
	t.Parallel()
	service, store := serviceFor(t, identity.SessionProof, ManageAction)
	created, err := service.Create(context.Background(), identity.Proof{}, uuid.New(),
		LayoutInput{Name: "n", Definition: textLayout("markdown"), Agencies: agencies()}, allow)
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

func TestAgenciesAreValidated(t *testing.T) {
	t.Parallel()
	service, _ := serviceFor(t, identity.SessionProof, ManageAction)
	many := make([]uuid.UUID, maxAgencies+1)
	for i := range many {
		many[i] = uuid.New()
	}
	for name, list := range map[string][]uuid.UUID{
		"none": nil, "nil ID": {uuid.Nil}, "duplicate": {testAgency, testAgency}, "too many": many,
	} {
		_, err := service.Create(context.Background(), identity.Proof{}, uuid.New(),
			LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: list}, allow)
		if !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestAgencyBoundRights proves that mutations need presentation.manage in every Agency
// involved and that a layout of foreign Agencies is invisible.
func TestAgencyBoundRights(t *testing.T) {
	t.Parallel()
	other := uuid.New()
	ctx := context.Background()
	manager, store := serviceWithScope(t, identity.SessionProof, authz.Scope{Kind: "agency", References: []string{testAgency.String()}}, ManageAction)
	created, err := manager.Create(ctx, identity.Proof{}, uuid.New(), LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: agencies()}, allow)
	if err != nil || len(created.Agencies) != 1 {
		t.Fatalf("create = %+v, %v", created, err)
	}
	if _, err := manager.Create(ctx, identity.Proof{}, uuid.New(),
		LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: []uuid.UUID{testAgency, other}}, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("create in a foreign Agency: %v", err)
	}
	if _, err := manager.Update(ctx, identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: []uuid.UUID{testAgency, other}}, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("update adding a foreign Agency: %v", err)
	}
	// A layout that also belongs to an Agency the caller does not manage cannot be changed or deleted.
	shared := store.records[created.ID]
	shared.Agencies = []uuid.UUID{testAgency, other}
	store.records[created.ID] = shared
	if _, err := manager.Update(ctx, identity.Proof{}, created.ID, 1,
		LayoutInput{Name: "n", Definition: textLayout("text"), Agencies: agencies()}, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("update of a shared layout: %v", err)
	}
	if _, err := manager.Delete(ctx, identity.Proof{}, created.ID, 1, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("delete of a shared layout: %v", err)
	}
	foreign := Record{ID: uuid.New(), Revision: 1, Agencies: []uuid.UUID{other}}
	store.records[foreign.ID] = foreign
	if _, err := manager.Read(ctx, identity.Proof{}, foreign.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("read of a foreign layout: %v", err)
	}
	if _, err := manager.Delete(ctx, identity.Proof{}, foreign.ID, 1, allow); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("delete of a foreign layout: %v", err)
	}
	if page, err := manager.List(ctx, identity.Proof{}, units.PageInput{}); err != nil || len(page.Items) != 1 {
		t.Fatalf("list = %+v, %v", page, err)
	}
}

func TestCheckUse(t *testing.T) {
	t.Parallel()
	reader, store := serviceWithScope(t, identity.SessionProof, authz.Scope{Kind: "agency", References: []string{testAgency.String()}}, ReadAction)
	own, foreign := uuid.New(), uuid.New()
	store.records[own] = Record{ID: own, Revision: 1, Agencies: agencies()}
	store.records[foreign] = Record{ID: foreign, Revision: 1, Agencies: []uuid.UUID{uuid.New()}}
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true, Grants: []authz.Grant{{
		ID: uuid.NewString(), SubjectID: uuid.NewString(), SubjectKind: "principal", Actions: []string{ReadAction},
		Scope: authz.Scope{Kind: "agency", References: []string{testAgency.String()}}}}}
	tx, now := fakeTx{context.Background()}, time.Now()
	if err := reader.CheckUse(context.Background(), tx, authority, own, now); err != nil {
		t.Fatalf("own layout: %v", err)
	}
	for name, id := range map[string]uuid.UUID{"foreign": foreign, "missing": uuid.New()} {
		if err := reader.CheckUse(context.Background(), tx, authority, id, now); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s layout: %v", name, err)
		}
	}
	if err := reader.CheckUse(context.Background(), tx, authz.Authority{}, own, now); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("no authority: %v", err)
	}
}
