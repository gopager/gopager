package main

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// processingScopeContext resolves Workflow scope references to their Agencies through the Network
// owner in use (production intake or a synthetic preview owner) and the master-data store.
type processingScopeContext struct {
	networks assignments.NetworkOwner
	units    *unitstore.Store
}

// ReadNetwork reads the Radio network's Protocol and Agency.
func (c processingScopeContext) ReadNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	return c.networks.ReadNetwork(ctx, tx, id)
}

// ReadUnitContext reads the Unit's current Agency, collections and deletion state.
func (c processingScopeContext) ReadUnitContext(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (units.Context, error) {
	return c.units.ReadUnitContext(ctx, tx, id)
}

type syntheticAssignmentNetworkOwner struct {
	owner synthetic.NetworkOwner
}

// ReadNetwork reads the synthetic Network's Protocol and Agency for matching and authorization.
func (owner syntheticAssignmentNetworkOwner) ReadNetwork(ctx context.Context,
	tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	if owner.owner == nil {
		return "", uuid.Nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Network owner is not wired"))
	}
	network, err := owner.owner.ReadSyntheticNetwork(ctx, tx, id)
	if err != nil {
		return "", uuid.Nil, err
	}
	if network.ID != id || network.Deleted || !network.Protocol.Valid() {
		return "", uuid.Nil, authz.ErrNotFound
	}
	return network.Protocol, network.AgencyID, nil
}
