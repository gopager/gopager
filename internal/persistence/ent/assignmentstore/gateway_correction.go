package assignmentstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// QueueGatewayCorrections records, in the caller's transaction, one gateway correction scope per
// Protocol (reason gateway-mapping-change) at one new Assignment revision and returns their IDs in
// the order of protocols. It returns an empty result without bumping the revision when protocols
// is empty, and authz.ErrInvalidInput for a nil Gateway or an invalid Protocol.
func (s *Store) QueueGatewayCorrections(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID,
	protocols []protocols.Protocol) ([]uuid.UUID, error) {
	if len(protocols) == 0 {
		return []uuid.UUID{}, nil
	}
	scopes := make([]assignments.CorrectionScope, len(protocols))
	for i, protocol := range protocols {
		scopes[i] = assignments.GatewayScope(gateway, protocol)
	}
	revision, err := s.MarkPending(ctx, tx, scopes, "gateway-mapping-change")
	if err != nil {
		return nil, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(protocols))
	for i, protocol := range protocols {
		if err := executor.QueryRowContext(ctx, `SELECT id FROM assignment_pending_scopes
			WHERE assignment_revision=$1 AND gateway_id=$2 AND protocol=$3`, revision, gateway,
			string(protocol)).Scan(&ids[i]); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// GatewayCorrectionAgencies returns, in ascending order and without duplicates, the Agencies of the
// Gateway's current Networks of the Protocol and of the Networks its Transmissions of that Protocol
// currently reference: the Agencies a gateway scope affects.
func (s *Store) GatewayCorrectionAgencies(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID,
	protocol protocols.Protocol) ([]uuid.UUID, error) {
	if gateway == uuid.Nil || !protocol.Valid() {
		return nil, authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT n.agency_id FROM networks n WHERE n.protocol=$2 AND (
			n.id IN (SELECT network_id FROM gateway_networks WHERE gateway_id=$1)
			OR n.id IN (SELECT DISTINCT network_id FROM transmissions WHERE gateway_id=$1 AND protocol=$2))
		GROUP BY n.agency_id ORDER BY n.agency_id`, gateway, string(protocol))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agencies := make([]uuid.UUID, 0)
	for rows.Next() {
		var agency uuid.UUID
		if err := rows.Scan(&agency); err != nil {
			return nil, err
		}
		agencies = append(agencies, agency)
	}
	return agencies, rows.Err()
}

// ruleInferenceScopes returns one gateway scope per Gateway and Protocol whose current mapping
// contains the Network of one of the rule scopes and at least two Networks of that Protocol, in
// ascending Gateway and Protocol order. Address rules decide between such Networks during
// inference, so a rule change can move their accepted Transmissions; a single-Network mapping
// never depends on rules. Scopes of another kind are ignored.
func ruleInferenceScopes(ctx context.Context, executor postgres.Executor,
	scopes []assignments.CorrectionScope) ([]assignments.CorrectionScope, error) {
	networks := make([]uuid.UUID, 0, len(scopes))
	for _, scope := range scopes {
		if scope.Kind() == "address-rules" {
			networks = append(networks, *scope.NetworkID)
		}
	}
	result := make([]assignments.CorrectionScope, 0)
	if len(networks) == 0 {
		return result, nil
	}
	rows, err := executor.QueryContext(ctx, `SELECT gn.gateway_id, n.protocol FROM gateway_networks gn
		JOIN networks n ON n.id=gn.network_id
		WHERE n.id = ANY($1::uuid[]) AND (SELECT count(*) FROM gateway_networks other
			JOIN networks o ON o.id=other.network_id
			WHERE other.gateway_id=gn.gateway_id AND o.protocol=n.protocol) > 1
		GROUP BY gn.gateway_id, n.protocol ORDER BY gn.gateway_id, n.protocol`, networks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var gateway uuid.UUID
		var protocol string
		if err := rows.Scan(&gateway, &protocol); err != nil {
			return nil, err
		}
		result = append(result, assignments.GatewayScope(gateway, protocols.Protocol(protocol)))
	}
	return result, rows.Err()
}
