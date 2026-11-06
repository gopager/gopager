package assignments

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// maxCandidateNetworks bounds the Network set one NetworksWithMatches call evaluates.
const maxCandidateNetworks = 1024

// NetworksWithMatches reports which of the allowed Networks of the protocol have a current rule
// matching at least one target at the observation time. Ingest uses it to choose among several
// Networks of one Gateway. The allowed set is server-derived from the Gateway assignment and
// never broadened; all Networks are evaluated at one captured assignment revision under the
// caller's transaction. Targetless input matches no Network. A set above 1024 entries or invalid
// targets fail with authz.ErrInvalidInput.
func (s *Service) NetworksWithMatches(ctx context.Context, tx platform.TransactionContext, allowed []uuid.UUID,
	protocol protocols.Protocol, observed time.Time, targets []protocols.Target) (NetworkCandidates, error) {
	if len(allowed) > maxCandidateNetworks {
		return NetworkCandidates{}, authz.ErrInvalidInput
	}
	if err := validateTargets(ctx, protocol, observed, targets); err != nil {
		return NetworkCandidates{}, err
	}
	revision, err := s.repository.LockRevision(ctx, tx)
	if err != nil {
		return NetworkCandidates{}, err
	}
	rules, err := s.repository.CurrentRules(ctx, tx, allowed)
	if err != nil {
		return NetworkCandidates{}, err
	}
	result := NetworkCandidates{Revision: revision, Networks: []uuid.UUID{}}
	seen := make(map[uuid.UUID]bool, len(allowed))
	for _, network := range allowed {
		if seen[network] {
			continue
		}
		seen[network] = true
		actual, _, err := s.network(ctx, tx, network)
		if err != nil {
			return NetworkCandidates{}, err
		}
		if actual != protocol {
			continue
		}
		resolved, err := s.resolve(ctx, tx, network, protocol, observed, targets, rules, revision)
		if err != nil {
			return NetworkCandidates{}, err
		}
		if len(resolved.UnitContext) > 0 {
			result.Networks = append(result.Networks, network)
		}
	}
	return result, nil
}
