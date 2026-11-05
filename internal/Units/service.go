package units

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

type Service struct {
	transactions identity.Transactions
	resolver     *identity.Resolver
	clock        platform.Clock
	repository   Repository
	rules        RuleOwner
	commands     commands.Repository
	facts        events.MasterDataAppender
}

// NewService composes only the approved early Unit/collection operations.
func NewService(transactions identity.Transactions, resolver *identity.Resolver, clock platform.Clock, repository Repository, rules RuleOwner, receipts commands.Repository, facts events.MasterDataAppender) (*Service, error) {
	if transactions == nil || resolver == nil || clock == nil || repository == nil || rules == nil || receipts == nil || facts == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Unit service dependency"))
	}
	return &Service{transactions: transactions, resolver: resolver, clock: clock, repository: repository,
		rules: rules, commands: receipts, facts: facts}, nil
}

// within resolves current authority and performs DTO construction under the same protected transaction.
func (s *Service) within(ctx context.Context, proof identity.Proof, mutation bool, check func(identity.Authentication) error, operation func(platform.TransactionContext, authz.Authority, time.Time) error) error {
	if mutation && check == nil {
		return authz.ErrInvalidInput
	}
	return s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(authentication); err != nil {
				return err
			}
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return operation(tx, authentication.Authority, now)
	})
}

// ReadUnit returns only the named current authorized Unit DTO.
func (s *Service) ReadUnit(ctx context.Context, proof identity.Proof, id uuid.UUID) (Unit, error) {
	if id == uuid.Nil {
		return Unit{}, authz.ErrInvalidInput
	}
	var result Unit
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadUnit(tx.Context(), tx, authority, "unit.read", id, now)
		return err
	})
	if err != nil {
		return Unit{}, err
	}
	return result, nil
}

// ListUnits returns a natively filtered bounded page and its permitted count.
func (s *Service) ListUnits(ctx context.Context, proof identity.Proof, page PageInput) (Page[Unit], error) {
	var result Page[Unit]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListUnits(tx.Context(), tx, authority, page, now)
		return err
	})
	if err != nil {
		return Page[Unit]{}, err
	}
	return result, nil
}

// ReadCollection returns configuration, never an unrestricted member list.
func (s *Service) ReadCollection(ctx context.Context, proof identity.Proof, id uuid.UUID) (Collection, error) {
	if id == uuid.Nil {
		return Collection{}, authz.ErrInvalidInput
	}
	var result Collection
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadCollection(tx.Context(), tx, authority, "unit-collection.read", id, now)
		return err
	})
	if err != nil {
		return Collection{}, err
	}
	return result, nil
}

// ListCollections filters before counting/paging and projects no hidden membership data.
func (s *Service) ListCollections(ctx context.Context, proof identity.Proof, page PageInput) (Page[Collection], error) {
	var result Page[Collection]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListCollections(tx.Context(), tx, authority, page, now)
		return err
	})
	if err != nil {
		return Page[Collection]{}, err
	}
	return result, nil
}

// authorizeUnitUses checks each current Network use independently, without exposing those uses.
func (s *Service) authorizeUnitUses(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, value Unit, now time.Time) error {
	networks, err := s.rules.NetworksForUnit(ctx, tx, value.ID)
	if err != nil {
		return err
	}
	target := authz.Target{UnitID: value.ID, UnitCollections: value.CollectionIDs}
	if len(networks) == 0 {
		return authz.Authorize(authority, action, nil, target, now)
	}
	for _, network := range networks {
		if err := ctx.Err(); err != nil {
			return err
		}
		target.NetworkID = network
		if err := authz.Authorize(authority, action, nil, target, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) authorizeOrganizationReference(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, id uuid.UUID, now time.Time) error {
	_, members, _, err := s.repository.ReadOrganizationContext(ctx, tx, id)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return authz.Authorize(authority, "organization.read", nil, authz.Target{}, now)
	}
	for _, member := range members {
		if err := s.authorizeUnitUses(ctx, tx, authority, "organization.read", member, now); err != nil {
			return authz.ErrForbidden
		}
	}
	return nil
}

// checkMembership checks existing collections and the caller's right to change each affected relation.
func (s *Service) checkMembership(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, before, after []uuid.UUID, now time.Time) error {
	for _, pair := range [][2][]uuid.UUID{{before, after}, {after, before}} {
		for _, id := range pair[0] {
			if slices.Contains(pair[1], id) {
				continue
			}
			if _, err := s.repository.ReadCollection(ctx, tx, authority, "unit-collection.update", id, now); err != nil {
				if errors.Is(err, authz.ErrNotFound) {
					return authz.ErrForbidden
				}
				return err
			}
		}
	}
	return nil
}

// record commits safe audit/current authority and the owner-specific fact with the state.
func (s *Service) record(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, target uuid.UUID, revision int64, kind, change, action string, command *uuid.UUID, now time.Time) error {
	actor, err := uuid.Parse(authority.PrincipalID)
	if err != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	if err := s.repository.RecordChange(ctx, tx, actor, target, action, now); err != nil {
		return err
	}
	_, err = s.facts.AppendMasterDataFact(ctx, tx, events.MasterDataFact{ID: uuid.New(),
		Aggregate: events.EntityRef{Type: kind, ID: target}, Revision: revision, OccurredAt: now, CausationID: command, ChangeKind: change})
	return err
}

// CreateUnit authorizes current context and records a compact actor-bound creation result.
func (s *Service) CreateUnit(ctx context.Context, proof identity.Proof, command uuid.UUID, input UnitInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	hash, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		id := uuid.New()
		if found {
			id = receipt.Result.ID
		}
		if err := authz.Authorize(authority, "unit.create", nil, authz.Target{UnitID: id, UnitCollections: input.CollectionIDs}, now); err != nil {
			return err
		}
		if input.NewOrganization != nil {
			if err := authz.Authorize(authority, "organization.create", nil, authz.Target{}, now); err != nil {
				return err
			}
		}
		if input.OrganizationID != nil {
			if err := s.authorizeOrganizationReference(tx.Context(), tx, authority, *input.OrganizationID, now); err != nil {
				return err
			}
		}
		if err := s.checkMembership(tx.Context(), tx, authority, nil, input.CollectionIDs, now); err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("unit.create", hash)
			return err
		}
		if input.NewOrganization != nil {
			organizationID := uuid.New()
			organization := Organization{ID: organizationID, Revision: 1,
				Name: input.NewOrganization.Name, Defaults: input.NewOrganization.Defaults}
			if err := s.repository.SaveOrganization(tx.Context(), tx, authority, organization, 0, now); err != nil {
				return err
			}
			if err := s.record(tx.Context(), tx, authority, organizationID, 1, "organization",
				"created", "organization.create", &command, now); err != nil {
				return err
			}
			input.OrganizationID = &organizationID
		}
		value := Unit{ID: id, Revision: 1, Name: input.Name, OrganizationID: input.OrganizationID,
			CollectionIDs: input.CollectionIDs, Settings: input.Settings}
		if err := s.repository.SaveUnit(tx.Context(), tx, authority, value, 0, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, id, 1, "unit", "created", "unit.create", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: 1}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "unit.create", Fingerprint: hash, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// ChangeUnit changes only supported fields after checking both old and proposed authority contexts.
func (s *Service) ChangeUnit(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input UnitInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || id == uuid.Nil || expected <= 0 || input.NewOrganization != nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadUnit(tx.Context(), tx, authority, "unit.update", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.authorizeUnitUses(tx.Context(), tx, authority, "unit.update", current, now); err != nil {
			return err
		}
		proposed := current
		proposed.Name, proposed.OrganizationID, proposed.CollectionIDs = input.Name, input.OrganizationID, input.CollectionIDs
		proposed.Settings, proposed.Revision = input.Settings, next
		if err := s.authorizeUnitUses(tx.Context(), tx, authority, "unit.update", proposed, now); err != nil {
			return err
		}
		if proposed.OrganizationID != nil {
			if err := s.authorizeOrganizationReference(tx.Context(), tx, authority, *proposed.OrganizationID, now); err != nil {
				return err
			}
		}
		if err := s.checkMembership(tx.Context(), tx, authority, current.CollectionIDs, input.CollectionIDs, now); err != nil {
			return err
		}
		if current.Name == proposed.Name && equalUUIDPointer(current.OrganizationID, proposed.OrganizationID) &&
			slices.Equal(current.CollectionIDs, proposed.CollectionIDs) && reflect.DeepEqual(current.Settings, proposed.Settings) {
			result = commands.Result{ID: id, Revision: current.Revision}
			return nil
		}
		if err := s.repository.SaveUnit(tx.Context(), tx, authority, proposed, expected, now); err != nil {
			return err
		}
		change := "updated"
		if !slices.Equal(current.CollectionIDs, proposed.CollectionIDs) {
			change = "membership-changed"
		}
		if err := s.record(tx.Context(), tx, authority, id, proposed.Revision, "unit", change, "unit.update", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: proposed.Revision}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

type Deletion struct {
	commands.Result
	Status string `json:"status"`
}

// DeleteUnit marks nonselectability and durably invalidates rules; it never claims historical cleanup.
func (s *Service) DeleteUnit(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, check func(identity.Authentication) error) (Deletion, error) {
	if id == uuid.Nil || expected <= 0 {
		return Deletion{}, authz.ErrInvalidInput
	}
	var result Deletion
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadUnit(tx.Context(), tx, authority, "unit.delete", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.authorizeUnitUses(tx.Context(), tx, authority, "unit.delete", current, now); err != nil {
			return err
		}
		current.Deleted, current.Revision = true, next
		if err := s.repository.SaveUnit(tx.Context(), tx, authority, current, expected, now); err != nil {
			return err
		}
		if err := s.rules.InvalidateUnit(tx.Context(), tx, id); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, id, current.Revision, "unit", "deleted", "unit.delete", nil, now); err != nil {
			return err
		}
		result = Deletion{Result: commands.Result{ID: id, Revision: current.Revision}, Status: "pending"}
		return nil
	})
	if err != nil {
		return Deletion{}, err
	}
	return result, nil
}

// CreateCollection requires explicit current creation authority and has actor-bound retry semantics.
func (s *Service) CreateCollection(ctx context.Context, proof identity.Proof, command uuid.UUID, name string, check func(identity.Authentication) error) (commands.Result, error) {
	if ValidateName(name) != nil || command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	hash, err := commands.Fingerprint(name)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "unit-collection.create", nil, authz.Target{}, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("unit-collection.create", hash)
			return err
		}
		value := Collection{ID: uuid.New(), Revision: 1, Name: name}
		if err := s.repository.SaveCollection(tx.Context(), tx, authority, value, 0, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, value.ID, 1, "unitCollection", "created", "unit-collection.create", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: value.ID, Revision: 1}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "unit-collection.create", Fingerprint: hash, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// authorizeCollectionUses checks every affected member/use using bounded internal pages.
func (s *Service) authorizeCollectionUses(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) error {
	after := uuid.Nil
	anyMember := false
	for {
		values, err := s.repository.CollectionUnits(ctx, tx, id, after)
		if err != nil {
			return err
		}
		for _, value := range values {
			anyMember = true
			if err := s.authorizeUnitUses(ctx, tx, authority, action, value, now); err != nil {
				return err
			}
			after = value.ID
		}
		if len(values) < 200 {
			break
		}
	}
	if !anyMember {
		return authz.Authorize(authority, action, nil, authz.Target{UnitCollections: []uuid.UUID{id}}, now)
	}
	return nil
}

// ChangeCollection checks all uses before changing a shared label.
func (s *Service) ChangeCollection(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, name string, check func(identity.Authentication) error) (commands.Result, error) {
	if ValidateName(name) != nil || id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadCollection(tx.Context(), tx, authority, "unit-collection.update", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.authorizeCollectionUses(tx.Context(), tx, authority, "unit-collection.update", id, now); err != nil {
			return err
		}
		current.Name, current.Revision = name, next
		if err := s.repository.SaveCollection(tx.Context(), tx, authority, current, expected, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, id, current.Revision, "unitCollection", "updated", "unit-collection.update", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: current.Revision}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// DeleteCollection detaches members and invalidates references without modifying original evidence.
func (s *Service) DeleteCollection(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadCollection(tx.Context(), tx, authority, "unit-collection.delete", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.authorizeCollectionUses(tx.Context(), tx, authority, "unit-collection.delete", id, now); err != nil {
			return err
		}
		if err := s.repository.DeleteCollection(tx.Context(), tx, authority, current, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return s.record(tx.Context(), tx, authority, id, result.Revision, "unitCollection", "deleted", "unit-collection.delete", nil, now)
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}
