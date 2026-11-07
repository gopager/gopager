package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// ErrNetworkInUse reports the deletion of a Network that is still assigned to a
// Gateway or referenced by an Address rule or Workflow. It names none of them,
// because the caller may not be allowed to see them.
var ErrNetworkInUse = platform.NewFailure(platform.CodeRevisionConflict,
	errors.New("the network is still assigned to a gateway or referenced by an address rule or workflow"))

// Network is the management view of a Radio network. Protocol and AgencyID are
// fixed at creation.
type Network struct {
	ID       uuid.UUID          `json:"id"`
	Revision int64              `json:"revision,string"`
	Name     string             `json:"name"`
	Protocol protocols.Protocol `json:"protocol"`
	AgencyID uuid.UUID          `json:"agencyId"`
}

// NetworkInput is the caller-supplied content of a new Network.
type NetworkInput struct {
	Name     string             `json:"name"`
	Protocol protocols.Protocol `json:"protocol"`
	AgencyID uuid.UUID          `json:"agencyId"`
}

// NetworkRepository persists Networks in the caller's transaction. Reads and
// locks treat deleted or missing Networks as authz.ErrNotFound; LockNetwork
// holds the row for the rest of the transaction. CreateNetwork fails with
// authz.ErrInvalidInput for an unknown Agency. DeleteNetwork soft-deletes and
// fails with ErrNetworkInUse while the Network is referenced. RecordChange
// writes the audit record of a configuration change.
type NetworkRepository interface {
	ReadNetworkRecord(context.Context, platform.TransactionContext, uuid.UUID) (Network, error)
	LockNetwork(context.Context, platform.TransactionContext, uuid.UUID) (Network, error)
	ListNetworks(context.Context, platform.TransactionContext, authz.AgencyReach, units.PageInput) (units.Page[Network], error)
	CreateNetwork(context.Context, platform.TransactionContext, Network) error
	SaveNetworkName(context.Context, platform.TransactionContext, uuid.UUID, string, int64) error
	DeleteNetwork(context.Context, platform.TransactionContext, uuid.UUID, int64) error
	RecordChange(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID, string, time.Time) error
}

// NetworkAdministration implements the authorized Network lifecycle: create,
// read, list, rename and delete. Every operation runs in one transaction that
// resolves the caller's authority and reads the database clock; authorization
// is evaluated against the Network's Agency. A Network on which the caller holds no
// right is reported as authz.ErrNotFound, one without the requested right as
// authz.ErrForbidden. It is safe for concurrent use.
type NetworkAdministration struct {
	transactions identity.Transactions
	resolver     *identity.Resolver
	clock        platform.Clock
	repository   NetworkRepository
	receipts     commands.Repository
}

// NewNetworkAdministration returns a NetworkAdministration, failing with
// CodeInvalidInput if any dependency is nil.
func NewNetworkAdministration(transactions identity.Transactions, resolver *identity.Resolver, clock platform.Clock,
	repository NetworkRepository, receipts commands.Repository) (*NetworkAdministration, error) {
	if transactions == nil || resolver == nil || clock == nil || repository == nil || receipts == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Network administration dependency"))
	}
	return &NetworkAdministration{transactions: transactions, resolver: resolver, clock: clock,
		repository: repository, receipts: receipts}, nil
}

// ReadNetwork returns one undeleted Network the caller may read (network.read on its Agency).
func (s *NetworkAdministration) ReadNetwork(ctx context.Context, proof identity.Proof, id uuid.UUID) (Network, error) {
	if id == uuid.Nil {
		return Network{}, authz.ErrInvalidInput
	}
	var result Network
	err := identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, false, nil,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			network, err := s.repository.ReadNetworkRecord(tx.Context(), tx, id)
			if err != nil {
				return err
			}
			if err := hideForbidden(authz.Authorize(authority, "network.read", nil, authz.Target{AgencyID: network.AgencyID}, now)); err != nil {
				return err
			}
			result = network
			return nil
		})
	return result, err
}

// ListNetworks pages the undeleted Networks of the Agencies in which the caller has network.read.
func (s *NetworkAdministration) ListNetworks(ctx context.Context, proof identity.Proof, page units.PageInput) (units.Page[Network], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Network]{}, err
	}
	var result units.Page[Network]
	err = identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, false, nil,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			reach, err := authz.ReachableAgencies(authority, "network.read", now)
			if err != nil {
				return err
			}
			result, err = s.repository.ListNetworks(tx.Context(), tx, reach, page)
			return err
		})
	return result, err
}

// CreateNetwork creates a Network in an Agency (network.create on that Agency)
// with actor-bound command idempotency.
func (s *NetworkAdministration) CreateNetwork(ctx context.Context, proof identity.Proof, command uuid.UUID,
	input NetworkInput, check func(identity.Authentication) error) (commands.Result, error) {
	if units.ValidateName(input.Name) != nil || command == uuid.Nil || input.AgencyID == uuid.Nil ||
		!input.Protocol.Valid() || check == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, true, check,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			if err := authz.Authorize(authority, "network.create", nil, authz.Target{AgencyID: input.AgencyID}, now); err != nil {
				return err
			}
			actor, err := uuid.Parse(authority.PrincipalID)
			if err != nil {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
			}
			receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
			if err != nil {
				return err
			}
			if found {
				result, err = receipt.Check("network.create", fingerprint)
				return err
			}
			network := Network{ID: uuid.New(), Revision: 1, Name: input.Name, Protocol: input.Protocol, AgencyID: input.AgencyID}
			if err := s.repository.CreateNetwork(tx.Context(), tx, network); err != nil {
				return err
			}
			if err := s.repository.RecordChange(tx.Context(), tx, actor, network.ID, "network.create", now); err != nil {
				return err
			}
			result = commands.Result{ID: network.ID, Revision: 1}
			return s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
				Operation: "network.create", Fingerprint: fingerprint, Result: result, RecordedAt: now})
		})
	return result, err
}

// RenameNetwork changes the Network's name at the expected revision (network.update
// on its Agency). An unchanged name returns the current revision without a write.
func (s *NetworkAdministration) RenameNetwork(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	name string, check func(identity.Authentication) error) (commands.Result, error) {
	if units.ValidateName(name) != nil || id == uuid.Nil || expected <= 0 || check == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.mutate(ctx, proof, id, "network.update", check,
		func(tx platform.TransactionContext, actor uuid.UUID, current Network, now time.Time) error {
			next, err := commands.NextRevision(current.Revision, expected)
			if err != nil {
				return err
			}
			if current.Name == name {
				result = commands.Result{ID: id, Revision: current.Revision}
				return nil
			}
			if err := s.repository.SaveNetworkName(tx.Context(), tx, id, name, next); err != nil {
				return err
			}
			result = commands.Result{ID: id, Revision: next}
			return s.repository.RecordChange(tx.Context(), tx, actor, id, "network.update", now)
		})
	return result, err
}

// DeleteNetwork soft-deletes an unreferenced Network at the expected revision
// (network.delete on its Agency); a referenced one fails with ErrNetworkInUse.
// Accepted Transmissions keep their reference to the deleted Network.
func (s *NetworkAdministration) DeleteNetwork(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 || check == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.mutate(ctx, proof, id, "network.delete", check,
		func(tx platform.TransactionContext, actor uuid.UUID, current Network, now time.Time) error {
			next, err := commands.NextRevision(current.Revision, expected)
			if err != nil {
				return err
			}
			if err := s.repository.DeleteNetwork(tx.Context(), tx, id, next); err != nil {
				return err
			}
			result = commands.Result{ID: id, Revision: next}
			return s.repository.RecordChange(tx.Context(), tx, actor, id, "network.delete", now)
		})
	return result, err
}

// mutate locks the Network, authorizes action on its Agency and runs change in the
// same transaction. A denied caller gets authz.ErrForbidden when it holds any
// right on the Network and authz.ErrNotFound otherwise.
func (s *NetworkAdministration) mutate(ctx context.Context, proof identity.Proof, id uuid.UUID, action string,
	check func(identity.Authentication) error,
	change func(platform.TransactionContext, uuid.UUID, Network, time.Time) error) error {
	return identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, true, check,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			current, err := s.repository.LockNetwork(tx.Context(), tx, id)
			if err != nil {
				return err
			}
			target := authz.Target{AgencyID: current.AgencyID}
			if err := authz.Authorize(authority, action, nil, target, now); err != nil {
				if errors.Is(err, authz.ErrForbidden) && knowsNetwork(authority, target, now) {
					return err
				}
				return hideForbidden(err)
			}
			actor, err := uuid.Parse(authority.PrincipalID)
			if err != nil {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
			}
			return change(tx, actor, current, now)
		})
}

// knowsNetwork reports whether the caller holds any right on the Network and may
// therefore learn that it exists.
func knowsNetwork(authority authz.Authority, target authz.Target, now time.Time) bool {
	for _, action := range []string{"network.read", "network.update", "network.delete"} {
		if authz.Authorize(authority, action, nil, target, now) == nil {
			return true
		}
	}
	return false
}

// hideForbidden reports a denied decision on an existing object as not found, so
// that callers outside its scope cannot learn that it exists.
func hideForbidden(err error) error {
	if errors.Is(err, authz.ErrForbidden) {
		return authz.ErrNotFound
	}
	return err
}
