package presentation

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

type assignmentKey struct {
	kind   string
	id     uuid.UUID
	output string
}

// memoryAssignments keeps assignments, Units and Organizations in memory.
type memoryAssignments struct {
	rows  map[assignmentKey]Assignment
	units map[uuid.UUID]UnitContext
	orgs  map[uuid.UUID]bool
}

func (m *memoryAssignments) List(_ context.Context, _ platform.TransactionContext, kind string, id uuid.UUID) ([]Assignment, error) {
	var result []Assignment
	for _, row := range m.rows {
		if (kind == "" || row.ScopeKind == kind) && (id == uuid.Nil || row.ScopeID == id) {
			result = append(result, row)
		}
	}
	slices.SortFunc(result, func(a, b Assignment) int {
		if a.ScopeKind != b.ScopeKind {
			return map[bool]int{true: -1, false: 1}[a.ScopeKind < b.ScopeKind]
		}
		if c := slices.Compare(a.ScopeID[:], b.ScopeID[:]); c != 0 {
			return c
		}
		return map[bool]int{true: -1, false: 1}[a.OutputKind < b.OutputKind]
	})
	return result, nil
}

func (m *memoryAssignments) Get(_ context.Context, _ platform.TransactionContext, kind string, id uuid.UUID, output string) (Assignment, error) {
	row, ok := m.rows[assignmentKey{kind, id, output}]
	if !ok {
		return Assignment{}, authz.ErrNotFound
	}
	return row, nil
}

func (m *memoryAssignments) Put(_ context.Context, _ platform.TransactionContext, a Assignment, expected int64) error {
	key := assignmentKey{a.ScopeKind, a.ScopeID, a.OutputKind}
	current, exists := m.rows[key]
	if (expected == 0 && exists) || (expected > 0 && current.Revision != expected) {
		return authz.ErrRevisionConflict
	}
	m.rows[key] = a
	return nil
}

func (m *memoryAssignments) Delete(_ context.Context, _ platform.TransactionContext, kind string, id uuid.UUID, output string, expected int64) error {
	key := assignmentKey{kind, id, output}
	if m.rows[key].Revision != expected {
		return authz.ErrRevisionConflict
	}
	delete(m.rows, key)
	return nil
}

func (m *memoryAssignments) ReadUnit(_ context.Context, _ platform.TransactionContext, id uuid.UUID) (UnitContext, error) {
	unit, ok := m.units[id]
	if !ok {
		return UnitContext{}, authz.ErrNotFound
	}
	return unit, nil
}

func (m *memoryAssignments) OrganizationExists(_ context.Context, _ platform.TransactionContext, id uuid.UUID) (bool, error) {
	return m.orgs[id], nil
}

type assignmentFixture struct {
	service *AssignmentService
	rows    *memoryAssignments
	layouts *memoryStore
	agencyA uuid.UUID
	agencyB uuid.UUID
	unit    uuid.UUID
	org     uuid.UUID
}

// newAssignmentFixture grants presentation.manage in agency A only.
func newAssignmentFixture(t *testing.T) *assignmentFixture {
	t.Helper()
	f := &assignmentFixture{rows: &memoryAssignments{rows: map[assignmentKey]Assignment{}, units: map[uuid.UUID]UnitContext{},
		orgs: map[uuid.UUID]bool{}}, layouts: newMemoryStore(), agencyA: uuid.New(), agencyB: uuid.New(), unit: uuid.New(), org: uuid.New()}
	f.rows.units[f.unit] = UnitContext{AgencyID: f.agencyA}
	f.rows.orgs[f.org] = true
	authority := authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true,
		Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: uuid.NewString(), SubjectKind: "principal",
			Actions: []string{ManageAction}, Scope: authz.Scope{Kind: "agency", References: []string{f.agencyA.String()}}}}}
	service, err := NewAssignmentService(fakeTransactions{},
		fakeResolver{identity.Authentication{Authority: authority, Kind: identity.SessionProof}}, fakeClock{}, f.rows, f.layouts)
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	return f
}

func (f *assignmentFixture) layout(t *testing.T, kind string, agencies ...uuid.UUID) uuid.UUID {
	t.Helper()
	definition, err := encode(textLayout(kind))
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := f.layouts.Insert(context.Background(), nil, Record{ID: id, Revision: 1, OutputKind: kind, Definition: definition,
		Agencies: agencies, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *assignmentFixture) put(scope string, id uuid.UUID, output string, layout uuid.UUID, expected int64) (Assignment, error) {
	return f.service.Put(context.Background(), identity.Proof{}, scope, id, output, layout, expected, allow)
}

func invalidField(t *testing.T, err error, field string) {
	t.Helper()
	var fieldError *authz.FieldError
	if !errors.As(err, &fieldError) || fieldError.Field != field {
		t.Fatalf("error = %v, want invalid field %q", err, field)
	}
}

func TestAssignmentUnitScope(t *testing.T) {
	t.Parallel()
	f := newAssignmentFixture(t)
	good := f.layout(t, "text", f.agencyA, f.agencyB)
	got, err := f.put(ScopeUnit, f.unit, "text", good, 0)
	if err != nil || got.Revision != 1 || got.LayoutID != good {
		t.Fatalf("create = %+v, %v", got, err)
	}
	if _, err := f.put(ScopeUnit, f.unit, "text", good, 0); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("create over existing: %v", err)
	}
	other := f.layout(t, "text", f.agencyA)
	got, err = f.put(ScopeUnit, f.unit, "text", other, 1)
	if err != nil || got.Revision != 2 || got.LayoutID != other {
		t.Fatalf("replace = %+v, %v", got, err)
	}
	if _, err := f.put(ScopeUnit, f.unit, "text", good, 1); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale replace: %v", err)
	}
	_, err = f.put(ScopeUnit, f.unit, "text", f.layout(t, "text", f.agencyB), 2)
	invalidField(t, err, "layoutId")
	_, err = f.put(ScopeUnit, f.unit, "markdown", good, 0)
	invalidField(t, err, "layoutId")
	_, err = f.put(ScopeUnit, f.unit, "text", uuid.New(), 2)
	invalidField(t, err, "layoutId")
	if _, err := f.put(ScopeUnit, uuid.New(), "text", good, 0); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("unknown unit: %v", err)
	}
	foreign := uuid.New()
	f.rows.units[foreign] = UnitContext{AgencyID: f.agencyB}
	if _, err := f.put(ScopeUnit, foreign, "text", good, 0); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("unit of an unmanaged Agency: %v", err)
	}
}

func TestAssignmentOrganizationScopeNeedsEveryAgency(t *testing.T) {
	t.Parallel()
	f := newAssignmentFixture(t)
	if _, err := f.put(ScopeOrganization, f.org, "text", f.layout(t, "text", f.agencyA, f.agencyB), 0); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("layout of an unmanaged Agency: %v", err)
	}
	only := f.layout(t, "text", f.agencyA)
	if got, err := f.put(ScopeOrganization, f.org, "text", only, 0); err != nil || got.Revision != 1 {
		t.Fatalf("organization = %+v, %v", got, err)
	}
	if _, err := f.put(ScopeOrganization, uuid.New(), "text", only, 0); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("unknown organization: %v", err)
	}
}

func TestAssignmentKeyIsValidated(t *testing.T) {
	t.Parallel()
	f := newAssignmentFixture(t)
	good := f.layout(t, "text", f.agencyA)
	_, err := f.put("agency", f.unit, "text", good, 0)
	invalidField(t, err, "scopeKind")
	_, err = f.put(ScopeUnit, f.unit, "html", good, 0)
	invalidField(t, err, "outputKind")
	_, err = f.put(ScopeUnit, f.unit, "text", uuid.Nil, 0)
	invalidField(t, err, "layoutId")
}

func TestAssignmentDeleteAndList(t *testing.T) {
	t.Parallel()
	f := newAssignmentFixture(t)
	good := f.layout(t, "text", f.agencyA)
	if _, err := f.put(ScopeUnit, f.unit, "text", good, 0); err != nil {
		t.Fatal(err)
	}
	// An assignment the caller may not manage stays out of the list.
	hidden := uuid.New()
	f.rows.units[hidden] = UnitContext{AgencyID: f.agencyB}
	f.rows.rows[assignmentKey{ScopeUnit, hidden, "text"}] = Assignment{ScopeKind: ScopeUnit, ScopeID: hidden, OutputKind: "text",
		LayoutID: f.layout(t, "text", f.agencyB), Revision: 1}
	page, err := f.service.List(context.Background(), identity.Proof{}, "", uuid.Nil, units.PageInput{Limit: 10})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ScopeID != f.unit {
		t.Fatalf("list = %+v, %v", page, err)
	}
	if _, err := f.service.List(context.Background(), identity.Proof{}, "agency", uuid.Nil, units.PageInput{Limit: 10}); err == nil {
		t.Fatal("list accepted an unknown scope kind")
	}
	if _, err := f.service.Delete(context.Background(), identity.Proof{}, ScopeUnit, f.unit, "text", 5, allow); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	got, err := f.service.Delete(context.Background(), identity.Proof{}, ScopeUnit, f.unit, "text", 1, allow)
	if err != nil || got.ID != f.unit || got.Revision != 2 {
		t.Fatalf("delete = %+v, %v", got, err)
	}
	if _, err := f.service.Delete(context.Background(), identity.Proof{}, ScopeUnit, f.unit, "text", 1, allow); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := f.service.Delete(context.Background(), identity.Proof{}, ScopeUnit, hidden, "text", 1, allow); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("delete of an unmanaged assignment: %v", err)
	}
}

func TestAssignmentsNeedPresentationManage(t *testing.T) {
	t.Parallel()
	f := newAssignmentFixture(t)
	f.service.resolver = fakeResolver{identity.Authentication{Kind: identity.SessionProof,
		Authority: authz.Authority{PrincipalID: uuid.NewString(), Revision: 1, Active: true}}}
	if _, err := f.service.List(context.Background(), identity.Proof{}, "", uuid.Nil, units.PageInput{Limit: 10}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("list: %v", err)
	}
	if _, err := f.put(ScopeUnit, uuid.New(), "text", uuid.New(), 0); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("put of an unknown unit without the right must not reveal it: %v", err)
	}
}
