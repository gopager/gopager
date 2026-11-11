package ingeststore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
)

var _ synthetic.NetworkOwner = (*Store)(nil)

// ReadSyntheticNetwork returns the current Network row (identifier, name, Protocol, Agency and
// deleted flag) for a synthetic Workflow preview in the caller's transaction. Deleted
// Networks are returned with Deleted set so the caller can reject them explicitly; a
// missing Network is authz.ErrNotFound.
func (s *Store) ReadSyntheticNetwork(ctx context.Context, tx platform.TransactionContext,
	id uuid.UUID,
) (synthetic.Network, error) {
	if id == uuid.Nil {
		return synthetic.Network{}, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return synthetic.Network{}, err
	}
	var network synthetic.Network
	var protocol string
	if err := executor.QueryRowContext(ctx, `SELECT id,name,protocol,deleted,agency_id FROM networks WHERE id=$1`,
		id).Scan(&network.ID, &network.Name, &protocol, &network.Deleted, &network.AgencyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return synthetic.Network{}, authz.ErrNotFound
		}
		return synthetic.Network{}, err
	}
	network.Protocol = protocols.Protocol(protocol)
	return network, nil
}
