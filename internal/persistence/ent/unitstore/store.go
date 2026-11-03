// Package unitstore implements the Unit owner's ports with native Ent row filtering.
package unitstore

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
	"github.com/gopager/gopager/internal/persistence/ent/unit"
	"github.com/gopager/gopager/internal/persistence/ent/unitcollection"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

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
	case "network":
		return entql.HasEdgeWith("address_rules", entql.FieldIn("network_id", values...)), nil
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
	case "network":
		return entql.HasEdgeWith("units", entql.And(entql.FieldEQ("deleted", false),
			entql.HasEdgeWith("address_rules", entql.FieldIn("network_id", values...)))), nil
	default:
		return nil, authz.ErrInvalidInput
	}
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

// unitDTO projects the supported public configuration without exposing Ent entities.
func unitDTO(row *ent.Unit) units.Unit {
	result := units.Unit{ID: row.ID, Revision: row.Revision, Name: row.Name, Deleted: row.Deleted, CollectionIDs: []uuid.UUID{}}
	for _, collection := range row.Edges.Collections {
		result.CollectionIDs = append(result.CollectionIDs, collection.ID)
	}
	return result
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
	row, err := client.Unit.Query().Select(unit.FieldID, unit.FieldRevision, unit.FieldDeleted).WithCollections(withCollections).Only(scope)
	if ent.IsNotFound(err) {
		return units.Context{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Context{}, err
	}
	result := units.Context{ID: row.ID, Revision: row.Revision, Deleted: row.Deleted, CollectionIDs: []uuid.UUID{}}
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
	}, nil)
	if err != nil {
		return units.Unit{}, err
	}
	row, err := client.Unit.Query().WithCollections(withCollections).Only(scope)
	if ent.IsNotFound(err) {
		return units.Unit{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Unit{}, err
	}
	return unitDTO(row), nil
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
		"Unit": entql.And(predicate, entql.FieldEQ("deleted", false)), "UnitCollection": entql.FieldNotNil("id"),
	}, nil)
	if err != nil {
		return units.Page[units.Unit]{}, err
	}
	total, err := client.Unit.Query().Count(scope)
	if err != nil {
		return units.Page[units.Unit]{}, err
	}
	rows, err := client.Unit.Query().Order(ent.Asc(unit.FieldID)).Offset(page.Offset).Limit(page.Limit).WithCollections(withCollections).All(scope)
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
		client, scope, err := s.client(ctx, tx, nil, map[string]bool{"Unit": true})
		if err != nil {
			return err
		}
		return client.Unit.Create().SetID(value.ID).SetRevision(1).SetName(value.Name).AddCollectionIDs(value.CollectionIDs...).Exec(scope)
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
	updated, err := client.Unit.Update().SetName(value.Name).SetRevision(value.Revision).SetDeleted(value.Deleted).
		ClearCollections().AddCollectionIDs(value.CollectionIDs...).Save(scope)
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

// ScopeExists checks every restricting reference against its current owning entity.
func (s *Store) ScopeExists(ctx context.Context, tx platform.TransactionContext, target authz.Scope) (bool, error) {
	if !authz.ValidScope(target) || (target.Kind != "unit" && target.Kind != "unitCollection") {
		return false, authz.ErrInvalidInput
	}
	values := make([]any, 0, len(target.References))
	seen := make(map[uuid.UUID]bool, len(target.References))
	for _, reference := range target.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return false, authz.ErrInvalidInput
		}
		if !seen[id] {
			seen[id] = true
			values = append(values, id)
		}
	}
	predicate := entql.FieldIn("id", values...)
	entity := "UnitCollection"
	if target.Kind == "unit" {
		entity = "Unit"
		predicate = entql.And(predicate, entql.FieldEQ("deleted", false))
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{entity: predicate}, nil)
	if err != nil {
		return false, err
	}
	var count int
	if entity == "Unit" {
		count, err = client.Unit.Query().Count(scope)
	} else {
		count, err = client.UnitCollection.Query().Count(scope)
	}
	return count == len(values), err
}

var _ units.Repository = (*Store)(nil)
