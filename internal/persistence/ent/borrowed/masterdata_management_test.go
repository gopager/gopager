package borrowed_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/units"
)

func TestOrganizationDefaultsTemplateSnapshotAndAtomicBulkUnits(t *testing.T) {
	f := newMatcherFixture(t)
	defaultColor := "#AABBCC"
	changedDefault := "#445566"
	overrideColor := "#112233"
	organization, err := f.units.CreateOrganization(t.Context(), f.proof, uuid.New(), units.OrganizationInput{
		Name: "Operations",
		Defaults: units.Defaults{
			Color:            units.DefaultSetting[string]{Mode: "value", Value: &defaultColor},
			Icon:             units.DefaultSetting[string]{Mode: "unset"},
			FunctionMeanings: units.DefaultSetting[map[int]string]{Mode: "unset"},
		},
	}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}

	organizationID := organization.ID
	inherited, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Inherited", OrganizationID: &organizationID, CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	overridden, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency,
		Name: "Override", OrganizationID: &organizationID, CollectionIDs: []uuid.UUID{},
		Settings: units.Settings{Color: units.FieldSetting[string]{Mode: "value", Value: &overrideColor}},
	}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	template, err := f.units.CreateUnitTemplate(t.Context(), f.proof, uuid.New(), units.UnitTemplateInput{
		Name: "Radio", OrganizationID: &organizationID,
	}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := f.units.CreateUnitFromTemplate(t.Context(), f.proof, uuid.New(), template.ID,
		units.UnitInput{AgencyID: f.agency, Name: "Copied", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	copiedUnit, err := f.units.ReadUnit(t.Context(), f.proof, copied.ID)
	if err != nil || copiedUnit.Settings.Color.Mode != "value" ||
		copiedUnit.Settings.Color.Value == nil || *copiedUnit.Settings.Color.Value != defaultColor {
		t.Fatalf("template did not copy its current effective settings: %+v %v", copiedUnit.Settings, err)
	}

	if _, err := f.units.ChangeOrganization(t.Context(), f.proof, organization.ID, organization.Revision,
		units.OrganizationInput{Name: "Operations", Defaults: units.Defaults{
			Color:            units.DefaultSetting[string]{Mode: "value", Value: &changedDefault},
			Icon:             units.DefaultSetting[string]{Mode: "unset"},
			FunctionMeanings: units.DefaultSetting[map[int]string]{Mode: "unset"},
		}}, checkedMutation); err != nil {
		t.Fatal(err)
	}
	if _, err := f.units.DeleteOrganization(t.Context(), f.proof, organization.ID, organization.Revision, checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale Organization revision did not reject deletion: %v", err)
	}
	stillAttached, err := f.units.ReadUnit(t.Context(), f.proof, inherited.ID)
	if err != nil || stillAttached.OrganizationID == nil || *stillAttached.OrganizationID != organization.ID ||
		stillAttached.Settings.Color.Mode != "inherit" {
		t.Fatalf("stale Organization deletion partially changed its Unit: %+v %v", stillAttached, err)
	}
	copiedUnit, err = f.units.ReadUnit(t.Context(), f.proof, copied.ID)
	if err != nil || copiedUnit.OrganizationID == nil || *copiedUnit.OrganizationID != organization.ID ||
		copiedUnit.Settings.Color.Value == nil || *copiedUnit.Settings.Color.Value != defaultColor {
		t.Fatalf("template edit/default change mutated copied Unit: %+v %v", copiedUnit, err)
	}
	beforeDelete := configurationCounts(t, f)
	if _, err := f.units.DeleteOrganization(t.Context(), f.proof, organization.ID, 2, checkedMutation); err != nil {
		t.Fatal(err)
	}
	if afterDelete := configurationCounts(t, f); afterDelete[5] != beforeDelete[5] || afterDelete[6] != beforeDelete[6] {
		t.Fatalf("Organization deletion scheduled Rematch work: before=%v after=%v", beforeDelete, afterDelete)
	}
	for _, id := range []uuid.UUID{inherited.ID, copied.ID} {
		value, err := f.units.ReadUnit(t.Context(), f.proof, id)
		if err != nil || value.OrganizationID != nil || value.Settings.Color.Mode != "value" ||
			value.Settings.Color.Value == nil {
			t.Fatalf("Organization deletion did not materialize Unit defaults: %+v %v", value, err)
		}
		want := changedDefault
		if id == copied.ID {
			want = defaultColor
		}
		if *value.Settings.Color.Value != want {
			t.Fatalf("Unit %s color = %s, want %s", id, *value.Settings.Color.Value, want)
		}
	}
	preserved, err := f.units.ReadUnit(t.Context(), f.proof, overridden.ID)
	if err != nil || preserved.OrganizationID != nil || preserved.Settings.Color.Value == nil ||
		*preserved.Settings.Color.Value != overrideColor {
		t.Fatalf("Organization deletion changed explicit override: %+v %v", preserved, err)
	}
	detachedTemplate, err := f.units.ReadUnitTemplate(t.Context(), f.proof, template.ID)
	if err != nil || detachedTemplate.OrganizationID != nil || detachedTemplate.Settings.Color.Mode != "value" ||
		detachedTemplate.Settings.Color.Value == nil || *detachedTemplate.Settings.Color.Value != changedDefault {
		t.Fatalf("Organization deletion did not materialize template defaults: %+v %v", detachedTemplate, err)
	}
	hiddenOrganization, err := f.units.CreateOrganization(t.Context(), f.proof, uuid.New(), units.OrganizationInput{
		Name: "Hidden Operations",
		Defaults: units.Defaults{
			Color:            units.DefaultSetting[string]{Mode: "unset"},
			Icon:             units.DefaultSetting[string]{Mode: "unset"},
			FunctionMeanings: units.DefaultSetting[map[int]string]{Mode: "unset"},
		},
	}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	hiddenOrganizationID := hiddenOrganization.ID
	hiddenUnit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Hidden Station", OrganizationID: &hiddenOrganizationID, CollectionIDs: []uuid.UUID{}},
		checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	hiddenTemplate, err := f.units.CreateUnitTemplate(t.Context(), f.proof, uuid.New(), units.UnitTemplateInput{
		Name: "Hidden Template", OrganizationID: &hiddenOrganizationID,
	}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{
		Actions: []string{"organization.delete"}, Scope: authz.Scope{Kind: "all"},
	}})
	if _, err := f.units.DeleteOrganization(t.Context(), f.proof, hiddenOrganization.ID,
		hiddenOrganization.Revision, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Organization deletion ignored an unmanageable Unit: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{
		Actions: []string{"organization.delete", "unit.update"}, Scope: authz.Scope{Kind: "all"},
	}})
	if _, err := f.units.DeleteOrganization(t.Context(), f.proof, hiddenOrganization.ID,
		hiddenOrganization.Revision, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Organization deletion ignored an unmanageable template: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: authz.Actions(), Scope: authz.Scope{Kind: "all"}}})
	hiddenUnitAfter, err := f.units.ReadUnit(t.Context(), f.proof, hiddenUnit.ID)
	if err != nil || hiddenUnitAfter.OrganizationID == nil || *hiddenUnitAfter.OrganizationID != hiddenOrganization.ID {
		t.Fatalf("denied Organization deletion detached its hidden Unit: %+v %v", hiddenUnitAfter, err)
	}
	if _, err := f.units.ReadOrganization(t.Context(), f.proof, hiddenOrganization.ID); err != nil {
		t.Fatalf("denied Organization deletion removed the Organization: %v", err)
	}
	hiddenTemplateAfter, err := f.units.ReadUnitTemplate(t.Context(), f.proof, hiddenTemplate.ID)
	if err != nil || hiddenTemplateAfter.OrganizationID == nil ||
		*hiddenTemplateAfter.OrganizationID != hiddenOrganization.ID {
		t.Fatalf("denied Organization deletion detached its hidden template: %+v %v", hiddenTemplateAfter, err)
	}

	bulkA, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Bulk A", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	bulkB, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agency, Name: "Bulk B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	bulkColor := "#123456"
	bulkSettings := units.Settings{
		Color:            units.FieldSetting[string]{Mode: "value", Value: &bulkColor},
		Icon:             units.FieldSetting[string]{Mode: "unset"},
		FunctionMeanings: units.FieldSetting[map[int]string]{Mode: "unset"},
	}
	input := units.BulkUnitInput{Operation: "update", Items: []units.BulkUnitItem{
		{ID: bulkA.ID, ExpectedRevision: 1, Settings: &bulkSettings},
		{ID: bulkB.ID, ExpectedRevision: 99, Settings: &bulkSettings},
	}}
	preview, err := f.units.PreviewBulkUnits(t.Context(), f.proof, input, nil)
	if err != nil || preview.Valid || len(preview.Items) != 2 {
		t.Fatalf("invalid bulk preview was not caller-safe: %+v %v", preview, err)
	}
	if _, err := f.units.BulkUnits(t.Context(), f.proof, uuid.New(), input, checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale bulk update did not reject atomically: %v", err)
	}
	for _, id := range []uuid.UUID{bulkA.ID, bulkB.ID} {
		value, err := f.units.ReadUnit(t.Context(), f.proof, id)
		if err != nil || value.Revision != 1 || value.Settings.Color.Value != nil {
			t.Fatalf("failed bulk operation partially changed Unit %s: %+v %v", id, value, err)
		}
	}
	input.Items[1].ExpectedRevision = 1
	results, err := f.units.BulkUnits(t.Context(), f.proof, uuid.New(), input, checkedMutation)
	if err != nil || len(results) != 2 {
		t.Fatalf("valid bulk update failed: %+v %v", results, err)
	}
	for _, item := range results {
		if item.Revision != 2 {
			t.Fatalf("bulk result revision = %d, want 2", item.Revision)
		}
	}
	noOp, err := f.units.BulkUnits(t.Context(), f.proof, uuid.New(),
		units.BulkUnitInput{Operation: "update", Items: []units.BulkUnitItem{
			{ID: bulkA.ID, ExpectedRevision: 2, Settings: &bulkSettings},
		}}, checkedMutation)
	if err != nil || len(noOp) != 1 || noOp[0].Revision != 2 {
		t.Fatalf("no-op bulk result reported a revision that was not stored: %+v %v", noOp, err)
	}
	afterNoOp, err := f.units.ReadUnit(t.Context(), f.proof, bulkA.ID)
	if err != nil || afterNoOp.Revision != 2 || !reflect.DeepEqual(afterNoOp.Settings, bulkSettings) {
		t.Fatalf("no-op bulk update changed stored state: %+v %v", afterNoOp, err)
	}

	bulkItems := make([]units.BulkUnitItem, 100)
	for i := range bulkItems {
		unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
			units.UnitInput{AgencyID: f.agency, Name: "Batch " + string(rune('A'+i%26)) + " " + string(rune('0'+i/26)),
				CollectionIDs: []uuid.UUID{}}, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		bulkItems[i] = units.BulkUnitItem{ID: unit.ID, ExpectedRevision: unit.Revision, Settings: &bulkSettings}
	}
	hundred := units.BulkUnitInput{Operation: "update", Items: bulkItems}
	preview, err = f.units.PreviewBulkUnits(t.Context(), f.proof, hundred, nil)
	if err != nil || !preview.Valid || len(preview.Items) != 100 {
		t.Fatalf("100-item bulk preview failed: items=%d valid=%t err=%v", len(preview.Items), preview.Valid, err)
	}
	previewUnit, err := f.units.ReadUnit(t.Context(), f.proof, bulkItems[0].ID)
	if err != nil || previewUnit.Revision != 1 {
		t.Fatalf("bulk preview mutated its first Unit: %+v %v", previewUnit, err)
	}
	results, err = f.units.BulkUnits(t.Context(), f.proof, uuid.New(), hundred, checkedMutation)
	if err != nil || len(results) != 100 {
		t.Fatalf("100-item bulk update failed: items=%d err=%v", len(results), err)
	}
	for _, result := range results {
		if result.Revision != 2 {
			t.Fatalf("100-item bulk result revision=%d, want 2", result.Revision)
		}
	}
	overflow := hundred
	overflow.Items = append(append([]units.BulkUnitItem(nil), hundred.Items...), hundred.Items[0])
	if _, err := f.units.BulkUnits(t.Context(), f.proof, uuid.New(), overflow, checkedMutation); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("101-item bulk update was not rejected: %v", err)
	}
	duplicate := units.BulkUnitInput{Operation: "update", Items: []units.BulkUnitItem{
		hundred.Items[0], hundred.Items[0],
	}}
	if _, err := f.units.BulkUnits(t.Context(), f.proof, uuid.New(), duplicate, checkedMutation); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("duplicate Unit IDs were not rejected: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"unit.read"}, Scope: authz.Scope{Kind: "all"}}})
	if _, err := f.units.BulkUnits(t.Context(), f.proof, uuid.New(),
		units.BulkUnitInput{Operation: "update", Items: []units.BulkUnitItem{{
			ID: bulkItems[0].ID, ExpectedRevision: 2, Settings: &bulkSettings,
		}}}, checkedMutation); !errors.Is(err, authz.ErrForbidden) && !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("unauthorized bulk target was not rejected: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: authz.Actions(), Scope: authz.Scope{Kind: "all"}}})
	afterDenied, err := f.units.ReadUnit(t.Context(), f.proof, bulkItems[0].ID)
	if err != nil || afterDenied.Revision != 2 || !reflect.DeepEqual(afterDenied.Settings, bulkSettings) {
		t.Fatalf("denied bulk operation changed the Unit: %+v %v", afterDenied, err)
	}
}

func TestDeleteOrganizationRollsBackOnStaleAffectedUnitRevision(t *testing.T) {
	f := newMatcherFixture(t)
	color := "#AABBCC"
	organization, err := f.units.CreateOrganization(t.Context(), f.proof, uuid.New(), units.OrganizationInput{
		Name: "Concurrent Operations",
		Defaults: units.Defaults{
			Color:            units.DefaultSetting[string]{Mode: "value", Value: &color},
			Icon:             units.DefaultSetting[string]{Mode: "unset"},
			FunctionMeanings: units.DefaultSetting[map[int]string]{Mode: "unset"},
		},
	}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	organizationID := organization.ID
	members := make([]units.Unit, 2)
	for i := range members {
		created, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agency,
			Name: "Concurrent Member", OrganizationID: &organizationID, CollectionIDs: []uuid.UUID{},
		}, checkedMutation)
		if err != nil {
			t.Fatal(err)
		}
		members[i], err = f.units.ReadUnit(t.Context(), f.proof, created.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	staleID := members[0].ID
	if members[1].ID.String() > staleID.String() {
		staleID = members[1].ID
	}
	repository := &staleUnitSaveRepository{Repository: f.unitStore, store: f.store, staleID: staleID}
	service, err := units.NewService(f.store, f.resolver, f.store, repository, f.ruleStore, f.receipts, f.facts)
	if err != nil {
		t.Fatal(err)
	}
	before := configurationCounts(t, f)
	if _, err := service.DeleteOrganization(t.Context(), f.proof, organization.ID,
		organization.Revision, checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale affected Unit revision did not abort Organization deletion: %v", err)
	}
	if after := configurationCounts(t, f); after != before {
		t.Fatalf("failed Organization deletion committed partial state: before=%v after=%v", before, after)
	}
	for _, member := range members {
		value, err := f.units.ReadUnit(t.Context(), f.proof, member.ID)
		if err != nil || value.Revision != member.Revision ||
			value.OrganizationID == nil || *value.OrganizationID != organization.ID ||
			value.Settings.Color.Mode != "inherit" {
			t.Fatalf("failed Organization deletion changed Unit %s: %+v %v", member.ID, value, err)
		}
	}
	current, err := f.units.ReadOrganization(t.Context(), f.proof, organization.ID)
	if err != nil || current.Revision != organization.Revision {
		t.Fatalf("failed Organization deletion changed its owner: %+v %v", current, err)
	}
}

// TestTemplateActionsAreNotInferredFromUnitOrOrganizationRights covers the rule that template actions require explicit grants.
func TestTemplateActionsAreNotInferredFromUnitOrOrganizationRights(t *testing.T) {
	f := newMatcherFixture(t)
	template, err := f.units.CreateUnitTemplate(t.Context(), f.proof, uuid.New(),
		units.UnitTemplateInput{Name: "Explicit Template"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	var withoutTemplate []string
	for _, action := range authz.Actions() {
		if len(action) < len("unit-template.") || action[:len("unit-template.")] != "unit-template." {
			withoutTemplate = append(withoutTemplate, action)
		}
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: withoutTemplate, Scope: authz.Scope{Kind: "all"}}})
	before := configurationCounts(t, f)
	if _, err := f.units.ReadUnitTemplate(t.Context(), f.proof, template.ID); !errors.Is(err, authz.ErrForbidden) &&
		!errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("template read inferred from other rights: %v", err)
	}
	if _, err := f.units.CreateUnitTemplate(t.Context(), f.proof, uuid.New(),
		units.UnitTemplateInput{Name: "Inferred"}, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("template create inferred from other rights: %v", err)
	}
	if _, err := f.units.ChangeUnitTemplate(t.Context(), f.proof, template.ID, template.Revision,
		units.UnitTemplateInput{Name: "Changed"}, checkedMutation); !errors.Is(err, authz.ErrForbidden) &&
		!errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("template update inferred from other rights: %v", err)
	}
	if _, err := f.units.DeleteUnitTemplate(t.Context(), f.proof, template.ID, template.Revision,
		checkedMutation); !errors.Is(err, authz.ErrForbidden) && !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("template delete inferred from other rights: %v", err)
	}
	if _, err := f.units.CreateUnitFromTemplate(t.Context(), f.proof, uuid.New(), template.ID,
		units.UnitInput{AgencyID: f.agency, Name: "Copy", CollectionIDs: []uuid.UUID{}}, checkedMutation); !errors.Is(err, authz.ErrForbidden) &&
		!errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("template copy inferred from unit.create: %v", err)
	}
	if after := configurationCounts(t, f); after != before {
		t.Fatalf("denied template operations committed state: before=%v after=%v", before, after)
	}
}
