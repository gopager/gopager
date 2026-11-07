package ingest

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// GatewayCorrectionQueue records the re-resolution of a Gateway's accepted Transmissions after a
// mapping change: QueueGatewayCorrections queues one correction scope per Protocol, in the
// caller's transaction, returning the scope IDs in the order of the Protocols;
// GatewayCorrectionAgencies returns the Agencies such a scope can move Transmissions from or to
// (those of the current mapping and of the Networks the Gateway's Transmissions of the Protocol
// reference). The assignment correction store implements it.
type GatewayCorrectionQueue interface {
	QueueGatewayCorrections(context.Context, platform.TransactionContext, uuid.UUID,
		[]protocols.Protocol) ([]uuid.UUID, error)
	GatewayCorrectionAgencies(context.Context, platform.TransactionContext, uuid.UUID,
		protocols.Protocol) ([]uuid.UUID, error)
}

// GatewayNetworksResult is the outcome of ConfigureNetworks: the Gateway's new revision and the
// correction scopes queued to re-resolve its accepted Transmissions, one per Protocol whose
// candidate Networks changed (empty when none did).
type GatewayNetworksResult struct {
	Revision    int64       `json:"revision,string"`
	Corrections []uuid.UUID `json:"correctionIds"`
}

// changedProtocols returns, in ascending order, the Protocols whose resolution candidates differ
// between two mappings: the identifiers and Networks of the Protocol in priority order. Only the
// relative order within a Protocol matters, because ResolveNetwork compares the candidates of the
// submission Protocol only.
func changedProtocols(before, after []GatewayNetwork) []protocols.Protocol {
	seen := make(map[protocols.Protocol]bool)
	for _, network := range slices.Concat(before, after) {
		seen[network.Protocol] = true
	}
	changed := make([]protocols.Protocol, 0, len(seen))
	for protocol := range seen {
		if !slices.Equal(candidateList(before, protocol), candidateList(after, protocol)) {
			changed = append(changed, protocol)
		}
	}
	slices.Sort(changed)
	return changed
}

// candidate is one resolution candidate of a Protocol: what ResolveNetwork reads besides order.
type candidate struct {
	identifier string
	network    uuid.UUID
}

// candidateList returns the mapping's candidates of protocol in priority order.
func candidateList(mapping []GatewayNetwork, protocol protocols.Protocol) []candidate {
	filtered := slices.DeleteFunc(slices.Clone(mapping), func(network GatewayNetwork) bool {
		return network.Protocol != protocol
	})
	slices.SortFunc(filtered, func(left, right GatewayNetwork) int { return left.Priority - right.Priority })
	list := make([]candidate, len(filtered))
	for i, network := range filtered {
		list[i] = candidate{identifier: network.Identifier, network: network.NetworkID}
	}
	return list
}
