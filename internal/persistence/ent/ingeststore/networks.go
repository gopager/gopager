package ingeststore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

const networkColumns = `id,revision,name,protocol,agency_id`

// ReadNetworkRecord returns an undeleted Network, or authz.ErrNotFound.
func (s *Store) ReadNetworkRecord(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (ingest.Network, error) {
	return s.readNetwork(ctx, tx, id, "")
}

// LockNetwork returns an undeleted Network locked for the rest of the transaction, or authz.ErrNotFound.
func (s *Store) LockNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (ingest.Network, error) {
	return s.readNetwork(ctx, tx, id, " FOR UPDATE")
}

func (s *Store) readNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, lock string) (ingest.Network, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return ingest.Network{}, err
	}
	network, err := scanNetwork(executor.QueryRowContext(ctx,
		`SELECT `+networkColumns+` FROM networks WHERE id=$1 AND NOT deleted`+lock, id))
	return network, notFoundIfNoRows(err)
}

// ListNetworks pages the undeleted Networks of the reachable Agencies ordered by name and
// ID; Total counts all of them.
func (s *Store) ListNetworks(ctx context.Context, tx platform.TransactionContext, reach authz.AgencyReach,
	page units.PageInput) (units.Page[ingest.Network], error) {
	result := units.Page[ingest.Network]{Items: []ingest.Network{}}
	if !reach.All && len(reach.Agencies) == 0 {
		return result, nil
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return units.Page[ingest.Network]{}, err
	}
	const filter = ` FROM networks WHERE NOT deleted AND ($1 OR agency_id = ANY(coalesce($2::uuid[], '{}')))`
	agencies := append([]uuid.UUID{}, reach.Agencies...)
	if err := executor.QueryRowContext(ctx, `SELECT count(*)`+filter, reach.All, agencies).Scan(&result.Total); err != nil {
		return units.Page[ingest.Network]{}, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT `+networkColumns+filter+` ORDER BY name, id OFFSET $3 LIMIT $4`,
		reach.All, agencies, page.Offset, page.Limit)
	if err != nil {
		return units.Page[ingest.Network]{}, err
	}
	for rows.Next() {
		network, err := scanNetwork(rows)
		if err != nil {
			_ = rows.Close()
			return units.Page[ingest.Network]{}, err
		}
		result.Items = append(result.Items, network)
	}
	return result, closeRows(rows)
}

// CreateNetwork inserts a Network at revision 1; an unknown Agency is authz.ErrInvalidInput.
func (s *Store) CreateNetwork(ctx context.Context, tx platform.TransactionContext, network ingest.Network) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var agency uuid.UUID
	if err := executor.QueryRowContext(ctx, `SELECT id FROM agencies WHERE id=$1 FOR SHARE`, network.AgencyID).
		Scan(&agency); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authz.ErrInvalidInput
		}
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO networks (id,revision,name,protocol,agency_id) VALUES($1,1,$2,$3,$4)`,
		network.ID, network.Name, string(network.Protocol), network.AgencyID)
	return err
}

// SaveNetworkName stores a new name and revision of a Network locked by LockNetwork.
func (s *Store) SaveNetworkName(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, name string,
	revision int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `UPDATE networks SET name=$2, revision=$3 WHERE id=$1 AND NOT deleted`,
		id, name, revision)
	return err
}

// DeleteNetwork soft-deletes a Network locked by LockNetwork at the given revision. It fails
// with ingest.ErrNetworkInUse while a Gateway assignment, an Address rule or the scope of an
// undeleted Workflow references the Network. Their writers read the Network through
// ReadNetwork with a share lock, so none can start referencing it concurrently.
func (s *Store) DeleteNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, revision int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var used bool
	if err := executor.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM gateway_networks WHERE network_id=$1)
		OR EXISTS(SELECT 1 FROM address_rules WHERE network_id=$1)
		OR EXISTS(SELECT 1 FROM processing_workflow_scope_references r
			JOIN processing_workflows w ON w.id=r.workflow_id WHERE r.network_id=$1 AND NOT w.deleted)`,
		id).Scan(&used); err != nil {
		return err
	}
	if used {
		return ingest.ErrNetworkInUse
	}
	_, err = executor.ExecContext(ctx, `UPDATE networks SET deleted=true, revision=$2 WHERE id=$1 AND NOT deleted`,
		id, revision)
	return err
}

// RecordChange writes the identity owner's audit record of a Network or Gateway change.
func (s *Store) RecordChange(ctx context.Context, tx platform.TransactionContext, actor, target uuid.UUID,
	action string, now time.Time) error {
	owner, err := identitystore.New(s.primary)
	if err != nil {
		return err
	}
	return owner.RecordConfigurationChange(ctx, tx, actor, target, action, now)
}

type rowScanner interface{ Scan(...any) error }

func scanNetwork(row rowScanner) (ingest.Network, error) {
	var network ingest.Network
	var protocol string
	if err := row.Scan(&network.ID, &network.Revision, &network.Name, &protocol, &network.AgencyID); err != nil {
		return ingest.Network{}, err
	}
	network.Protocol = protocols.Protocol(protocol)
	return network, nil
}

var (
	_ ingest.NetworkRepository             = (*Store)(nil)
	_ identity.GatewayCredentialRepository = (*Store)(nil)
)
