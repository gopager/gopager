package borrowed_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/presentationstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// presentationWorld seeds Agencies, Organizations, Units and Layouts with plain SQL.
type presentationWorld struct {
	t      *testing.T
	store  *postgres.Store
	layers *presentationstore.Store
}

func newPresentationWorld(t *testing.T) *presentationWorld {
	t.Helper()
	store := newStore(t)
	layers, err := presentationstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	return &presentationWorld{t: t, store: store, layers: layers}
}

func (w *presentationWorld) exec(query string, args ...any) {
	w.t.Helper()
	if err := w.store.WithinTransaction(w.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := w.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), query, args...)
		return err
	}); err != nil {
		w.t.Fatal(err)
	}
}

func (w *presentationWorld) agency() uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO agencies (id, name) VALUES ($1, 'Agency')`, id)
	return id
}

func (w *presentationWorld) organization() uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO organizations (id, name, defaults) VALUES ($1, 'Org', '{}'::jsonb)`, id)
	return id
}

func (w *presentationWorld) unit(agency uuid.UUID, organization uuid.UUID) uuid.UUID {
	id := uuid.New()
	var org any
	if organization != uuid.Nil {
		org = organization
	}
	w.exec(`INSERT INTO units (id, name, agency_id, organization_id) VALUES ($1, 'Unit', $2, $3)`, id, agency, org)
	return id
}

func (w *presentationWorld) layout(kind string, agencies ...uuid.UUID) uuid.UUID {
	id := uuid.New()
	w.exec(`INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at)
		VALUES ($1, 1, 'Layout', $2, $3, now(), now())`, id, kind, uuid.New())
	w.exec(`INSERT INTO layout_definition_versions (layout_id, version, definition, created_by, created_at)
		VALUES ($1, 1, '{}'::jsonb, $2, now())`, id, uuid.New())
	for _, agency := range agencies {
		w.exec(`INSERT INTO layout_definition_agencies (layout_id, agency_id) VALUES ($1, $2)`, id, agency)
	}
	return id
}

func (w *presentationWorld) within(fn func(tx platform.TransactionContext) error) error {
	return w.store.WithinTransaction(w.t.Context(), fn)
}

func (w *presentationWorld) put(kind string, scope uuid.UUID, output string, layout uuid.UUID, expected int64) error {
	return w.within(func(tx platform.TransactionContext) error {
		return w.layers.Put(tx.Context(), tx, presentation.Assignment{ScopeKind: kind, ScopeID: scope, OutputKind: output,
			LayoutID: layout, Revision: expected + 1, UpdatedAt: time.Now()}, expected)
	})
}

func (w *presentationWorld) unitLayouts(output string, units ...uuid.UUID) map[uuid.UUID]uuid.UUID {
	w.t.Helper()
	var result map[uuid.UUID]uuid.UUID
	if err := w.within(func(tx platform.TransactionContext) error {
		var err error
		result, err = w.layers.UnitLayouts(tx.Context(), tx, units, output)
		return err
	}); err != nil {
		w.t.Fatal(err)
	}
	return result
}

func TestInstallationSettingsAreSeededAndRevisioned(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	read := func() presentation.Installation {
		var got presentation.Installation
		if err := w.within(func(tx platform.TransactionContext) error {
			var err error
			got, err = w.layers.ReadInstallation(tx.Context(), tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := read(); got.Title != "GoPager" || got.Revision != 1 || got.Banner != nil || got.DefaultLayoutID != nil {
		t.Fatalf("seed = %+v", got)
	}
	update := func(expected int64, input presentation.InstallationInput) error {
		return w.within(func(tx platform.TransactionContext) error {
			return w.layers.UpdateInstallation(tx.Context(), tx, expected, input, time.Now())
		})
	}
	layout := w.layout("text", w.agency())
	banner, severity := "Maintenance tonight", "warning"
	if err := update(1, presentation.InstallationInput{Title: "Control room", Banner: &banner, Severity: &severity,
		DefaultLayoutID: &layout}); err != nil {
		t.Fatal(err)
	}
	got := read()
	if got.Title != "Control room" || got.Revision != 2 || *got.Banner != banner || *got.Severity != severity || *got.DefaultLayoutID != layout {
		t.Fatalf("updated = %+v", got)
	}
	if err := update(1, presentation.InstallationInput{Title: "Stale"}); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale: %v", err)
	}
	missing := uuid.New()
	err := update(2, presentation.InstallationInput{Title: "t", DefaultLayoutID: &missing})
	var fieldError *authz.FieldError
	if !errors.As(err, &fieldError) || fieldError.Field != "defaultLayoutId" {
		t.Fatalf("missing default Layout: %v", err)
	}
	if err := update(2, presentation.InstallationInput{Title: "Cleared"}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.Banner != nil || got.Severity != nil || got.DefaultLayoutID != nil || got.Revision != 3 {
		t.Fatalf("cleared = %+v", got)
	}
}

func TestLayoutReferencesAreReported(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	agency := w.agency()
	unit := w.unit(agency, uuid.Nil)
	free, assigned, installed := w.layout("text", agency), w.layout("text", agency), w.layout("text", agency)
	if err := w.put(presentation.ScopeUnit, unit, "text", assigned, 0); err != nil {
		t.Fatal(err)
	}
	if err := w.within(func(tx platform.TransactionContext) error {
		return w.layers.UpdateInstallation(tx.Context(), tx, 1, presentation.InstallationInput{Title: "t", DefaultLayoutID: &installed}, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	for layout, want := range map[uuid.UUID]bool{free: false, assigned: true, installed: true} {
		var got bool
		if err := w.within(func(tx platform.TransactionContext) error {
			var err error
			got, err = w.layers.LayoutReferenced(tx.Context(), tx, layout)
			return err
		}); err != nil || got != want {
			t.Fatalf("referenced(%s) = %v, %v; want %v", layout, got, err, want)
		}
	}
}

func TestAssignmentRevisions(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	agency := w.agency()
	unit := w.unit(agency, uuid.Nil)
	first, second := w.layout("text", agency), w.layout("text", agency)
	if err := w.put(presentation.ScopeUnit, unit, "text", first, 0); err != nil {
		t.Fatal(err)
	}
	if err := w.put(presentation.ScopeUnit, unit, "text", second, 0); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("insert over existing: %v", err)
	}
	if err := w.put(presentation.ScopeUnit, unit, "text", second, 5); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale replace: %v", err)
	}
	if err := w.put(presentation.ScopeUnit, unit, "text", second, 1); err != nil {
		t.Fatal(err)
	}
	if err := w.put(presentation.ScopeUnit, unit, "text", uuid.New(), 2); err == nil {
		t.Fatal("assignment of a missing Layout accepted")
	} else if fieldError := new(authz.FieldError); !errors.As(err, &fieldError) || fieldError.Field != "layoutId" {
		t.Fatalf("missing Layout: %v", err)
	}
	var listed []presentation.Assignment
	var read presentation.Assignment
	if err := w.within(func(tx platform.TransactionContext) error {
		var err error
		if listed, err = w.layers.List(tx.Context(), tx, presentation.ScopeUnit, unit); err != nil {
			return err
		}
		read, err = w.layers.Get(tx.Context(), tx, presentation.ScopeUnit, unit, "text")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].LayoutID != second || listed[0].Revision != 2 || read.LayoutID != second {
		t.Fatalf("listed %+v, read %+v", listed, read)
	}
	remove := func(expected int64) error {
		return w.within(func(tx platform.TransactionContext) error {
			return w.layers.Delete(tx.Context(), tx, presentation.ScopeUnit, unit, "text", expected)
		})
	}
	if err := remove(1); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err := remove(2); err != nil {
		t.Fatal(err)
	}
	if err := remove(2); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestUnitAndOrganizationLookup(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	agency, org := w.agency(), w.organization()
	unit := w.unit(agency, org)
	check := func(unit uuid.UUID, org uuid.UUID) (presentation.UnitContext, error, bool) {
		var context presentation.UnitContext
		var exists bool
		var unitErr error
		if err := w.within(func(tx platform.TransactionContext) error {
			var err error
			context, unitErr = w.layers.ReadUnit(tx.Context(), tx, unit)
			exists, err = w.layers.OrganizationExists(tx.Context(), tx, org)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return context, unitErr, exists
	}
	if context, err, exists := check(unit, org); err != nil || context.AgencyID != agency || !exists {
		t.Fatalf("lookup = %+v, %v, %v", context, err, exists)
	}
	if _, err, exists := check(uuid.New(), uuid.New()); !errors.Is(err, authz.ErrNotFound) || exists {
		t.Fatalf("unknown = %v, %v", err, exists)
	}
	w.exec(`UPDATE units SET deleted = true WHERE id = $1`, unit)
	w.exec(`UPDATE organizations SET deleted = true WHERE id = $1`, org)
	if _, err, exists := check(unit, org); !errors.Is(err, authz.ErrNotFound) || exists {
		t.Fatalf("deleted = %v, %v", err, exists)
	}
}

func TestDeletingAUnitOrOrganizationRemovesItsAssignments(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	agency, org := w.agency(), w.organization()
	unit := w.unit(agency, org)
	layout := w.layout("text", agency)
	if err := w.put(presentation.ScopeUnit, unit, "text", layout, 0); err != nil {
		t.Fatal(err)
	}
	if err := w.put(presentation.ScopeOrganization, org, "text", layout, 0); err != nil {
		t.Fatal(err)
	}
	w.exec(`UPDATE units SET deleted = true WHERE id = $1`, unit)
	w.exec(`UPDATE organizations SET deleted = true WHERE id = $1`, org)
	var left []presentation.Assignment
	if err := w.within(func(tx platform.TransactionContext) error {
		var err error
		left, err = w.layers.List(tx.Context(), tx, "", uuid.Nil)
		return err
	}); err != nil || len(left) != 0 {
		t.Fatalf("assignments after deleting their scopes: %+v, %v", left, err)
	}
}

func TestUnitLayoutsFollowTheLayers(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	agencyA, agencyB := w.agency(), w.agency()
	org, otherOrg := w.organization(), w.organization()
	byUnit, byOrg, byBoth, noValue := w.unit(agencyA, org), w.unit(agencyA, org), w.unit(agencyA, org), w.unit(agencyA, otherOrg)
	elsewhere := w.unit(agencyB, org)
	unitLayout, orgLayout := w.layout("text", agencyA), w.layout("text", agencyA)
	markdown := w.layout("markdown", agencyA)
	for _, step := range []struct {
		kind   string
		scope  uuid.UUID
		output string
		layout uuid.UUID
	}{
		{presentation.ScopeUnit, byUnit, "text", unitLayout},
		{presentation.ScopeUnit, byBoth, "text", unitLayout},
		{presentation.ScopeOrganization, org, "text", orgLayout},
		{presentation.ScopeOrganization, org, "markdown", markdown},
	} {
		if err := w.put(step.kind, step.scope, step.output, step.layout, 0); err != nil {
			t.Fatal(err)
		}
	}
	got := w.unitLayouts("text", byUnit, byOrg, byBoth, noValue, elsewhere, uuid.New())
	// The Unit of Agency B loses the Organization's Layout, which is not assigned to its Agency.
	want := map[uuid.UUID]uuid.UUID{byUnit: unitLayout, byOrg: orgLayout, byBoth: unitLayout}
	if len(got) != len(want) {
		t.Fatalf("layers = %v, want %v", got, want)
	}
	for unit, layout := range want {
		if got[unit] != layout {
			t.Fatalf("layers = %v, want %v", got, want)
		}
	}
	if got := w.unitLayouts("markdown", byOrg); got[byOrg] != markdown {
		t.Fatalf("markdown layers = %v", got)
	}
	// A Unit assignment whose Layout lost the Unit's Agency falls through to the Organization.
	w.exec(`DELETE FROM layout_definition_agencies WHERE layout_id = $1`, unitLayout)
	w.exec(`INSERT INTO layout_definition_agencies (layout_id, agency_id) VALUES ($1, $2)`, unitLayout, agencyB)
	if got := w.unitLayouts("text", byUnit); got[byUnit] != orgLayout {
		t.Fatalf("stale Unit assignment: %v", got)
	}
	w.exec(`DELETE FROM layout_definition_agencies WHERE layout_id = $1`, orgLayout)
	if got := w.unitLayouts("text", byUnit, byOrg); len(got) != 0 {
		t.Fatalf("stale assignments: %v", got)
	}
}

func TestInstallationDefaultReportsItsOutputKind(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	read := func() presentation.DefaultLayout {
		var got presentation.DefaultLayout
		if err := w.within(func(tx platform.TransactionContext) error {
			var err error
			got, err = w.layers.InstallationDefault(tx.Context(), tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := read(); got.ID != uuid.Nil {
		t.Fatalf("unset default = %+v", got)
	}
	layout := w.layout("markdown", w.agency())
	if err := w.within(func(tx platform.TransactionContext) error {
		return w.layers.UpdateInstallation(tx.Context(), tx, 1, presentation.InstallationInput{Title: "t", DefaultLayoutID: &layout}, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.ID != layout || got.OutputKind != "markdown" {
		t.Fatalf("default = %+v", got)
	}
}
