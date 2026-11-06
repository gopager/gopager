package ingest

import (
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// ResolveNetwork applies only the assigned, protocol-compatible Gateway Network set, in order:
// the declared identifier ("declared"), the only candidate ("inferred-single"), the only
// candidate in addressMatches ("inferred-address"), the highest-priority candidate in
// addressMatches ("address-priority", the caller records the ambiguity) and finally the
// highest-priority candidate ("priority"). Choosing only among matching Networks keeps a
// Transmission off a Network whose own rules provably do not match. addressMatches lists the
// Networks with a matching Address rule; entries outside the candidates are ignored.
func ResolveNetwork(input ValidatedSubmission, assigned []GatewayNetwork,
	addressMatches []uuid.UUID) (uuid.UUID, string, error) {
	var candidates []GatewayNetwork
	identifiers := make(map[string]bool)
	for _, network := range assigned {
		if network.Identifier == "" || network.NetworkID == uuid.Nil || !network.Protocol.Valid() ||
			identifiers[network.Identifier] {
			return uuid.Nil, "", ErrInvalidSubmission
		}
		identifiers[network.Identifier] = true
		if network.Protocol == input.Protocol {
			candidates = append(candidates, network)
		}
	}
	if input.GatewayNetworkIdentifier != "" {
		for _, candidate := range candidates {
			if candidate.Identifier == input.GatewayNetworkIdentifier {
				return candidate.NetworkID, "declared", nil
			}
		}
		return uuid.Nil, "", ErrInvalidSubmission
	}
	if len(candidates) == 0 {
		return uuid.Nil, "", ErrInvalidSubmission
	}
	if len(candidates) == 1 {
		return candidates[0].NetworkID, "inferred-single", nil
	}
	var matching []GatewayNetwork
	for _, candidate := range candidates {
		if slices.Contains(addressMatches, candidate.NetworkID) {
			matching = append(matching, candidate)
		}
	}
	switch len(matching) {
	case 0:
		return highestPriority(candidates), "priority", nil
	case 1:
		return matching[0].NetworkID, "inferred-address", nil
	default:
		return highestPriority(matching), "address-priority", nil
	}
}

// highestPriority returns the Network with the lowest Priority number of a non-empty set.
func highestPriority(networks []GatewayNetwork) uuid.UUID {
	best := networks[0]
	for _, network := range networks[1:] {
		if network.Priority < best.Priority {
			best = network
		}
	}
	return best.NetworkID
}

// addressMatches asks the matcher which of the Gateway's Networks of the submission protocol
// have a rule matching the targets. It is only needed, and only called, when no Gateway Network
// is declared and more than one Network of the protocol competes; otherwise the result is nil.
func (s *Service) addressMatches(tx platform.TransactionContext, submission ValidatedSubmission,
	assigned []GatewayNetwork) ([]uuid.UUID, error) {
	if submission.GatewayNetworkIdentifier != "" {
		return nil, nil
	}
	var allowed []uuid.UUID
	for _, network := range assigned {
		if network.Protocol == submission.Protocol {
			allowed = append(allowed, network.NetworkID)
		}
	}
	if len(allowed) < 2 {
		return nil, nil
	}
	candidates, err := s.matcher.NetworksWithMatches(tx.Context(), tx, allowed, submission.Protocol,
		submission.ObservedAt, submission.TransmissionTargets)
	if err != nil {
		return nil, err
	}
	return candidates.Networks, nil
}
