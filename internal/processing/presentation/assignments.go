package presentation

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/units"
)

// The scope kinds a Layout can be assigned to.
const (
	// ScopeUnit assigns a Layout to one Unit.
	ScopeUnit = "unit"
	// ScopeOrganization assigns a Layout to an Organization, and so to its Units.
	ScopeOrganization = "organization"
)

// Assignment makes a Layout the one a Unit or Organization renders with for an output kind.
type Assignment struct {
	ScopeKind  string    `json:"scopeKind"`
	ScopeID    uuid.UUID `json:"scopeId"`
	OutputKind string    `json:"outputKind"`
	LayoutID   uuid.UUID `json:"layoutId"`
	Revision   int64     `json:"revision,string"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// UnitContext is what authorization needs to know about a Unit.
type UnitContext struct {
	AgencyID      uuid.UUID
	CollectionIDs []uuid.UUID
}

// AssignmentStore is the persistence port of layout assignments. Every method runs in the
// caller's transaction.
type AssignmentStore interface {
	// List returns the assignments of the scope kind and scope ID (either may be empty or
	// uuid.Nil to not filter), ordered by scope kind, scope ID and output kind.
	List(ctx context.Context, tx platform.TransactionContext, scopeKind string, scopeID uuid.UUID) ([]Assignment, error)
	// Get returns one assignment, or authz.ErrNotFound.
	Get(ctx context.Context, tx platform.TransactionContext, scopeKind string, scopeID uuid.UUID, outputKind string) (Assignment, error)
	// Put stores the assignment: expected 0 inserts and expected > 0 replaces at that revision.
	// A stale revision, or an insert where one exists, is authz.ErrRevisionConflict. A Layout
	// that does not exist is authz.InvalidField("layoutId"); it stays until the transaction ends.
	Put(ctx context.Context, tx platform.TransactionContext, assignment Assignment, expected int64) error
	// Delete removes the assignment at the expected revision, or fails with
	// authz.ErrRevisionConflict (stale) or authz.ErrNotFound (absent).
	Delete(ctx context.Context, tx platform.TransactionContext, scopeKind string, scopeID uuid.UUID, outputKind string, expected int64) error
	// ReadUnit returns the Agency and collections of a Unit, or authz.ErrNotFound when the
	// Unit does not exist or is deleted. It holds the Unit against deletion until the caller's
	// transaction ends, so an assignment written in it never outlives its Unit.
	ReadUnit(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (UnitContext, error)
	// OrganizationExists reports whether the Organization exists and is not deleted, holding it
	// against deletion until the caller's transaction ends like ReadUnit.
	OrganizationExists(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (bool, error)
}

// AssignmentService manages layout assignments. Create it with NewAssignmentService; it is
// safe for concurrent use.
type AssignmentService struct {
	callerScope
	store   AssignmentStore
	layouts Store
}

// NewAssignmentService composes the service. layouts supplies the Layouts that are assigned.
// It fails with authz.ErrInvalidInput on a missing dependency.
func NewAssignmentService(transactions identity.Transactions, resolver identity.AuthorityResolver, clock platform.Clock,
	store AssignmentStore, layouts Store) (*AssignmentService, error) {
	if transactions == nil || resolver == nil || clock == nil || store == nil || layouts == nil {
		return nil, authz.ErrInvalidInput
	}
	return &AssignmentService{callerScope: callerScope{transactions: transactions, resolver: resolver, clock: clock},
		store: store, layouts: layouts}, nil
}

// validateKey checks the scope kind, scope ID and output kind of a path.
func validateKey(scopeKind string, scopeID uuid.UUID, outputKind string) error {
	if scopeKind != ScopeUnit && scopeKind != ScopeOrganization {
		return authz.InvalidField("scopeKind")
	}
	if scopeID == uuid.Nil {
		return authz.InvalidField("scopeId")
	}
	if !slices.Contains(processing.LayoutOutputKinds(), outputKind) {
		return authz.InvalidField("outputKind")
	}
	return nil
}

// requireAnyManage rejects a caller that holds presentation.manage nowhere, before any scope is
// looked up, so that a caller without the right learns nothing about Units and Organizations.
func requireAnyManage(who caller) error {
	_, err := authz.VisibleAgencies(who.authority, []string{ManageAction}, false)
	return err
}

// authorizeScope requires the right to assign for the scope: presentation.manage on the Unit,
// or presentation.manage in every Agency of the Layout for an Organization. A Unit or
// Organization that does not exist is authz.ErrNotFound. layoutAgencies are the Agencies of the
// Layout whose assignment is written, replaced or removed.
func (s *AssignmentService) authorizeScope(ctx context.Context, tx platform.TransactionContext, who caller, now time.Time,
	scopeKind string, scopeID uuid.UUID, layoutAgencies []uuid.UUID) (unitAgency uuid.UUID, err error) {
	if scopeKind == ScopeOrganization {
		exists, err := s.store.OrganizationExists(ctx, tx, scopeID)
		if err != nil {
			return uuid.Nil, err
		}
		if !exists {
			return uuid.Nil, authz.ErrNotFound
		}
		return uuid.Nil, requireManage(who.authority, now, layoutAgencies)
	}
	unit, err := s.store.ReadUnit(ctx, tx, scopeID)
	if err != nil {
		return uuid.Nil, err
	}
	target := authz.Target{AgencyID: unit.AgencyID, UnitID: scopeID, UnitCollections: unit.CollectionIDs}
	return unit.AgencyID, authz.Authorize(who.authority, ManageAction, nil, target, now)
}

// List returns the assignments the caller may manage, optionally for one scope kind and scope ID,
// ordered by scope kind, scope ID and output kind. It needs presentation.manage.
func (s *AssignmentService) List(ctx context.Context, proof identity.Proof, scopeKind string, scopeID uuid.UUID,
	page units.PageInput) (units.Page[Assignment], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Assignment]{}, err
	}
	if scopeKind != "" && scopeKind != ScopeUnit && scopeKind != ScopeOrganization {
		return units.Page[Assignment]{}, authz.InvalidField("scopeKind")
	}
	result := units.Page[Assignment]{Items: []Assignment{}}
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who caller, now time.Time) error {
		if err := requireAnyManage(who); err != nil {
			return err
		}
		all, err := s.store.List(tx.Context(), tx, scopeKind, scopeID)
		if err != nil {
			return err
		}
		manageable := make([]Assignment, 0, len(all))
		for _, assignment := range all {
			err := s.authorizeAssignment(tx.Context(), tx, who, now, assignment)
			if err == nil {
				manageable = append(manageable, assignment)
			} else if !errors.Is(err, authz.ErrForbidden) && !errors.Is(err, authz.ErrNotFound) {
				return err
			}
		}
		result.Total = len(manageable)
		if page.Offset < len(manageable) {
			result.Items = manageable[page.Offset:min(page.Offset+page.Limit, len(manageable))]
		}
		return nil
	})
	return result, err
}

// authorizeAssignment requires the right to manage an existing assignment.
func (s *AssignmentService) authorizeAssignment(ctx context.Context, tx platform.TransactionContext, who caller, now time.Time,
	assignment Assignment) error {
	layout, err := s.layouts.Get(ctx, tx, assignment.LayoutID)
	if err != nil {
		return err
	}
	_, err = s.authorizeScope(ctx, tx, who, now, assignment.ScopeKind, assignment.ScopeID, layout.Agencies)
	return err
}

// Put assigns the Layout to the scope for the output kind. expected 0 creates the assignment and
// a positive value replaces it at that revision (authz.ErrRevisionConflict otherwise). The Layout
// must exist and have the output kind, and for a Unit it must be assigned to the Unit's Agency
// (authz.InvalidField("layoutId")). A Unit or Organization that does not exist is
// authz.ErrNotFound; the caller needs presentation.manage on the Unit, or in every Agency of the
// Layout for an Organization.
func (s *AssignmentService) Put(ctx context.Context, proof identity.Proof, scopeKind string, scopeID uuid.UUID, outputKind string,
	layoutID uuid.UUID, expected int64, check func(identity.Authentication) error) (Assignment, error) {
	if err := validateKey(scopeKind, scopeID, outputKind); err != nil {
		return Assignment{}, err
	}
	if layoutID == uuid.Nil {
		return Assignment{}, authz.InvalidField("layoutId")
	}
	if expected < 0 {
		return Assignment{}, authz.InvalidField("expectedRevision")
	}
	var result Assignment
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		if err := requireAnyManage(who); err != nil {
			return err
		}
		layout, err := s.layouts.Get(tx.Context(), tx, layoutID)
		if errors.Is(err, authz.ErrNotFound) {
			return authz.InvalidField("layoutId")
		}
		if err != nil {
			return err
		}
		unitAgency, err := s.authorizeScope(tx.Context(), tx, who, now, scopeKind, scopeID, layout.Agencies)
		if err != nil {
			return err
		}
		if layout.OutputKind != outputKind || (scopeKind == ScopeUnit && !slices.Contains(layout.Agencies, unitAgency)) {
			return authz.InvalidField("layoutId")
		}
		next := expected + 1
		result = Assignment{ScopeKind: scopeKind, ScopeID: scopeID, OutputKind: outputKind, LayoutID: layoutID,
			Revision: next, UpdatedAt: now}
		if expected > 0 {
			// Replacing an assignment of another Layout needs the right over that Layout too.
			current, err := s.store.Get(tx.Context(), tx, scopeKind, scopeID, outputKind)
			if err != nil {
				return authz.ErrRevisionConflict
			}
			if err := s.authorizeAssignment(tx.Context(), tx, who, now, current); err != nil {
				return err
			}
			if _, err := commands.NextRevision(current.Revision, expected); err != nil {
				return err
			}
		}
		return s.store.Put(tx.Context(), tx, result, expected)
	})
	return result, err
}

// Delete removes the assignment at the expected revision and returns the scope ID with the
// revision it reached. An absent assignment is authz.ErrNotFound, a stale revision
// authz.ErrRevisionConflict. The caller needs the right Put requires for the assigned Layout.
func (s *AssignmentService) Delete(ctx context.Context, proof identity.Proof, scopeKind string, scopeID uuid.UUID, outputKind string,
	expected int64, check func(identity.Authentication) error) (commands.Result, error) {
	if err := validateKey(scopeKind, scopeID, outputKind); err != nil {
		return commands.Result{}, err
	}
	if expected <= 0 {
		return commands.Result{}, authz.InvalidField("expectedRevision")
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		if err := requireAnyManage(who); err != nil {
			return err
		}
		current, err := s.store.Get(tx.Context(), tx, scopeKind, scopeID, outputKind)
		if err != nil {
			return err
		}
		if err := s.authorizeAssignment(tx.Context(), tx, who, now, current); err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.store.Delete(tx.Context(), tx, scopeKind, scopeID, outputKind, expected); err != nil {
			return err
		}
		result = commands.Result{ID: scopeID, Revision: next}
		return nil
	})
	return result, err
}
