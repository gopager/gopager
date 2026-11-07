package ingeststore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/units"
)

const gatewayColumns = `id,revision,name,enabled,state,last_contact_at,last_heartbeat_at,last_transmission_at`

// CreateGateway inserts an enabled, never-seen Gateway at revision 1 with its initial
// Network assignments; a missing, deleted or Protocol-mismatched Network is
// authz.ErrInvalidInput.
func (s *Store) CreateGateway(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, name string,
	networks []ingest.GatewayNetwork) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO gateways (id,revision,name,enabled,state)
		VALUES($1,1,$2,true,'never-seen')`, id, name); err != nil {
		return err
	}
	return writeGatewayNetworks(ctx, executor, id, networks)
}

// ReadGatewayRecord returns the management view of an undeleted Gateway with its
// assignments in priority order, or authz.ErrNotFound.
func (s *Store) ReadGatewayRecord(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (ingest.Gateway, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return ingest.Gateway{}, err
	}
	gateway, err := scanGateway(executor.QueryRowContext(ctx,
		`SELECT `+gatewayColumns+` FROM gateways WHERE id=$1 AND NOT deleted`, id))
	if err != nil {
		return ingest.Gateway{}, notFoundIfNoRows(err)
	}
	gateways := []ingest.Gateway{gateway}
	if err := attachGatewayNetworks(ctx, executor, gateways); err != nil {
		return ingest.Gateway{}, err
	}
	return gateways[0], nil
}

// ListGateways pages the undeleted Gateways ordered by name and ID. Without an
// installation-wide reach it returns only Gateways that have Networks, all of them in
// reachable Agencies; Total counts the same set.
func (s *Store) ListGateways(ctx context.Context, tx platform.TransactionContext, reach authz.AgencyReach,
	page units.PageInput) (units.Page[ingest.Gateway], error) {
	result := units.Page[ingest.Gateway]{Items: []ingest.Gateway{}}
	if !reach.All && len(reach.Agencies) == 0 {
		return result, nil
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return units.Page[ingest.Gateway]{}, err
	}
	// The agency list is never NULL here, and coalesce keeps the predicate fail-closed anyway:
	// a NULL comparison inside NOT EXISTS would otherwise make every Gateway visible.
	const filter = ` FROM gateways g WHERE NOT g.deleted AND ($1 OR (
		EXISTS(SELECT 1 FROM gateway_networks gn WHERE gn.gateway_id=g.id)
		AND NOT EXISTS(SELECT 1 FROM gateway_networks gn JOIN networks n ON n.id=gn.network_id
			WHERE gn.gateway_id=g.id AND n.agency_id <> ALL(coalesce($2::uuid[], '{}')))))`
	agencies := append([]uuid.UUID{}, reach.Agencies...)
	if err := executor.QueryRowContext(ctx, `SELECT count(*)`+filter, reach.All, agencies).Scan(&result.Total); err != nil {
		return units.Page[ingest.Gateway]{}, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT g.id,g.revision,g.name,g.enabled,g.state,g.last_contact_at,
		g.last_heartbeat_at,g.last_transmission_at`+filter+` ORDER BY g.name, g.id OFFSET $3 LIMIT $4`,
		reach.All, agencies, page.Offset, page.Limit)
	if err != nil {
		return units.Page[ingest.Gateway]{}, err
	}
	for rows.Next() {
		gateway, err := scanGateway(rows)
		if err != nil {
			_ = rows.Close()
			return units.Page[ingest.Gateway]{}, err
		}
		result.Items = append(result.Items, gateway)
	}
	if err := closeRows(rows); err != nil {
		return units.Page[ingest.Gateway]{}, err
	}
	return result, attachGatewayNetworks(ctx, executor, result.Items)
}

// ChangeGateway stores the name and enablement at the expected revision and returns the
// new revision; an unchanged Gateway keeps its revision. A stale revision is
// authz.ErrRevisionConflict and a deleted or missing Gateway authz.ErrNotFound.
func (s *Store) ChangeGateway(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64,
	change ingest.GatewayChange) (int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var current int64
	var name string
	var enabled bool
	if err := executor.QueryRowContext(ctx, `SELECT revision,name,enabled FROM gateways WHERE id=$1 AND NOT deleted
		FOR UPDATE`, id).Scan(&current, &name, &enabled); err != nil {
		return 0, notFoundIfNoRows(err)
	}
	next, err := commands.NextRevision(current, expected)
	if err != nil {
		return 0, err
	}
	if name == change.Name && enabled == change.Enabled {
		return current, nil
	}
	_, err = executor.ExecContext(ctx, `UPDATE gateways SET name=$2, enabled=$3, revision=$4 WHERE id=$1`,
		id, change.Name, change.Enabled, next)
	return next, err
}

// DeleteGateway soft-deletes the Gateway at the expected revision, removes its Network
// assignments and revokes every unrevoked credential at now. It returns the revoked
// credential IDs and the new revision.
func (s *Store) DeleteGateway(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64,
	now time.Time) ([]uuid.UUID, int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, 0, err
	}
	var current int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM gateways WHERE id=$1 AND NOT deleted FOR UPDATE`,
		id).Scan(&current); err != nil {
		return nil, 0, notFoundIfNoRows(err)
	}
	next, err := commands.NextRevision(current, expected)
	if err != nil {
		return nil, 0, err
	}
	rows, err := executor.QueryContext(ctx, `UPDATE gateway_credentials SET revoked_at=$2
		WHERE gateway_id=$1 AND revoked_at IS NULL RETURNING id`, id, now)
	if err != nil {
		return nil, 0, err
	}
	revoked := []uuid.UUID{}
	for rows.Next() {
		var credential uuid.UUID
		if err := rows.Scan(&credential); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		revoked = append(revoked, credential)
	}
	if err := closeRows(rows); err != nil {
		return nil, 0, err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM gateway_networks WHERE gateway_id=$1`, id); err != nil {
		return nil, 0, err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE gateways SET deleted=true, revision=$2 WHERE id=$1`, id, next); err != nil {
		return nil, 0, err
	}
	return revoked, next, nil
}

// ReadGatewayDiagnostics returns the diagnostic view of an undeleted Gateway, or authz.ErrNotFound.
func (s *Store) ReadGatewayDiagnostics(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (ingest.GatewayDiagnostics, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return ingest.GatewayDiagnostics{}, err
	}
	var result ingest.GatewayDiagnostics
	var contact, heartbeat, transmission, removed sql.NullTime
	var report []byte
	if err := executor.QueryRowContext(ctx, `SELECT id,name,enabled,state,last_contact_at,last_heartbeat_at,
		last_transmission_at,last_report,history_removed_before FROM gateways WHERE id=$1 AND NOT deleted`, id).
		Scan(&result.ID, &result.Name, &result.Enabled, &result.State, &contact, &heartbeat, &transmission,
			&report, &removed); err != nil {
		return ingest.GatewayDiagnostics{}, notFoundIfNoRows(err)
	}
	result.LastContactAt, result.LastHeartbeatAt = optionalTime(contact), optionalTime(heartbeat)
	result.LastTransmissionAt, result.HistoryRemovedBefore = optionalTime(transmission), optionalTime(removed)
	if report != nil {
		var stored ingest.GatewayStateReport
		if err := json.Unmarshal(report, &stored); err != nil {
			return ingest.GatewayDiagnostics{}, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		result.LastReport = &ingest.GatewayLastReport{Operation: stored.Operation, ReasonCode: stored.ReasonCode,
			Detail: stored.Detail}
	}
	return result, nil
}

// ListGatewayTransitions pages the recorded transitions of an undeleted Gateway, newest
// revision first; Total counts the retained transitions.
func (s *Store) ListGatewayTransitions(ctx context.Context, tx platform.TransactionContext, id uuid.UUID,
	page units.PageInput) (units.Page[ingest.GatewayTransition], error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return units.Page[ingest.GatewayTransition]{}, err
	}
	result := units.Page[ingest.GatewayTransition]{Items: []ingest.GatewayTransition{}}
	if err := executor.QueryRowContext(ctx, `SELECT count(h.id) FROM gateways g
		LEFT JOIN gateway_state_history h ON h.gateway_id=g.id WHERE g.id=$1 AND NOT g.deleted GROUP BY g.id`,
		id).Scan(&result.Total); err != nil {
		return units.Page[ingest.GatewayTransition]{}, notFoundIfNoRows(err)
	}
	rows, err := executor.QueryContext(ctx, `SELECT revision,old_state,new_state,cause,occurred_at,contact_at,
		coalesce(reason_code,''),coalesce(detail,'') FROM gateway_state_history WHERE gateway_id=$1
		ORDER BY revision DESC OFFSET $2 LIMIT $3`, id, page.Offset, page.Limit)
	if err != nil {
		return units.Page[ingest.GatewayTransition]{}, err
	}
	for rows.Next() {
		var transition ingest.GatewayTransition
		if err := rows.Scan(&transition.Revision, &transition.OldState, &transition.NewState, &transition.Cause,
			&transition.OccurredAt, &transition.ContactAt, &transition.ReasonCode, &transition.Detail); err != nil {
			_ = rows.Close()
			return units.Page[ingest.GatewayTransition]{}, err
		}
		transition.OccurredAt, transition.ContactAt = transition.OccurredAt.UTC(), transition.ContactAt.UTC()
		result.Items = append(result.Items, transition)
	}
	return result, closeRows(rows)
}

// writeGatewayNetworks validates and inserts a Gateway's Network assignments; every Network
// must exist undeleted with the assignment's Protocol (otherwise authz.ErrInvalidInput).
func writeGatewayNetworks(ctx context.Context, executor postgres.Executor, gateway uuid.UUID,
	networks []ingest.GatewayNetwork) error {
	for _, network := range networks {
		var protocol string
		var deleted bool
		if err := executor.QueryRowContext(ctx, `SELECT protocol,deleted FROM networks WHERE id=$1 FOR SHARE`,
			network.NetworkID).Scan(&protocol, &deleted); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return authz.ErrInvalidInput
			}
			return err
		}
		if deleted || protocol != string(network.Protocol) {
			return authz.ErrInvalidInput
		}
	}
	for _, network := range networks {
		if _, err := executor.ExecContext(ctx, `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,priority)
			VALUES($1,$2,$3,$4,$5)`, uuid.New(), gateway, network.Identifier, network.NetworkID, network.Priority); err != nil {
			return err
		}
	}
	return nil
}

// attachGatewayNetworks loads the assignments of the given Gateways in priority order.
func attachGatewayNetworks(ctx context.Context, executor postgres.Executor, gateways []ingest.Gateway) error {
	if len(gateways) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(gateways))
	index := make(map[uuid.UUID]int, len(gateways))
	for i := range gateways {
		ids[i], index[gateways[i].ID] = gateways[i].ID, i
		gateways[i].Networks = []ingest.GatewayNetworkView{}
	}
	rows, err := executor.QueryContext(ctx, `SELECT gateway_id,gateway_network_identifier,network_id,priority
		FROM gateway_networks WHERE gateway_id = ANY($1::uuid[]) ORDER BY gateway_id, priority`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var gateway uuid.UUID
		var view ingest.GatewayNetworkView
		if err := rows.Scan(&gateway, &view.Identifier, &view.NetworkID, &view.Priority); err != nil {
			_ = rows.Close()
			return err
		}
		position := index[gateway]
		gateways[position].Networks = append(gateways[position].Networks, view)
	}
	return closeRows(rows)
}

func scanGateway(row rowScanner) (ingest.Gateway, error) {
	var gateway ingest.Gateway
	var contact, heartbeat, transmission sql.NullTime
	if err := row.Scan(&gateway.ID, &gateway.Revision, &gateway.Name, &gateway.Enabled, &gateway.State,
		&contact, &heartbeat, &transmission); err != nil {
		return ingest.Gateway{}, err
	}
	gateway.LastContactAt, gateway.LastHeartbeatAt = optionalTime(contact), optionalTime(heartbeat)
	gateway.LastTransmissionAt = optionalTime(transmission)
	return gateway, nil
}

func optionalTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}

var _ ingest.GatewayLifecycleRepository = (*Store)(nil)
