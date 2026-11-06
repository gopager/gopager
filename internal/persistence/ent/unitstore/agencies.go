package unitstore

import (
	"context"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/agency"
	"github.com/gopager/gopager/internal/persistence/ent/network"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
	"github.com/gopager/gopager/internal/persistence/ent/unit"
	"github.com/gopager/gopager/internal/platform"
)

func agencyDTO(row *ent.Agency) units.Agency {
	return units.Agency{ID: row.ID, Revision: row.Revision, Name: row.Name}
}

// agencyPlan binds the caller's grants for an Agency action to Agency rows.
func agencyPlan(authority authz.Authority, action string, now time.Time) (entql.P, error) {
	result, err := permissions.ReadScope(authority, action, nil, permissions.Attributes{Scope: agencyScope}, now)
	if err != nil {
		return nil, err
	}
	return result.Predicate, nil
}

// ReadAgency returns the Agency when the caller's action scope covers it; otherwise authz.ErrNotFound.
func (s *Store) ReadAgency(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) (units.Agency, error) {
	predicate, err := agencyPlan(authority, action, now)
	if err != nil {
		return units.Agency{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"Agency": entql.And(predicate, entql.FieldEQ("id", id))}, nil)
	if err != nil {
		return units.Agency{}, err
	}
	row, err := client.Agency.Query().Only(scope)
	if ent.IsNotFound(err) {
		return units.Agency{}, authz.ErrNotFound
	}
	if err != nil {
		return units.Agency{}, err
	}
	return agencyDTO(row), nil
}

// ListAgencies pages the Agencies the caller may read, ordered by ID.
func (s *Store) ListAgencies(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, page units.PageInput, now time.Time) (units.Page[units.Agency], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[units.Agency]{}, err
	}
	predicate, err := agencyPlan(authority, "agency.read", now)
	if err != nil {
		return units.Page[units.Agency]{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"Agency": predicate}, nil)
	if err != nil {
		return units.Page[units.Agency]{}, err
	}
	total, err := client.Agency.Query().Count(scope)
	if err != nil {
		return units.Page[units.Agency]{}, err
	}
	rows, err := client.Agency.Query().Order(ent.Asc(agency.FieldID)).Offset(page.Offset).Limit(page.Limit).All(scope)
	if err != nil {
		return units.Page[units.Agency]{}, err
	}
	result := units.Page[units.Agency]{Items: make([]units.Agency, len(rows)), Total: total}
	for i, row := range rows {
		result.Items[i] = agencyDTO(row)
	}
	return result, nil
}

// SaveAgency creates the Agency (expected 0) or renames it at the expected revision under the
// caller's agency.update scope.
func (s *Store) SaveAgency(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value units.Agency, expected int64, now time.Time) error {
	if expected > 0 {
		if _, err := commands.NextRevision(expected, expected); err != nil {
			return err
		}
	}
	if value.ID == uuid.Nil || expected < 0 || value.Revision != expected+1 {
		return authz.ErrInvalidInput
	}
	if expected == 0 {
		client, scope, err := s.client(ctx, tx, nil, map[string]bool{"Agency": true})
		if err != nil {
			return err
		}
		return client.Agency.Create().SetID(value.ID).SetRevision(1).SetName(value.Name).Exec(scope)
	}
	predicate, err := agencyPlan(authority, "agency.update", now)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"Agency": entql.And(predicate,
		entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", expected))}, nil)
	if err != nil {
		return err
	}
	updated, err := client.Agency.Update().SetName(value.Name).SetRevision(value.Revision).Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// DeleteAgency removes an Agency at its current revision; one that still owns Units (deleted Units
// included, since their history references it) or Radio networks fails with units.ErrAgencyInUse.
func (s *Store) DeleteAgency(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value units.Agency, now time.Time) error {
	predicate, err := agencyPlan(authority, "agency.delete", now)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Agency":  entql.And(predicate, entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", value.Revision)),
		"Unit":    entql.FieldEQ("agency_id", value.ID),
		"Network": entql.FieldEQ("agency_id", value.ID),
	}, nil)
	if err != nil {
		return err
	}
	if _, err := client.Agency.Query().Where(agency.ID(value.ID)).ForUpdate().Only(scope); ent.IsNotFound(err) {
		return authz.ErrRevisionConflict
	} else if err != nil {
		return err
	}
	for _, exist := range []func() (bool, error){
		func() (bool, error) { return client.Unit.Query().Where(unit.AgencyID(value.ID)).Exist(scope) },
		func() (bool, error) { return client.Network.Query().Where(network.AgencyID(value.ID)).Exist(scope) },
	} {
		inUse, err := exist()
		if err != nil {
			return err
		}
		if inUse {
			return units.ErrAgencyInUse
		}
	}
	deleted, err := client.Agency.Delete().Where(agency.ID(value.ID), agency.Revision(value.Revision)).Exec(scope)
	if err != nil {
		return err
	}
	if deleted != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// LockAgency holds the Agency against deletion for the rest of the transaction, so a Unit saved into
// it cannot lose its Agency; an unknown Agency is authz.ErrInvalidInput.
func (s *Store) LockAgency(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) error {
	if id == uuid.Nil {
		return authz.ErrInvalidInput
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"Agency": entql.FieldEQ("id", id)}, nil)
	if err != nil {
		return err
	}
	exists, err := client.Agency.Query().Where(agency.ID(id)).ForShare().Exist(scope)
	if err != nil {
		return err
	}
	if !exists {
		return authz.ErrInvalidInput
	}
	return nil
}
