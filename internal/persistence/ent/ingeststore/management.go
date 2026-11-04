package ingeststore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

func (s *Store) ReplaceGatewayNetworks(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID,
	expected int64, networks []ingest.GatewayNetwork, now time.Time) (int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var current int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM gateways WHERE id=$1 FOR UPDATE`,
		gateway).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, authz.ErrNotFound
		}
		return 0, err
	}
	next, err := commands.NextRevision(current, expected)
	if err != nil {
		return 0, err
	}
	for _, network := range networks {
		var protocol string
		var deleted bool
		if err := executor.QueryRowContext(ctx, `SELECT protocol,deleted FROM networks WHERE id=$1 FOR SHARE`,
			network.NetworkID).Scan(&protocol, &deleted); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, authz.ErrInvalidInput
			}
			return 0, err
		}
		if deleted || protocol != string(network.Protocol) {
			return 0, authz.ErrInvalidInput
		}
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM gateway_networks WHERE gateway_id=$1`, gateway); err != nil {
		return 0, err
	}
	for _, network := range networks {
		if _, err := executor.ExecContext(ctx, `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,$3,$4,$5)`, uuid.New(), gateway, network.Identifier, network.NetworkID, network.Default); err != nil {
			return 0, err
		}
	}
	if err := executor.QueryRowContext(ctx, `UPDATE gateways SET revision=$2 WHERE id=$1
		RETURNING revision`, gateway, next).Scan(&next); err != nil {
		return 0, err
	}
	return next, nil
}

func (s *Store) GatewayNetworksForAuthorization(ctx context.Context, tx platform.TransactionContext,
	gateway uuid.UUID) ([]ingest.GatewayNetwork, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	var exists uuid.UUID
	if err := executor.QueryRowContext(ctx, `SELECT id FROM gateways WHERE id=$1 FOR UPDATE`, gateway).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, authz.ErrNotFound
		}
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT gn.gateway_network_identifier,gn.network_id,n.protocol,gn.is_default
		FROM gateway_networks gn JOIN networks n ON n.id=gn.network_id
		WHERE gn.gateway_id=$1 ORDER BY gn.gateway_network_identifier`, gateway)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ingest.GatewayNetwork
	for rows.Next() {
		var network ingest.GatewayNetwork
		var protocol string
		if err := rows.Scan(&network.Identifier, &network.NetworkID, &protocol, &network.Default); err != nil {
			return nil, err
		}
		network.Protocol = protocols.Protocol(protocol)
		result = append(result, network)
	}
	return result, rows.Err()
}

func (s *Store) GatewayCredentialStatus(ctx context.Context, tx platform.TransactionContext,
	gateway uuid.UUID) ([]ingest.GatewayCredentialView, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	var exists uuid.UUID
	if err := executor.QueryRowContext(ctx, `SELECT id FROM gateways WHERE id=$1`, gateway).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, authz.ErrNotFound
		}
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id,pending,created_at,last_used_at,revoked_at
		FROM gateway_credentials WHERE gateway_id=$1 ORDER BY created_at,id`, gateway)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ingest.GatewayCredentialView, 0)
	for rows.Next() {
		var view ingest.GatewayCredentialView
		var pending bool
		var lastUsed, revoked sql.NullTime
		var created time.Time
		if err := rows.Scan(&view.ID, &pending, &created, &lastUsed, &revoked); err != nil {
			return nil, err
		}
		view.Status = "current"
		if pending {
			view.Status = "pending"
		}
		if revoked.Valid {
			view.Status = "revoked"
			value := revoked.Time.UTC().Format(time.RFC3339Nano)
			view.RevokedAt = &value
		}
		view.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		if lastUsed.Valid {
			value := lastUsed.Time.UTC().Format(time.RFC3339Nano)
			view.LastUsedAt = &value
		}
		result = append(result, view)
	}
	return result, rows.Err()
}

func (s *Store) IssueGatewayCredential(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID,
	expected int64, credential uuid.UUID, hash [32]byte, now time.Time) (bool, int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, 0, err
	}
	var current int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM gateways WHERE id=$1 FOR UPDATE`,
		gateway).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, 0, authz.ErrNotFound
		}
		return false, 0, err
	}
	next, err := commands.NextRevision(current, expected)
	if err != nil {
		return false, 0, err
	}
	var hasCurrent, hasPending bool
	if err := executor.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM gateway_credentials WHERE gateway_id=$1 AND revoked_at IS NULL AND pending=false),
		EXISTS(SELECT 1 FROM gateway_credentials WHERE gateway_id=$1 AND revoked_at IS NULL AND pending=true)`,
		gateway).Scan(&hasCurrent, &hasPending); err != nil {
		return false, 0, err
	}
	if hasPending {
		return false, 0, authz.ErrRevisionConflict
	}
	pending := hasCurrent
	if _, err := executor.ExecContext(ctx, `INSERT INTO gateway_credentials
		(id,gateway_id,secret_hash,pending,created_at) VALUES($1,$2,$3,$4,$5)`,
		credential, gateway, hash[:], pending, now); err != nil {
		return false, 0, err
	}
	if err := executor.QueryRowContext(ctx, `UPDATE gateways SET revision=$2 WHERE id=$1
		RETURNING revision`, gateway, next).Scan(&next); err != nil {
		return false, 0, err
	}
	return pending, next, nil
}

func (s *Store) RevokeGatewayCredential(ctx context.Context, tx platform.TransactionContext, gateway,
	credential uuid.UUID, expected int64, now time.Time) (int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var current int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM gateways WHERE id=$1 FOR UPDATE`,
		gateway).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, authz.ErrNotFound
		}
		return 0, err
	}
	next, err := commands.NextRevision(current, expected)
	if err != nil {
		return 0, err
	}
	result, err := executor.ExecContext(ctx, `UPDATE gateway_credentials SET revoked_at=$3
		WHERE gateway_id=$1 AND id=$2 AND revoked_at IS NULL`, gateway, credential, now)
	if err != nil {
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed != 1 {
		return 0, authz.ErrNotFound
	}
	if err := executor.QueryRowContext(ctx, `UPDATE gateways SET revision=$2 WHERE id=$1
		RETURNING revision`, gateway, next).Scan(&next); err != nil {
		return 0, err
	}
	return next, nil
}

var _ ingest.GatewayAdministrationRepository = (*Store)(nil)
