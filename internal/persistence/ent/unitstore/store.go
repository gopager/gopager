// Package unitstore implements the Unit owner's ports with native Ent row filtering.
package unitstore

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/organization"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
	"github.com/gopager/gopager/internal/persistence/ent/unit"
	"github.com/gopager/gopager/internal/persistence/ent/unitcollection"
	"github.com/gopager/gopager/internal/persistence/ent/unittemplate"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/units"
)

// Store persists Units, Organizations, Unit templates and Unit collections, filtering rows
// natively by the caller's authority. It holds no transaction of its own: every method
// runs in the caller's active primary transaction.
type Store struct{ primary *postgres.Store }

// New binds all owner operations to the existing platform Primary.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Store{primary: primary}, nil
}

// client installs only the adapter's transaction-bound native predicates/capabilities.
func (s *Store) client(ctx context.Context, tx platform.TransactionContext, predicates map[string]entql.P, create map[string]bool) (*ent.Client, context.Context, error) {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return nil, nil, err
	}
	return ent.NewClient(ent.Driver(driver)), guard.Bind(ctx, guard.Access{
		Current: driver.Current, Predicates: predicates, Create: create,
	}), nil
}

// referenceValues supplies UUID-typed SQL parameters, not string predicates.
func referenceValues(references []uuid.UUID) []any {
	values := make([]any, len(references))
	for i, id := range references {
		values[i] = id
	}
	return values
}

// unitScope binds each grant to the same current Unit and its actual owner relations.
func unitScope(kind string, references []uuid.UUID) (entql.P, error) {
	values := referenceValues(references)
	switch kind {
	case "unit":
		return entql.FieldIn("id", values...), nil
	case "unitCollection":
		return entql.HasEdgeWith("collections", entql.FieldIn("id", values...)), nil
	case "agency":
		return entql.FieldIn("agency_id", values...), nil
	default:
		return nil, authz.ErrInvalidInput
	}
}

// collectionScope correlates grants through actual members, not an unrestricted ID union.
func collectionScope(kind string, references []uuid.UUID) (entql.P, error) {
	values := referenceValues(references)
	switch kind {
	case "unitCollection":
		return entql.FieldIn("id", values...), nil
	case "unit":
		return entql.HasEdgeWith("units", entql.And(entql.FieldEQ("deleted", false), entql.FieldIn("id", values...))), nil
	case "agency":
		return entql.HasEdgeWith("units", entql.And(entql.FieldEQ("deleted", false), entql.FieldIn("agency_id", values...))), nil
	default:
		return nil, authz.ErrInvalidInput
	}
}

// agencyScope binds a grant to Agencies themselves; Agency management is grantable on all or on
// the Agency, never through Units or collections.
func agencyScope(kind string, references []uuid.UUID) (entql.P, error) {
	if kind != "agency" {
		return nil, authz.ErrInvalidInput
	}
	return entql.FieldIn("id", referenceValues(references)...), nil
}

// plan preserves the central action/key/field composition while binding owner relations.
func plan(authority authz.Authority, action string, collection bool, now time.Time) (entql.P, error) {
	var fields []string
	if strings.HasSuffix(action, ".read") {
		fields = []string{"unit"}
	}
	binding := unitScope
	if collection {
		binding = collectionScope
	}
	result, err := permissions.ReadScope(authority, action, fields, permissions.Attributes{Scope: binding}, now)
	return result.Predicate, err
}

func organizationPlan(authority authz.Authority, action string, now time.Time) (entql.P, error) {
	fields := []string(nil)
	if strings.HasSuffix(action, ".read") {
		fields = []string{"unit"}
	}
	result, err := permissions.ReadScope(authority, action, fields, permissions.Attributes{Scope: unitScope}, now)
	if err != nil {
		return nil, err
	}
	if authz.Authorize(authority, action, fields, authz.Target{}, now) == nil {
		return entql.FieldNotNil("id"), nil
	}
	return entql.And(entql.HasEdgeWith("units"),
		entql.Not(entql.HasEdgeWith("units", entql.Not(result.Predicate)))), nil
}

// unitDTO projects the supported public configuration without exposing Ent entities.
func unitDTO(row *ent.Unit) units.Unit {
	result := units.Unit{ID: row.ID, Revision: row.Revision, Name: row.Name, AgencyID: row.AgencyID, Deleted: row.Deleted,
		OrganizationID: row.OrganizationID, Settings: row.Settings, CollectionIDs: []uuid.UUID{}}
	var defaults *units.Defaults
	if row.Edges.Organization != nil {
		value := row.Edges.Organization.Defaults
		defaults = &value
	}
	result.EffectiveSettings = units.ResolveSettings(row.Settings, defaults)
	for _, collection := range row.Edges.Collections {
		result.CollectionIDs = append(result.CollectionIDs, collection.ID)
	}
	return result
}

func (s *Store) lockOrganizations(ctx context.Context, tx platform.TransactionContext, ids ...*uuid.UUID) error {
	unique := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if id != nil {
			unique[*id] = struct{}{}
		}
	}
	ordered := make([]uuid.UUID, 0, len(unique))
	for id := range unique {
		ordered = append(ordered, id)
	}
	slices.SortFunc(ordered, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	for _, id := range ordered {
		client, scope, err := s.client(ctx, tx, map[string]entql.P{
			"Organization": entql.And(entql.FieldEQ("id", id), entql.FieldEQ("deleted", false)),
		}, nil)
		if err != nil {
			return err
		}
		if _, err := client.Organization.Query().Where(organization.IDEQ(id), organization.DeletedEQ(false)).
			ForUpdate().Only(scope); ent.IsNotFound(err) {
			return authz.ErrRevisionConflict
		} else if err != nil {
			return err
		}
	}
	return nil
}

func templateDTO(row *ent.UnitTemplate) units.UnitTemplate {
	result := units.UnitTemplate{ID: row.ID, Revision: row.Revision, Name: row.Name,
		OrganizationID: row.OrganizationID, Settings: row.Settings, Deleted: row.Deleted}
	var defaults *units.Defaults
	if row.Edges.Organization != nil {
		value := row.Edges.Organization.Defaults
		defaults = &value
	}
	result.EffectiveSettings = units.ResolveSettings(row.Settings, defaults)
	return result
}

// ReadOrganizationContext locks an Organization and its current member rows in stable order.
func (s *Store) ReadOrganizationContext(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (units.Organization, []units.Unit, []units.UnitTemplate, error) {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Organization":   entql.FieldEQ("id", id),
		"Unit":           entql.FieldEQ("organization_id", id),
		"UnitCollection": entql.FieldNotNil("id"),
		"UnitTemplate":   entql.FieldEQ("organization_id", id),
	}, nil)
	if err != nil {
		return units.Organization{}, nil, nil, err
	}
	row, err := client.Organization.Query().Where(organization.IDEQ(id), organization.DeletedEQ(false)).ForUpdate().Only(scope)
	if ent.IsNotFound(err) {
		return units.Organization{}, nil, nil, authz.ErrNotFound
	}
	if err != nil {
		return units.Organization{}, nil, nil, err
	}
	unitRows, err := client.Unit.Query().Where(unit.OrganizationIDEQ(id)).Order(ent.Asc(unit.FieldID)).ForUpdate().
		WithCollections(withCollections).All(scope)
	if err != nil {
		return units.Organization{}, nil, nil, err
	}
	members := make([]units.Unit, len(unitRows))
	for i, member := range unitRows {
		value := units.Unit{ID: member.ID, Revision: member.Revision, Name: member.Name, AgencyID: member.AgencyID,
			Deleted: member.Deleted, OrganizationID: member.OrganizationID, Settings: member.Settings, CollectionIDs: []uuid.UUID{}}
		for _, collection := range member.Edges.Collections {
			value.CollectionIDs = append(value.CollectionIDs, collection.ID)
		}
		members[i] = value
	}
	templateRows, err := client.UnitTemplate.Query().Where(unittemplate.OrganizationIDEQ(id)).
		Order(ent.Asc(unittemplate.FieldID)).ForUpdate().All(scope)
	if err != nil {
		return units.Organization{}, nil, nil, err
	}
	templates := make([]units.UnitTemplate, len(templateRows))
	for i, template := range templateRows {
		templates[i] = templateDTO(template)
	}
	return units.Organization{ID: row.ID, Revision: row.Revision, Name: row.Name,
		Defaults: row.Defaults, Deleted: row.Deleted}, members, templates, nil
}

// ReadOrganization applies all-current-member authority before returning configuration.
func (s *Store) ReadOrganization(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) (units.Organization, error) {
	predicate, err := organizationPlan(authority, action, now)
	if errors.Is(err, authz.ErrForbidden) {
		return units.Organization{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Organization{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Organization": entql.And(predicate, entql.FieldEQ("id", id), entql.FieldEQ("deleted", false)),
	}, nil)
	if err != nil {
		return units.Organization{}, err
	}
	row, err := client.Organization.Query().Only(scope)
	if ent.IsNotFound(err) {
		return units.Organization{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Organization{}, err
	}
	return units.Organization{ID: row.ID, Revision: row.Revision, Name: row.Name, Defaults: row.Defaults}, nil
}

// ListOrganizations filters complete current member scope before count and pagination.
func (s *Store) ListOrganizations(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, page units.PageInput, now time.Time) (units.Page[units.Organization], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[units.Organization]{}, err
	}
	predicate, err := organizationPlan(authority, "organization.read", now)
	if err != nil {
		return units.Page[units.Organization]{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Organization": entql.And(predicate, entql.FieldEQ("deleted", false)),
	}, nil)
	if err != nil {
		return units.Page[units.Organization]{}, err
	}
	total, err := client.Organization.Query().Count(scope)
	if err != nil {
		return units.Page[units.Organization]{}, err
	}
	rows, err := client.Organization.Query().Order(ent.Asc(organization.FieldID)).
		Offset(page.Offset).Limit(page.Limit).All(scope)
	if err != nil {
		return units.Page[units.Organization]{}, err
	}
	result := units.Page[units.Organization]{Items: make([]units.Organization, len(rows)), Total: total}
	for i, row := range rows {
		result.Items[i] = units.Organization{ID: row.ID, Revision: row.Revision, Name: row.Name, Defaults: row.Defaults}
	}
	return result, nil
}

// SaveOrganization creates or revision-checks an authorized complete replacement.
func (s *Store) SaveOrganization(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value units.Organization, expected int64, now time.Time) error {
	if value.ID == uuid.Nil || value.Revision != expected+1 || expected < 0 {
		return authz.ErrInvalidInput
	}
	if expected == 0 {
		client, scope, err := s.client(ctx, tx, nil, map[string]bool{"Organization": true})
		if err != nil {
			return err
		}
		return client.Organization.Create().SetID(value.ID).SetRevision(1).SetName(value.Name).
			SetDefaults(value.Defaults).Exec(scope)
	}
	predicate, err := organizationPlan(authority, "organization.update", now)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Organization": entql.And(predicate, entql.FieldEQ("id", value.ID),
			entql.FieldEQ("revision", expected), entql.FieldEQ("deleted", false)),
	}, nil)
	if err != nil {
		return err
	}
	if _, err := client.Organization.Query().Where(organization.IDEQ(value.ID),
		organization.RevisionEQ(expected), organization.DeletedEQ(false)).ForUpdate().Only(scope); ent.IsNotFound(err) {
		return authz.ErrRevisionConflict
	} else if err != nil {
		return err
	}
	updated, err := client.Organization.Update().SetName(value.Name).SetDefaults(value.Defaults).
		SetRevision(value.Revision).Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// DeleteOrganization preserves a revisioned tombstone after references are detached.
func (s *Store) DeleteOrganization(ctx context.Context, tx platform.TransactionContext, _ authz.Authority, value units.Organization, expected int64, _ time.Time) error {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Organization": entql.And(entql.FieldEQ("id", value.ID),
			entql.FieldEQ("revision", expected), entql.FieldEQ("deleted", false)),
	}, nil)
	if err != nil {
		return err
	}
	if _, err := client.Organization.Query().Where(organization.IDEQ(value.ID),
		organization.RevisionEQ(expected), organization.DeletedEQ(false)).ForUpdate().Only(scope); ent.IsNotFound(err) {
		return authz.ErrRevisionConflict
	} else if err != nil {
		return err
	}
	updated, err := client.Organization.Update().SetDeleted(true).SetRevision(value.Revision).Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// ReadUnitTemplate reads a current template and resolves its current Organization defaults.
func (s *Store) ReadUnitTemplate(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (units.UnitTemplate, error) {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"UnitTemplate": entql.And(entql.FieldEQ("id", id), entql.FieldEQ("deleted", false)),
		"Organization": entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return units.UnitTemplate{}, err
	}
	row, err := client.UnitTemplate.Query().WithOrganization(func(query *ent.OrganizationQuery) {
		query.Select(organization.FieldDefaults)
	}).Only(scope)
	if ent.IsNotFound(err) {
		return units.UnitTemplate{}, authz.ErrNotFound
	}
	if err != nil {
		return units.UnitTemplate{}, err
	}
	return templateDTO(row), nil
}

// ListUnitTemplates returns only current templates; authorization is all-scope at the service.
func (s *Store) ListUnitTemplates(ctx context.Context, tx platform.TransactionContext, page units.PageInput) (units.Page[units.UnitTemplate], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[units.UnitTemplate]{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"UnitTemplate": entql.FieldEQ("deleted", false), "Organization": entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return units.Page[units.UnitTemplate]{}, err
	}
	total, err := client.UnitTemplate.Query().Count(scope)
	if err != nil {
		return units.Page[units.UnitTemplate]{}, err
	}
	rows, err := client.UnitTemplate.Query().Order(ent.Asc(unittemplate.FieldID)).
		Offset(page.Offset).Limit(page.Limit).WithOrganization(func(query *ent.OrganizationQuery) {
		query.Select(organization.FieldDefaults)
	}).All(scope)
	if err != nil {
		return units.Page[units.UnitTemplate]{}, err
	}
	result := units.Page[units.UnitTemplate]{Items: make([]units.UnitTemplate, len(rows)), Total: total}
	for i, row := range rows {
		result.Items[i] = templateDTO(row)
	}
	return result, nil
}

// SaveUnitTemplate creates or updates a standalone template under revision control.
func (s *Store) SaveUnitTemplate(ctx context.Context, tx platform.TransactionContext, value units.UnitTemplate, expected int64) error {
	if value.ID == uuid.Nil || expected < 0 || value.Revision != expected+1 {
		return authz.ErrInvalidInput
	}
	predicates := map[string]entql.P{
		"UnitTemplate": entql.FieldEQ("id", value.ID),
		"Organization": entql.FieldNotNil("id"),
	}
	client, scope, err := s.client(ctx, tx, predicates, map[string]bool{"UnitTemplate": expected == 0})
	if err != nil {
		return err
	}
	if expected == 0 {
		if err := s.lockOrganizations(ctx, tx, value.OrganizationID); err != nil {
			return err
		}
		create := client.UnitTemplate.Create().SetID(value.ID).SetRevision(1).SetName(value.Name).
			SetSettings(value.Settings)
		if value.OrganizationID != nil {
			create.SetOrganizationID(*value.OrganizationID)
		}
		return create.Exec(scope)
	}
	currentClient, currentScope, err := s.client(ctx, tx, predicates, nil)
	if err != nil {
		return err
	}
	current, err := currentClient.UnitTemplate.Query().Select(unittemplate.FieldOrganizationID).
		Where(unittemplate.IDEQ(value.ID), unittemplate.RevisionEQ(expected), unittemplate.DeletedEQ(false)).
		Only(currentScope)
	if ent.IsNotFound(err) {
		return authz.ErrRevisionConflict
	}
	if err != nil {
		return err
	}
	if err := s.lockOrganizations(ctx, tx, current.OrganizationID, value.OrganizationID); err != nil {
		return err
	}
	update := client.UnitTemplate.Update().Where(unittemplate.IDEQ(value.ID),
		unittemplate.RevisionEQ(expected), unittemplate.DeletedEQ(false)).
		SetName(value.Name).SetSettings(value.Settings).SetRevision(value.Revision)
	if value.OrganizationID == nil {
		update.ClearOrganizationID()
	} else {
		update.SetOrganizationID(*value.OrganizationID)
	}
	if value.Deleted {
		update.SetDeleted(true)
	}
	updated, err := update.Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// collectionDTO omits member enumeration and unrelated owner state.
func collectionDTO(row *ent.UnitCollection) units.Collection {
	return units.Collection{ID: row.ID, Revision: row.Revision, Name: row.Name}
}

// withCollections fetches only membership references for the already filtered Unit rows.
func withCollections(query *ent.UnitCollectionQuery) {
	query.Select(unitcollection.FieldID).Order(ent.Asc(unitcollection.FieldID))
}

// ReadUnitContext is the internal current-context port, never a public read bypass.
func (s *Store) ReadUnitContext(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (units.Context, error) {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Unit":           entql.FieldEQ("id", id),
		"UnitCollection": entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return units.Context{}, err
	}
	row, err := client.Unit.Query().Select(unit.FieldID, unit.FieldRevision, unit.FieldAgencyID, unit.FieldDeleted).
		WithCollections(withCollections).Only(scope)
	if ent.IsNotFound(err) {
		return units.Context{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Context{}, err
	}
	result := units.Context{ID: row.ID, Revision: row.Revision, AgencyID: row.AgencyID, Deleted: row.Deleted, CollectionIDs: []uuid.UUID{}}
	for _, collection := range row.Edges.Collections {
		result.CollectionIDs = append(result.CollectionIDs, collection.ID)
	}
	return result, nil
}

// ReadUnit applies the exact action scope before selecting a public or mutation target.
func (s *Store) ReadUnit(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) (units.Unit, error) {
	predicate, err := plan(authority, action, false, now)
	if errors.Is(err, authz.ErrForbidden) {
		return units.Unit{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Unit{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Unit":           entql.And(predicate, entql.FieldEQ("id", id), entql.FieldEQ("deleted", false)),
		"UnitCollection": entql.FieldNotNil("id"),
		"Organization":   entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return units.Unit{}, err
	}
	row, err := client.Unit.Query().WithCollections(withCollections).WithOrganization(func(query *ent.OrganizationQuery) {
		query.Select(organization.FieldDefaults)
	}).Only(scope)
	if ent.IsNotFound(err) {
		return units.Unit{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Unit{}, err
	}
	return unitDTO(row), nil
}

// LockUnits applies authorization before taking stable-order row locks for bulk changes.
func (s *Store) LockUnits(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, ids []uuid.UUID, now time.Time) ([]units.Unit, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, authz.ErrInvalidInput
	}
	ordered := slices.Clone(ids)
	slices.SortFunc(ordered, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	values := make([]any, len(ordered))
	for i, id := range ordered {
		if id == uuid.Nil || (i > 0 && id == ordered[i-1]) {
			return nil, authz.ErrInvalidInput
		}
		values[i] = id
	}
	predicate, err := plan(authority, action, false, now)
	if errors.Is(err, authz.ErrForbidden) {
		return nil, authz.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Unit":           entql.And(predicate, entql.FieldIn("id", values...), entql.FieldEQ("deleted", false)),
		"UnitCollection": entql.FieldNotNil("id"),
		"Organization":   entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return nil, err
	}
	rows, err := client.Unit.Query().Where(unit.IDIn(ordered...)).Order(ent.Asc(unit.FieldID)).ForUpdate().
		WithCollections(withCollections).WithOrganization(func(query *ent.OrganizationQuery) {
		query.Select(organization.FieldDefaults)
	}).All(scope)
	if err != nil {
		return nil, err
	}
	if len(rows) != len(ordered) {
		return nil, authz.ErrNotFound
	}
	result := make([]units.Unit, len(rows))
	for i, row := range rows {
		result[i] = unitDTO(row)
	}
	return result, nil
}

// ListUnits filters rows and fields before the count, stable order and bounded page.
func (s *Store) ListUnits(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, page units.PageInput, now time.Time) (units.Page[units.Unit], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[units.Unit]{}, err
	}
	predicate, err := plan(authority, "unit.read", false, now)
	if err != nil {
		return units.Page[units.Unit]{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Unit":           entql.And(predicate, entql.FieldEQ("deleted", false)),
		"UnitCollection": entql.FieldNotNil("id"), "Organization": entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return units.Page[units.Unit]{}, err
	}
	total, err := client.Unit.Query().Count(scope)
	if err != nil {
		return units.Page[units.Unit]{}, err
	}
	rows, err := client.Unit.Query().Order(ent.Asc(unit.FieldID)).Offset(page.Offset).Limit(page.Limit).
		WithCollections(withCollections).WithOrganization(func(query *ent.OrganizationQuery) {
		query.Select(organization.FieldDefaults)
	}).All(scope)
	if err != nil {
		return units.Page[units.Unit]{}, err
	}
	result := units.Page[units.Unit]{Items: make([]units.Unit, len(rows)), Total: total}
	for i, row := range rows {
		result.Items[i] = unitDTO(row)
	}
	return result, nil
}

// SaveUnit mutates only supported columns/membership, preserving future owner configuration.
func (s *Store) SaveUnit(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value units.Unit, expected int64, now time.Time) error {
	if expected > 0 {
		if _, err := commands.NextRevision(expected, expected); err != nil {
			return err
		}
	}
	if value.ID == uuid.Nil || expected < 0 || value.Revision != expected+1 {
		return authz.ErrInvalidInput
	}
	if expected == 0 {
		if err := s.lockOrganizations(ctx, tx, value.OrganizationID); err != nil {
			return err
		}
		client, scope, err := s.client(ctx, tx, nil, map[string]bool{"Unit": true})
		if err != nil {
			return err
		}
		create := client.Unit.Create().SetID(value.ID).SetRevision(1).SetName(value.Name).SetAgencyID(value.AgencyID).
			SetSettings(value.Settings).AddCollectionIDs(value.CollectionIDs...)
		if value.OrganizationID != nil {
			create.SetOrganizationID(*value.OrganizationID)
		}
		return create.Exec(scope)
	}
	action := "unit.update"
	if value.Deleted {
		action = "unit.delete"
	}
	predicate, err := plan(authority, action, false, now)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"Unit": entql.And(predicate,
		entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", expected), entql.FieldEQ("deleted", false))}, nil)
	if err != nil {
		return err
	}
	currentClient, currentScope, err := s.client(ctx, tx, map[string]entql.P{
		"Unit": entql.And(predicate, entql.FieldEQ("id", value.ID), entql.FieldEQ("deleted", false)),
	}, nil)
	if err != nil {
		return err
	}
	current, err := currentClient.Unit.Query().Select(unit.FieldOrganizationID).Only(currentScope)
	if ent.IsNotFound(err) {
		return authz.ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.lockOrganizations(ctx, tx, current.OrganizationID, value.OrganizationID); err != nil {
		return err
	}
	update := client.Unit.Update().SetName(value.Name).SetAgencyID(value.AgencyID).SetRevision(value.Revision).
		SetDeleted(value.Deleted).SetSettings(value.Settings).ClearCollections().AddCollectionIDs(value.CollectionIDs...)
	if value.OrganizationID == nil {
		update.ClearOrganizationID()
	} else {
		update.SetOrganizationID(*value.OrganizationID)
	}
	updated, err := update.Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// ReadCollection protects the named configuration without revealing hidden members.
func (s *Store) ReadCollection(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) (units.Collection, error) {
	predicate, err := plan(authority, action, true, now)
	if errors.Is(err, authz.ErrForbidden) {
		return units.Collection{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Collection{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"UnitCollection": entql.And(predicate, entql.FieldEQ("id", id)),
	}, nil)
	if err != nil {
		return units.Collection{}, err
	}
	row, err := client.UnitCollection.Query().Only(scope)
	if ent.IsNotFound(err) {
		return units.Collection{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Collection{}, err
	}
	return collectionDTO(row), nil
}

// ListCollections applies correlated owner predicates before count/page.
func (s *Store) ListCollections(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, page units.PageInput, now time.Time) (units.Page[units.Collection], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[units.Collection]{}, err
	}
	predicate, err := plan(authority, "unit-collection.read", true, now)
	if err != nil {
		return units.Page[units.Collection]{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"UnitCollection": predicate}, nil)
	if err != nil {
		return units.Page[units.Collection]{}, err
	}
	total, err := client.UnitCollection.Query().Count(scope)
	if err != nil {
		return units.Page[units.Collection]{}, err
	}
	rows, err := client.UnitCollection.Query().Order(ent.Asc(unitcollection.FieldID)).Offset(page.Offset).Limit(page.Limit).All(scope)
	if err != nil {
		return units.Page[units.Collection]{}, err
	}
	result := units.Page[units.Collection]{Items: make([]units.Collection, len(rows)), Total: total}
	for i, row := range rows {
		result.Items[i] = collectionDTO(row)
	}
	return result, nil
}

// SaveCollection creates or updates a named collection under current native scope/revision.
func (s *Store) SaveCollection(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value units.Collection, expected int64, now time.Time) error {
	if expected > 0 {
		if _, err := commands.NextRevision(expected, expected); err != nil {
			return err
		}
	}
	if value.ID == uuid.Nil || expected < 0 || value.Revision != expected+1 {
		return authz.ErrInvalidInput
	}
	if expected == 0 {
		client, scope, err := s.client(ctx, tx, nil, map[string]bool{"UnitCollection": true})
		if err != nil {
			return err
		}
		return client.UnitCollection.Create().SetID(value.ID).SetName(value.Name).SetRevision(1).Exec(scope)
	}
	predicate, err := plan(authority, "unit-collection.update", true, now)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"UnitCollection": entql.And(predicate,
		entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", expected))}, nil)
	if err != nil {
		return err
	}
	updated, err := client.UnitCollection.Update().SetName(value.Name).SetRevision(value.Revision).Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// CollectionUnits walks bounded internal pages for all-use checks without public enumeration.
func (s *Store) CollectionUnits(ctx context.Context, tx platform.TransactionContext, collection, after uuid.UUID) ([]units.Unit, error) {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Unit": entql.HasEdgeWith("collections", entql.FieldEQ("id", collection)), "UnitCollection": entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return nil, err
	}
	query := client.Unit.Query().Order(ent.Asc(unit.FieldID)).Limit(200)
	if after != uuid.Nil {
		query.Where(unit.IDGT(after))
	}
	rows, err := query.WithCollections(withCollections).All(scope)
	if err != nil {
		return nil, err
	}
	result := make([]units.Unit, len(rows))
	for i, row := range rows {
		result[i] = unitDTO(row)
	}
	return result, nil
}

// DeleteCollection rechecks native member scope, advances member revisions and detaches atomically.
func (s *Store) DeleteCollection(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value units.Collection, now time.Time) error {
	collectionPredicate, err := plan(authority, "unit-collection.delete", true, now)
	if err != nil {
		return err
	}
	unitPredicate, err := plan(authority, "unit-collection.delete", false, now)
	if err != nil {
		return err
	}
	membership := entql.HasEdgeWith("collections", entql.FieldEQ("id", value.ID))
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Unit":           membership,
		"UnitCollection": entql.And(collectionPredicate, entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", value.Revision)),
	}, nil)
	if err != nil {
		return err
	}
	exists, err := client.UnitCollection.Query().Exist(scope)
	if err != nil {
		return err
	}
	if !exists {
		return authz.ErrRevisionConflict
	}
	total, err := client.Unit.Query().Count(scope)
	if err != nil {
		return err
	}
	overflow, err := client.Unit.Query().Where(unit.RevisionEQ(math.MaxInt64)).Exist(scope)
	if err != nil {
		return err
	}
	if overflow {
		return platform.NewFailure(platform.CodeResourceLimit, errors.New("member Unit revision exhausted"))
	}
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return err
	}
	authorized := guard.Bind(scope, guard.Access{Current: driver.Current, Predicates: map[string]entql.P{
		"Unit":           entql.And(membership, unitPredicate, entql.FieldLT("revision", int64(math.MaxInt64))),
		"UnitCollection": entql.And(entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", value.Revision)),
	}})
	updated, err := client.Unit.Update().AddRevision(1).RemoveCollectionIDs(value.ID).Save(authorized)
	if err != nil {
		return err
	}
	if updated != total {
		return authz.ErrRevisionConflict
	}
	deleted, err := client.UnitCollection.Delete().Exec(authorized)
	if err != nil {
		return err
	}
	if deleted != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// RecordChange advances shared access/configuration authority and appends safe audit metadata.
func (s *Store) RecordChange(ctx context.Context, tx platform.TransactionContext, actor, target uuid.UUID, action string, now time.Time) error {
	owner, err := identitystore.New(s.primary)
	if err != nil {
		return err
	}
	return owner.RecordConfigurationChange(ctx, tx, actor, target, action, now)
}

var _ units.Repository = (*Store)(nil)
